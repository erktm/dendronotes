// Package sso signs people in through an OpenID Connect provider such as
// Keycloak: authorization code flow with PKCE, state and nonce. The ID token
// is checked for signature, issuer, audience, expiry and nonce; the account
// behind it is the business of the caller (internal/auth).
package sso

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config describes the provider and how dendronotes is registered there.
type Config struct {
	Issuer        string
	ClientID      string
	ClientSecret  string // empty for a public client
	RedirectURL   string // PUBLIC_URL + /api/auth/oidc/callback
	PostLogoutURL string // where the provider sends the browser after logout
	Scopes        []string
	UsernameClaim string
	GroupsClaim   string // dotted path into the claims, e.g. realm_access.roles
	CAPEM         []byte // extra trusted CA certificates
}

// Claims is what dendronotes takes from the provider about a person.
type Claims struct {
	Issuer   string
	Subject  string
	Username string
	Name     string
	Email    string
	Groups   []string
}

// Status describes the connection to the provider (admin area).
type Status struct {
	Issuer      string    `json:"issuer"`
	ClientID    string    `json:"clientId"`
	RedirectURL string    `json:"redirectUrl"`
	Ready       bool      `json:"ready"`
	CheckedAt   time.Time `json:"checkedAt"`
	Error       string    `json:"error,omitempty"`
	Logout      bool      `json:"logout"` // the provider offers RP-initiated logout
}

// Errors returned by Finish.
var (
	ErrExpired = errors.New("unknown or expired sign-in attempt")
	ErrBusy    = errors.New("too many sign-in attempts in progress")
)

const (
	pendingTTL  = 10 * time.Minute
	maxPending  = 10_000
	retryAfter  = 10 * time.Second // between failed discovery attempts
	httpTimeout = 15 * time.Second
)

// Provider talks to one OpenID Connect provider.
type Provider struct {
	cfg    Config
	log    *slog.Logger
	client *http.Client
	base   context.Context // carries client; never cancelled (used by the key set)

	mu         sync.Mutex
	op         *oidc.Provider
	verifier   *oidc.IDTokenVerifier
	oauth      oauth2.Config
	endSession string
	status     Status
	lastTry    time.Time

	pmu     sync.Mutex
	pending map[string]pending
}

type pending struct {
	nonce    string
	verifier string
	next     string
	expires  time.Time
}

// New prepares a provider; discovery happens on first use or via Start.
func New(cfg Config, log *slog.Logger) (*Provider, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if len(cfg.CAPEM) > 0 && !pool.AppendCertsFromPEM(cfg.CAPEM) {
		return nil, errors.New("no usable CA certificates")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	client := &http.Client{Transport: tr, Timeout: httpTimeout}
	p := &Provider{
		cfg:     cfg,
		log:     log,
		client:  client,
		base:    oidc.ClientContext(context.Background(), client),
		pending: map[string]pending{},
		status:  Status{Issuer: cfg.Issuer, ClientID: cfg.ClientID, RedirectURL: cfg.RedirectURL},
	}
	return p, nil
}

// Start keeps trying discovery in the background until it works, so the
// admin area shows the state early and the first sign-in is fast.
func (p *Provider) Start(ctx context.Context) {
	go func() {
		for {
			if _, err := p.discover(false); err == nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
	}()
}

// Check runs discovery again and returns the resulting status.
func (p *Provider) Check() Status {
	_, _ = p.discover(true)
	return p.Status()
}

// Status reports the connection state.
func (p *Provider) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// discover loads the provider metadata. Unless forced, a working provider is
// reused and failures are retried at most every few seconds.
func (p *Provider) discover(force bool) (*oidc.Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.op != nil && !force {
		return p.op, nil
	}
	if !force && !p.lastTry.IsZero() && time.Since(p.lastTry) < retryAfter {
		return nil, errors.New(p.status.Error)
	}
	p.lastTry = time.Now()
	p.status.CheckedAt = p.lastTry.UTC()
	op, err := oidc.NewProvider(p.base, p.cfg.Issuer)
	if err != nil {
		p.status.Error = err.Error()
		p.log.Warn("OIDC discovery failed", "issuer", p.cfg.Issuer, "err", err)
		if p.op != nil {
			return p.op, nil // keep using what worked before
		}
		return nil, err
	}
	var meta struct {
		EndSession string `json:"end_session_endpoint"`
	}
	_ = op.Claims(&meta)
	p.op = op
	p.verifier = op.VerifierContext(p.base, &oidc.Config{ClientID: p.cfg.ClientID})
	p.oauth = oauth2.Config{
		ClientID:     p.cfg.ClientID,
		ClientSecret: p.cfg.ClientSecret,
		Endpoint:     op.Endpoint(),
		RedirectURL:  p.cfg.RedirectURL,
		Scopes:       p.cfg.Scopes,
	}
	p.endSession = meta.EndSession
	p.status.Ready, p.status.Error, p.status.Logout = true, "", meta.EndSession != ""
	p.log.Info("OIDC provider ready", "issuer", p.cfg.Issuer)
	return op, nil
}

// Begin starts a sign-in: it remembers state, nonce and PKCE verifier and
// returns the provider URL to send the browser to, plus the state, which the
// caller binds to the browser (cookie).
func (p *Provider) Begin(next string) (authURL, state string, err error) {
	if _, err := p.discover(false); err != nil {
		return "", "", err
	}
	state, nonce, verifier := random(), random(), oauth2.GenerateVerifier()
	if err := p.remember(state, pending{nonce: nonce, verifier: verifier, next: next, expires: time.Now().Add(pendingTTL)}); err != nil {
		return "", "", err
	}
	p.mu.Lock()
	cfg := p.oauth
	p.mu.Unlock()
	return cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), state, nil
}

// Finish completes a sign-in: it redeems the code (once per state), verifies
// the ID token and returns the person's claims and where to go next.
func (p *Provider) Finish(ctx context.Context, state, code string) (Claims, string, error) {
	pd, ok := p.take(state)
	if !ok {
		return Claims{}, "", ErrExpired
	}
	op, err := p.discover(false)
	if err != nil {
		return Claims{}, "", err
	}
	p.mu.Lock()
	cfg, verifier := p.oauth, p.verifier
	p.mu.Unlock()

	ctx = oidc.ClientContext(ctx, p.client)
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(pd.verifier))
	if err != nil {
		return Claims{}, "", fmt.Errorf("redeeming the code: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return Claims{}, "", errors.New("the provider returned no ID token")
	}
	idt, err := verifier.Verify(ctx, raw)
	if err != nil {
		return Claims{}, "", fmt.Errorf("checking the ID token: %w", err)
	}
	if idt.Nonce != pd.nonce {
		return Claims{}, "", errors.New("the ID token carries the wrong nonce")
	}
	var all map[string]any
	if err := idt.Claims(&all); err != nil {
		return Claims{}, "", fmt.Errorf("reading the ID token: %w", err)
	}
	c := p.claims(idt.Issuer, idt.Subject, all)

	// Some providers put names, e-mail or groups only into the userinfo
	// response; ask there for what the ID token lacks.
	if c.Username == "" || c.Name == "" || c.Email == "" || (c.Groups == nil && p.cfg.GroupsClaim != "") {
		if op.UserInfoEndpoint() != "" {
			if ui, err := op.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil && ui.Subject == idt.Subject {
				var more map[string]any
				if ui.Claims(&more) == nil {
					extra := p.claims(idt.Issuer, idt.Subject, more)
					c.Username = cmpOr(c.Username, extra.Username)
					c.Name = cmpOr(c.Name, extra.Name)
					c.Email = cmpOr(c.Email, extra.Email)
					if c.Groups == nil {
						c.Groups = extra.Groups
					}
				}
			}
		}
	}
	return c, pd.next, nil
}

func (p *Provider) claims(issuer, subject string, all map[string]any) Claims {
	c := Claims{Issuer: issuer, Subject: subject}
	c.Username, _ = lookup(all, p.cfg.UsernameClaim).(string)
	c.Name, _ = all["name"].(string)
	c.Email, _ = all["email"].(string)
	if p.cfg.GroupsClaim != "" {
		switch v := lookup(all, p.cfg.GroupsClaim).(type) {
		case []any:
			c.Groups = []string{}
			for _, g := range v {
				if s, ok := g.(string); ok {
					c.Groups = append(c.Groups, s)
				}
			}
		case string:
			c.Groups = strings.Fields(strings.ReplaceAll(v, ",", " "))
		}
	}
	return c
}

// LogoutURL is where the browser signs out at the provider too ("" if the
// provider does not offer it).
func (p *Provider) LogoutURL() string {
	p.mu.Lock()
	end := p.endSession
	p.mu.Unlock()
	if end == "" {
		return ""
	}
	u, err := url.Parse(end)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("client_id", p.cfg.ClientID)
	q.Set("post_logout_redirect_uri", p.cfg.PostLogoutURL)
	u.RawQuery = q.Encode()
	return u.String()
}

// InGroup reports whether any of groups is one of wanted. A leading slash
// (Keycloak's full group path) is ignored on both sides.
func InGroup(groups, wanted []string) bool {
	for _, g := range groups {
		g = strings.TrimPrefix(g, "/")
		for _, w := range wanted {
			if g == strings.TrimPrefix(w, "/") {
				return true
			}
		}
	}
	return false
}

func (p *Provider) remember(state string, pd pending) error {
	p.pmu.Lock()
	defer p.pmu.Unlock()
	if len(p.pending) >= maxPending {
		now := time.Now()
		for k, v := range p.pending {
			if now.After(v.expires) {
				delete(p.pending, k)
			}
		}
		if len(p.pending) >= maxPending {
			return ErrBusy
		}
	}
	p.pending[state] = pd
	return nil
}

// Discard forgets a sign-in attempt (the provider reported an error).
func (p *Provider) Discard(state string) { p.take(state) }

func (p *Provider) take(state string) (pending, bool) {
	p.pmu.Lock()
	defer p.pmu.Unlock()
	pd, ok := p.pending[state]
	delete(p.pending, state)
	return pd, ok && time.Now().Before(pd.expires)
}

// lookup follows a dotted path (realm_access.roles) through nested objects.
func lookup(m map[string]any, path string) any {
	if path == "" {
		return nil
	}
	if v, ok := m[path]; ok {
		return v
	}
	var cur any = m
	for part := range strings.SplitSeq(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[part]
	}
	return cur
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func random() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}
