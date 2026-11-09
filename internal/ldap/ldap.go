// Package ldap checks sign-ins against Active Directory (or another LDAP
// directory): a read-only service account finds the account, a bind with the
// person's password proves it, and the groups the account is in (nested ones
// too, with the default Active Directory filter) come along.
package ldap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	goldap "github.com/go-ldap/ldap/v3"

	"dendronotes/internal/config"
)

// timeout bounds connecting and every request to the directory.
const timeout = 10 * time.Second

var (
	// ErrInvalidCredentials means no single account matched or the password
	// was wrong (deliberately not told apart).
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnavailable wraps everything else: network, TLS, service account.
	ErrUnavailable = errors.New("directory unavailable")
)

// Entry is an account found in the directory.
type Entry struct {
	ID       string // stable id: objectGUID (formatted), entryUUID, else the DN
	DN       string
	Username string
	Name     string
	Email    string
	Groups   []Group // only when groups are configured
}

// Group is a group an account is in.
type Group struct {
	ID   string
	DN   string
	Name string
	// Synced is true for groups below LDAP_GROUP_BASE_DN, which become
	// dendronotes groups.
	Synced bool
}

// InAny reports whether the account is in one of the groups, given by name
// (case-insensitive) or by DN.
func (e Entry) InAny(groups []string) bool {
	for _, want := range groups {
		wantDN, dnErr := goldap.ParseDN(want)
		for _, g := range e.Groups {
			if strings.EqualFold(g.Name, want) {
				return true
			}
			if dnErr == nil && strings.Contains(want, "=") {
				if have, err := goldap.ParseDN(g.DN); err == nil && have.EqualFold(wantDN) {
					return true
				}
			}
		}
	}
	return false
}

// Client talks to one directory.
type Client struct {
	cfg config.LDAP
	tls *tls.Config
}

// New prepares a client (it connects on use).
func New(cfg config.LDAP) (*Client, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if len(cfg.CAPEM) > 0 && !pool.AppendCertsFromPEM(cfg.CAPEM) {
		return nil, errors.New("LDAP CA file contains no certificates")
	}
	return &Client{cfg: cfg, tls: &tls.Config{ServerName: u.Hostname(), RootCAs: pool, MinVersion: tls.VersionTLS12}}, nil
}

// dial connects, encrypts (ldaps or StartTLS) and binds as the service
// account. The connection is closed when ctx ends.
func (c *Client) dial(ctx context.Context) (*goldap.Conn, func(), error) {
	conn, err := goldap.DialURL(c.cfg.URL, goldap.DialWithTLSConfig(c.tls), goldap.DialWithDialer(&net.Dialer{Timeout: timeout}))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	conn.SetTimeout(timeout)
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()
	done := func() {
		close(stop)
		conn.Close()
	}
	if c.cfg.StartTLS {
		if err := conn.StartTLS(c.tls); err != nil {
			done()
			return nil, nil, fmt.Errorf("%w: StartTLS: %v", ErrUnavailable, err)
		}
	}
	if err := c.bindService(conn); err != nil {
		done()
		return nil, nil, err
	}
	return conn, done, nil
}

func (c *Client) bindService(conn *goldap.Conn) error {
	if c.cfg.BindDN == "" {
		return nil // anonymous search
	}
	if err := conn.Bind(c.cfg.BindDN, c.cfg.BindPassword.Reveal()); err != nil {
		return fmt.Errorf("%w: service account: %v", ErrUnavailable, err)
	}
	return nil
}

// Check connects with the service account and reads the base DN (for the
// administration area).
func (c *Client) Check(ctx context.Context) error {
	conn, done, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer done()
	_, err = conn.Search(goldap.NewSearchRequest(c.cfg.BaseDN, goldap.ScopeBaseObject, goldap.NeverDerefAliases,
		1, int(timeout.Seconds()), false, "(objectClass=*)", []string{"dn"}, nil))
	if err != nil {
		return fmt.Errorf("%w: base DN: %v", ErrUnavailable, err)
	}
	return nil
}

// Authenticate finds the account for login (user name, UPN or e-mail with
// the default filter) and checks its password with a bind.
func (c *Client) Authenticate(ctx context.Context, login, password string) (Entry, error) {
	// An empty password would be an "unauthenticated bind", which succeeds.
	if login == "" || password == "" || !utf8.ValidString(login) {
		return Entry{}, ErrInvalidCredentials
	}
	conn, done, err := c.dial(ctx)
	if err != nil {
		return Entry{}, err
	}
	defer done()

	filter := strings.ReplaceAll(c.cfg.UserFilter, "{login}", goldap.EscapeFilter(login))
	attrs := []string{c.cfg.UsernameAttr, c.cfg.NameAttr, c.cfg.EmailAttr, c.cfg.IDAttr}
	res, err := conn.Search(goldap.NewSearchRequest(c.cfg.BaseDN, goldap.ScopeWholeSubtree, goldap.NeverDerefAliases,
		2, int(timeout.Seconds()), false, filter, attrs, nil))
	switch {
	case goldap.IsErrorWithCode(err, goldap.LDAPResultSizeLimitExceeded):
		return Entry{}, ErrInvalidCredentials // ambiguous
	case err != nil:
		return Entry{}, fmt.Errorf("%w: search: %v", ErrUnavailable, err)
	case len(res.Entries) != 1:
		return Entry{}, ErrInvalidCredentials
	}
	found := res.Entries[0]

	if err := conn.Bind(found.DN, password); err != nil {
		if goldap.IsErrorWithCode(err, goldap.LDAPResultInvalidCredentials) {
			return Entry{}, ErrInvalidCredentials
		}
		return Entry{}, fmt.Errorf("%w: bind: %v", ErrUnavailable, err)
	}
	e := Entry{
		ID:       c.id(found),
		DN:       found.DN,
		Username: found.GetAttributeValue(c.cfg.UsernameAttr),
		Name:     found.GetAttributeValue(c.cfg.NameAttr),
		Email:    found.GetAttributeValue(c.cfg.EmailAttr),
	}
	if !c.cfg.NeedsGroups() {
		return e, nil
	}
	// Back to the service account: the person may not see the groups.
	if err := c.bindService(conn); err != nil {
		return Entry{}, err
	}
	e.Groups, err = c.groups(conn, found.DN)
	return e, err
}

// groups lists the groups of an account (below the base DN).
func (c *Client) groups(conn *goldap.Conn, dn string) ([]Group, error) {
	var base *goldap.DN
	if c.cfg.GroupBaseDN != "" {
		var err error
		if base, err = goldap.ParseDN(c.cfg.GroupBaseDN); err != nil {
			return nil, fmt.Errorf("%w: group base DN: %v", ErrUnavailable, err)
		}
	}
	filter := strings.ReplaceAll(c.cfg.GroupFilter, "{dn}", goldap.EscapeFilter(dn))
	res, err := conn.SearchWithPaging(goldap.NewSearchRequest(c.cfg.BaseDN, goldap.ScopeWholeSubtree, goldap.NeverDerefAliases,
		0, int(timeout.Seconds()), false, filter, []string{c.cfg.GroupNameAttr, c.cfg.IDAttr}, nil), 500)
	if err != nil {
		return nil, fmt.Errorf("%w: groups: %v", ErrUnavailable, err)
	}
	out := make([]Group, 0, len(res.Entries))
	for _, g := range res.Entries {
		name := g.GetAttributeValue(c.cfg.GroupNameAttr)
		if name == "" {
			continue
		}
		synced := false
		if base != nil {
			if parsed, err := goldap.ParseDN(g.DN); err == nil {
				synced = base.AncestorOfFold(parsed) || base.EqualFold(parsed)
			}
		}
		out = append(out, Group{ID: c.id(g), DN: g.DN, Name: name, Synced: synced})
	}
	return out, nil
}

// id is the stable id of an entry: objectGUID as text, another id attribute
// as it is, else the DN.
func (c *Client) id(e *goldap.Entry) string {
	raw := e.GetRawAttributeValue(c.cfg.IDAttr)
	switch {
	case len(raw) == 16 && strings.EqualFold(c.cfg.IDAttr, "objectGUID"):
		return guid(raw)
	case len(raw) > 0 && utf8.Valid(raw):
		return string(raw)
	case len(raw) > 0:
		return fmt.Sprintf("%x", raw)
	}
	return strings.ToLower(e.DN)
}

// guid formats an Active Directory objectGUID (the first three parts are
// little-endian) like Windows shows it.
func guid(b []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%x-%x",
		binary.LittleEndian.Uint32(b[0:4]), binary.LittleEndian.Uint16(b[4:6]), binary.LittleEndian.Uint16(b[6:8]), b[8:10], b[10:16])
}
