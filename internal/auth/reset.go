package auth

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dendronotes/internal/httpx"
	"dendronotes/internal/store"
)

// "Forgot password": a link by email sets a new password. Only accounts
// that may sign in with a password get one, at most every resetEvery; the
// answer never tells whether an account exists. Tokens work once and
// expire; the store keeps only their hash.

const (
	// ResetTTL is how long a reset link works.
	ResetTTL = time.Hour
	// WelcomeTTL is how long the link in a welcome email works.
	WelcomeTTL = 7 * 24 * time.Hour
	resetEvery = 5 * time.Minute
)

func (a *Authenticator) resetEnabled() bool {
	return a.cfg.Mode == ModeLocal && a.cfg.SendResetLink != nil
}

// mayUsePassword reports whether an account signs in with a password of its
// own in dendronotes.
func (a *Authenticator) mayUsePassword(u store.User) bool {
	if u.Disabled || u.Source != store.SourceLocal {
		return false
	}
	return !a.cfg.PasswordsAdminOnly || strings.EqualFold(u.Username, a.cfg.AdminUsername)
}

// limitResets applies the per-IP limit; false means the answer was written.
func (a *Authenticator) limitResets(w http.ResponseWriter, r *http.Request) bool {
	ip := a.ips.ClientIP(r).String()
	if ok, wait := a.resets.allow(ip); !ok {
		secs := int(math.Ceil(wait.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		a.log.Warn("password reset rate limited", "ip", ip)
		httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retryAfter": secs})
		return false
	}
	return true
}

// HandleForgotPassword sends a reset link to the account with the given
// user name or email address, if there is one that may have it.
func (a *Authenticator) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !a.resetEnabled() {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	var body struct {
		Login string `json:"login"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if !a.limitResets(w, r) {
		return
	}
	login := strings.TrimSpace(body.Login)
	if login != "" && len(login) <= maxLoginLen {
		a.requestReset(r.Context(), login, a.ips.ClientIP(r).String())
	}
	// The same answer for every input.
	httpx.WriteJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (a *Authenticator) requestReset(ctx context.Context, login, ip string) {
	u, err := a.store.UserByLogin(ctx, login)
	if err != nil || !a.mayUsePassword(u) || u.Email == "" {
		return
	}
	if last, err := a.store.LastPasswordToken(ctx, u.ID, store.TokenReset); err != nil || time.Since(last) < resetEvery {
		return
	}
	token, err := a.store.NewPasswordToken(ctx, u.ID, store.TokenReset, ResetTTL)
	if err != nil {
		a.log.Error("creating a reset token", "err", err)
		return
	}
	a.store.Audit(ctx, store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: "password.reset.request", Target: u.Username, IP: ip})
	a.cfg.SendResetLink(ctx, u, token)
}

// HandleResetPassword sets a new password with a token from an email (a
// reset or a welcome link). Every session of the account ends.
func (a *Authenticator) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Mode != ModeLocal {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if !a.limitResets(w, r) {
		return
	}
	// Checked before the token is spent, so that a weak password can be
	// corrected with the same link.
	if err := ValidPassword(body.Password); err != nil {
		httpx.Error(w, http.StatusBadRequest, "weak_password")
		return
	}
	ctx := r.Context()
	ip := a.ips.ClientIP(r).String()
	userID, purpose, err := a.store.UsePasswordToken(ctx, body.Token)
	if errors.Is(err, store.ErrNotFound) {
		a.log.Warn("password reset with an invalid link", "ip", ip)
		httpx.Error(w, http.StatusBadRequest, "invalid_token")
		return
	}
	if err != nil {
		a.log.Error("using a reset token", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error")
		return
	}
	u, err := a.store.UserByID(ctx, userID)
	if err != nil || !a.mayUsePassword(u) {
		httpx.Error(w, http.StatusBadRequest, "invalid_token")
		return
	}
	hash, err := a.HashNew(body.Password)
	if err != nil {
		a.log.Error("hashing a password", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := a.store.UpdateUser(ctx, u.ID, store.UserUpdate{PasswordHash: &hash}); err != nil {
		a.log.Error("setting a password", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error")
		return
	}
	_ = a.store.DropPasswordTokens(ctx, u.ID)
	if _, err := a.store.EndUserSessions(ctx, u.ID, 0); err != nil {
		a.log.Error("ending sessions after a reset", "err", err)
	}
	action := "password.reset"
	if purpose == store.TokenWelcome {
		action = "password.set"
	}
	a.log.Info(action, "ip", ip, "user", u.Username)
	a.store.Audit(ctx, store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: action, Target: u.Username, IP: ip})
	if a.cfg.OnPasswordSet != nil {
		a.cfg.OnPasswordSet(ctx, u, purpose)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"username": u.Username})
}
