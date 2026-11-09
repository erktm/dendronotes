package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"dendronotes/internal/auth"
	"dendronotes/internal/httpx"
	"dendronotes/internal/store"
)

// Personal API tokens let scripts use the API as their account: "read"
// tokens only read, "write" tokens also change notes and files. No token
// manages anything – administration, sharing and deleting somas, the
// account itself and tokens stay with signed-in sessions.

// tokenMay reports whether a request with an API token of scope may use rt.
func tokenMay(rt route, method, scope string) bool {
	read := method == http.MethodGet || method == http.MethodHead
	if !read && scope != store.ScopeWrite {
		return false
	}
	switch rt.access {
	case public:
		return true
	case adminOnly, somaOwner:
		return false
	case signedIn:
		// The account's own data only to read; never the tokens.
		return read && !strings.HasPrefix(rt.pattern, "/api/me/tokens")
	}
	return true // somaViewer … somaEditor
}

// tokenGuard refuses what a token may not do (403 token_not_allowed).
func (s *Server) tokenGuard(rt route, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t, ok := auth.TokenFrom(r.Context()); ok && !tokenMay(rt, r.Method, t.Scope) {
			httpx.Error(w, http.StatusForbidden, "token_not_allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFrom(r.Context())
	list, err := s.store.Tokens(r.Context(), me.ID)
	if err != nil {
		s.internalError(w, r, "listing tokens", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tokens": list})
}

// handleCreateToken makes a token: {"name", "scope": "read"|"write",
// "expiresInDays": 0 (never) … 3650}. The token is in the answer only.
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFrom(r.Context())
	var body struct {
		Name          string `json:"name"`
		Scope         string `json:"scope"`
		ExpiresInDays int    `json:"expiresInDays"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_name")
		return
	}
	if body.Scope != store.ScopeRead && body.Scope != store.ScopeWrite {
		httpx.Error(w, http.StatusBadRequest, "invalid_scope")
		return
	}
	if body.ExpiresInDays < 0 || body.ExpiresInDays > 3650 {
		httpx.Error(w, http.StatusBadRequest, "invalid_expiry")
		return
	}
	var expires time.Time
	if body.ExpiresInDays > 0 {
		expires = time.Now().AddDate(0, 0, body.ExpiresInDays)
	}
	token, t, err := s.store.CreateToken(r.Context(), me.ID, name, body.Scope, expires)
	if errors.Is(err, store.ErrTooManyTokens) {
		httpx.Error(w, http.StatusConflict, "too_many_tokens")
		return
	}
	if err != nil {
		s.internalError(w, r, "creating token", err)
		return
	}
	s.audit(r, "token.create", me.Username, t.Name+" ("+t.Scope+")")
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"token": token, "info": t})
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	t, err := s.store.DeleteToken(r.Context(), me.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "revoking token", err)
		return
	}
	s.audit(r, "token.revoke", me.Username, t.Name)
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"id": t.ID})
}

// handleAdminRevokeTokens revokes all tokens of an account.
func (s *Server) handleAdminRevokeTokens(w http.ResponseWriter, r *http.Request) {
	u, ok := s.pathUser(w, r)
	if !ok {
		return
	}
	n, err := s.store.DeleteUserTokens(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, "revoking tokens", err)
		return
	}
	s.audit(r, "user.tokens.revoke", u.Username, strconv.FormatInt(n, 10))
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"revoked": n})
}
