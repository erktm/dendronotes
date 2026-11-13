package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/httpx"
	"dendronotes/internal/mail"
	"dendronotes/internal/store"
)

// People can create their own accounts when an admin allows it: the account
// exists only after the e-mail address was confirmed with the link sent to
// it. Until then nothing is reserved; the answer to a sign-up never tells
// whether the address already has an account (that address gets an e-mail
// saying so instead).

// signupTTL is how long a confirmation link works.
const signupTTL = 24 * time.Hour

// signupPossible reports whether this server can offer sign-ups at all:
// e-mail is configured and everyone signs in with local passwords.
func (s *Server) signupPossible() bool {
	return s.mailEnabled() && s.cfg.Auth.Mode == string(auth.ModeLocal) && s.cfg.Auth.LocalLogin
}

// signupOpen reports whether sign-ups are open now.
func (s *Server) signupOpen(ctx context.Context) (store.SignupSettings, bool) {
	if !s.signupPossible() {
		return store.SignupSettings{}, false
	}
	v, err := s.store.Signup(ctx)
	if err != nil {
		s.log.Error("reading sign-up settings", "err", err)
		return v, false
	}
	return v, v.Open
}

// handleSignup takes a sign-up and sends the link that confirms it.
func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	settings, open := s.signupOpen(r.Context())
	if !open {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	var body struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Email       string `json:"email"`
		Password    string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	name := strings.TrimSpace(body.Username)
	if auth.ValidUsername(name) != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_username")
		return
	}
	up, ok := profileUpdate(w, &body.DisplayName, &body.Email)
	if !ok {
		return
	}
	email := *up.Email
	if email == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_email")
		return
	}
	if auth.ValidPassword(body.Password) != nil {
		httpx.Error(w, http.StatusBadRequest, "weak_password")
		return
	}
	if !settings.Allows(email) {
		httpx.Error(w, http.StatusBadRequest, "domain_not_allowed")
		return
	}
	if !s.auth.LimitSignup(w, r, email) {
		return
	}
	// User names are visible in the app anyway; addresses are not.
	if _, err := s.store.UserByUsername(r.Context(), name); err == nil {
		httpx.Error(w, http.StatusConflict, "username_taken")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		s.internalError(w, r, "user lookup", err)
		return
	}
	accounts, err := s.store.UsersByEmail(r.Context(), email)
	if err != nil {
		s.internalError(w, r, "user lookup", err)
		return
	}
	if len(accounts) > 0 {
		names := make([]string, 0, len(accounts))
		for _, u := range accounts {
			names = append(names, u.Username)
		}
		s.queueMailTo(r.Context(), email, accounts[0].Name(), mail.KindSignupExists,
			mail.Data{Username: strings.Join(names, ", "), URL: s.appLink()})
		s.log.Info("sign-up for an address that has an account")
		httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
		return
	}
	hash, err := s.auth.HashNew(body.Password)
	if err != nil {
		s.internalError(w, r, "hashing password", err)
		return
	}
	display := *up.DisplayName
	token, err := s.store.AddPendingSignup(r.Context(), store.PendingSignup{
		Username: name, DisplayName: display, Email: email, PasswordHash: hash,
	}, signupTTL)
	if err != nil {
		s.internalError(w, r, "storing sign-up", err)
		return
	}
	greet := display
	if greet == "" {
		greet = name
	}
	s.queueMailTo(r.Context(), email, greet, mail.KindSignupConfirm, mail.Data{
		Username: name, URL: s.appLink() + "/confirm-account#" + token, Expires: "24 hours",
	})
	s.log.Info("sign-up waiting for confirmation", "user", name)
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
}

// handleConfirmSignup creates the account of a confirmed sign-up with its
// first soma and signs it in.
func (s *Server) handleConfirmSignup(w http.ResponseWriter, r *http.Request) {
	if _, open := s.signupOpen(r.Context()); !open {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	if !s.auth.LimitSignup(w, r, "") {
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	p, err := s.store.TakePendingSignup(r.Context(), body.Token)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusBadRequest, "invalid_token")
		return
	}
	if err != nil {
		s.internalError(w, r, "reading sign-up", err)
		return
	}
	// Someone may have taken the name or the address meanwhile.
	if others, err := s.store.UsersByEmail(r.Context(), p.Email); err != nil {
		s.internalError(w, r, "user lookup", err)
		return
	} else if len(others) > 0 {
		httpx.Error(w, http.StatusConflict, "email_taken")
		return
	}
	u, err := s.store.CreateUser(r.Context(), store.User{
		Username: p.Username, DisplayName: p.DisplayName, Email: p.Email,
		Role: store.RoleUser, Source: store.SourceLocal, PasswordHash: p.PasswordHash,
	})
	if errors.Is(err, store.ErrTaken) {
		httpx.Error(w, http.StatusConflict, "username_taken")
		return
	}
	if err != nil {
		s.internalError(w, r, "creating user", err)
		return
	}
	s.store.Audit(r.Context(), store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: "user.signup", Target: u.Username,
		IP: s.ips.ClientIP(r).String()})
	_ = s.welcome(r.Context(), u)
	s.auth.StartSession(w, r, u)
}

// handleAdminSignup shows the sign-up settings and whether they can apply.
func (s *Server) handleAdminSignup(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Signup(r.Context())
	if err != nil {
		s.internalError(w, r, "reading sign-up settings", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"open": v.Open, "domains": v.Domains, "possible": s.signupPossible(),
		"mail": s.mailEnabled(), "localLogin": s.cfg.Auth.Mode == string(auth.ModeLocal) && s.cfg.Auth.LocalLogin,
	})
}

// handleAdminSetSignup opens or closes sign-ups and sets the allowed domains.
func (s *Server) handleAdminSetSignup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Open    bool     `json:"open"`
		Domains []string `json:"domains"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if body.Open && !s.signupPossible() {
		httpx.Error(w, http.StatusBadRequest, "signup_not_possible")
		return
	}
	if len(body.Domains) > store.MaxSignupDomains {
		httpx.Error(w, http.StatusBadRequest, "invalid_domain")
		return
	}
	domains := []string{}
	seen := map[string]bool{}
	for _, d := range body.Domains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d == "" || seen[d] {
			continue
		}
		if len(d) > 253 || !strings.Contains(d, ".") || strings.ContainsAny(d, " @/\\\t\r\n<>") {
			httpx.Error(w, http.StatusBadRequest, "invalid_domain")
			return
		}
		seen[d] = true
		domains = append(domains, d)
	}
	if err := s.store.SetSignup(r.Context(), store.SignupSettings{Open: body.Open, Domains: domains}); err != nil {
		s.internalError(w, r, "storing sign-up settings", err)
		return
	}
	detail := "closed"
	if body.Open {
		detail = "open " + strings.Join(domains, ", ")
	}
	s.audit(r, "settings.signup", "", strings.TrimSpace(detail))
	s.handleAdminSignup(w, r)
}
