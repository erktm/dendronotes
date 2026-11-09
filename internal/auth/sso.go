package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"dendronotes/internal/sso"
	"dendronotes/internal/store"
)

// Sign-in through an OpenID Connect provider. The browser is sent to the
// provider with a random state that is also kept in a short-lived cookie;
// the callback accepts only the state of this very browser (no login CSRF)
// and only once.

const (
	stateCookieSecure   = "__Host-dendronotes_sso"
	stateCookieInsecure = "dendronotes_sso"
	stateCookieMaxAge   = 600
)

// HandleSSOLogin sends the browser to the provider. ?next= is the page to
// return to (a path within dendronotes).
func (a *Authenticator) HandleSSOLogin(w http.ResponseWriter, r *http.Request) {
	if a.cfg.SSO == nil || a.cfg.Mode != ModeLocal {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ip := a.ips.ClientIP(r).String()
	if ok, _ := a.starts.allow(ip); !ok {
		a.loginError(w, r, "rate_limited")
		return
	}
	authURL, state, err := a.cfg.SSO.Begin(safeNext(r.URL.Query().Get("next")))
	if err != nil {
		a.log.Warn("SSO sign-in could not start", "err", err)
		a.loginError(w, r, "sso_unavailable")
		return
	}
	secure := a.secure(r)
	name := stateCookieInsecure
	if secure {
		name = stateCookieSecure
	}
	// Lax, not Strict: the provider's redirect back is a cross-site navigation.
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: state, Path: "/", MaxAge: stateCookieMaxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleSSOCallback finishes the sign-in, finds or creates the account and
// starts a session.
func (a *Authenticator) HandleSSOCallback(w http.ResponseWriter, r *http.Request) {
	if a.cfg.SSO == nil || a.cfg.Mode != ModeLocal {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ip := a.ips.ClientIP(r).String()
	q := r.URL.Query()
	fail := func(code, actor, reason string) {
		a.counted("sso", false)
		a.log.Warn("SSO sign-in failed", "ip", ip, "user", actor, "reason", reason)
		a.store.Audit(r.Context(), store.AuditEntry{Actor: actor, Action: "login.failed", Detail: "sso: " + reason, IP: ip})
		a.loginError(w, r, code)
	}

	var cookieState string
	for _, name := range []string{stateCookieSecure, stateCookieInsecure} {
		if c, err := r.Cookie(name); err == nil && cookieState == "" {
			cookieState = c.Value
		}
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
			Secure: name == stateCookieSecure, SameSite: http.SameSiteLaxMode})
	}
	state := q.Get("state")
	if state == "" || cookieState == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		fail("sso_failed", "", "state does not match this browser")
		return
	}
	if e := q.Get("error"); e != "" {
		a.cfg.SSO.Discard(state)
		code := "sso_failed"
		if e == "access_denied" {
			code = "sso_denied"
		}
		fail(code, "", "provider: "+truncate(e+" "+q.Get("error_description"), 200))
		return
	}
	claims, next, err := a.cfg.SSO.Finish(r.Context(), state, q.Get("code"))
	if errors.Is(err, sso.ErrExpired) {
		fail("sso_expired", "", err.Error())
		return
	}
	if err != nil {
		fail("sso_failed", "", err.Error())
		return
	}
	if len(a.cfg.AllowedGroups) > 0 && !sso.InGroup(claims.Groups, a.cfg.AllowedGroups) {
		fail("sso_forbidden", claims.Username, "not in an allowed group")
		return
	}
	u, err := a.ssoAccount(r.Context(), claims, ip)
	if err != nil {
		a.log.Error("SSO account failed", "err", err)
		a.loginError(w, r, "sso_failed")
		return
	}
	if u.Disabled {
		fail("account_disabled", u.Username, "disabled")
		return
	}
	token, err := a.store.CreateSession(r.Context(), u.ID, a.cfg.SessionTTL, ip, r.UserAgent())
	if err != nil {
		a.log.Error("creating session failed", "err", err)
		a.loginError(w, r, "sso_failed")
		return
	}
	_ = a.store.TouchLogin(r.Context(), u.ID)
	a.setCookie(w, r, token)
	a.log.Info("login", "ip", ip, "user", u.Username, "via", "sso")
	a.counted("sso", true)
	a.store.Audit(r.Context(), store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: "login", Detail: "sso", IP: ip})
	http.Redirect(w, r, next, http.StatusFound)
}

// ssoAccount returns the account of a provider identity, creating it on the
// first sign-in. Name, e-mail and (with admin groups configured) the role
// follow the provider on every sign-in. Accounts are matched by issuer and
// subject only, never by name: a local account "erik" is not taken over by
// a provider user "erik", who gets "erik-2" instead.
func (a *Authenticator) ssoAccount(ctx context.Context, c sso.Claims, ip string) (store.User, error) {
	syncRole := len(a.cfg.AdminGroups) > 0
	return a.externalAccount(ctx, external{
		via: "sso", source: store.SourceOIDC, issuer: c.Issuer, subject: c.Subject,
		username: c.Username, name: c.Name, email: c.Email,
		syncRole: syncRole, admin: syncRole && sso.InGroup(c.Groups, a.cfg.AdminGroups),
		lookup: func(ctx context.Context) (store.User, error) { return a.store.UserByOIDC(ctx, c.Issuer, c.Subject) },
	}, ip)
}

// usernameFrom turns a provider user name into a valid one: characters that
// are not allowed become dashes; at most 60 bytes (room for a suffix).
func usernameFrom(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), strings.ContainsRune("._-@", r):
		default:
			r = '-'
		}
		if b.Len()+utf8.RuneLen(r) > 60 {
			break
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), "-")
	if ValidUsername(out) != nil {
		return ""
	}
	return out
}

// loginError sends the browser back to the sign-in page with an error code.
func (a *Authenticator) loginError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/?login_error="+url.QueryEscape(code), http.StatusFound)
}

// safeNext keeps only paths within dendronotes as targets after sign-in.
func safeNext(next string) string {
	if next == "" || len(next) > 2048 || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") ||
		strings.ContainsAny(next, "\\\r\n\t") || strings.HasPrefix(next, "/api/") {
		return "/"
	}
	if u, err := url.Parse(next); err != nil || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	return next
}

func (a *Authenticator) secure(r *http.Request) bool {
	return a.cfg.CookieSecure == "true" || (a.cfg.CookieSecure != "false" && a.ips.IsHTTPS(r))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
