package server

import (
	"context"
	"errors"
	"net/http"

	"dendronotes/internal/auth"
	"dendronotes/internal/httpx"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// access says who may call a route.
type access int

const (
	public          access = iota // anyone
	signedIn                      // any signed-in account
	adminOnly                     // accounts with the admin role
	somaViewer                    // anyone with access to the soma in {soma}
	somaWriter                    // who may change existing notes
	somaContributor               // who may also create notes, folders and files
	somaEditor                    // who may also rename, move and delete
	somaOwner                     // the owner of the soma
)

// somaRole is the minimal member role for the soma access levels.
func (a access) somaRole() string {
	switch a {
	case somaViewer:
		return store.MemberViewer
	case somaWriter:
		return store.MemberWriter
	case somaContributor:
		return store.MemberContributor
	case somaEditor:
		return store.MemberEditor
	case somaOwner:
		return store.MemberOwner
	}
	return ""
}

type somaCtxKey struct{}

// somaCtx is the soma a request works on.
type somaCtx struct {
	soma store.Soma
	role string
	rt   *somas.Runtime
}

// withSoma resolves {soma} for a signed-in account: without access (neither
// personally nor through a group) 404 – a soma's existence is not revealed –,
// with a lower role 403. The role is the effective one. The soma is opened for
// the duration of the request.
func (s *Server) withSoma(min string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		u, _ := auth.UserFrom(ctx)
		id := r.PathValue("soma")
		role, err := s.store.EffectiveRole(ctx, id, u.ID)
		if err != nil {
			s.internalError(w, r, "membership lookup", err)
			return
		}
		if role == "" {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
		if store.RoleRank(role) < store.RoleRank(min) {
			httpx.Error(w, http.StatusForbidden, "forbidden")
			return
		}
		sm, err := s.store.SomaByID(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			s.internalError(w, r, "soma lookup", err)
			return
		}
		rt, err := s.somas.Acquire(sm)
		if errors.Is(err, somas.ErrClosed) {
			httpx.Error(w, http.StatusServiceUnavailable, "shutting_down")
			return
		}
		if err != nil {
			s.internalError(w, r, "opening soma", err)
			return
		}
		defer rt.Release()
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, somaCtxKey{}, somaCtx{soma: sm, role: role, rt: rt})))
	})
}

// runtimeOf returns the open soma of a request behind withSoma.
func runtimeOf(r *http.Request) *somas.Runtime {
	return r.Context().Value(somaCtxKey{}).(somaCtx).rt
}

// somaOf returns the soma and the caller's role in it.
func somaOf(r *http.Request) (store.Soma, string) {
	c := r.Context().Value(somaCtxKey{}).(somaCtx)
	return c.soma, c.role
}
