package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
	"dendronotes/internal/store"
)

// Banners: announcements the admins write for the sign-in page and the app
// (store/banners.go). The text is short Markdown, rendered and sanitized
// in the browser (web/src/lib/bannerText.ts).

// shownBanner is what people see: no authors, groups or schedule.
type shownBanner struct {
	ID          int64  `json:"id"`
	Message     string `json:"message"`
	Kind        string `json:"kind"`
	Dismissible bool   `json:"dismissible"`
	Revision    int64  `json:"revision"`
}

func shown(list []store.Banner) []shownBanner {
	out := make([]shownBanner, len(list))
	for i, b := range list {
		out[i] = shownBanner{ID: b.ID, Message: b.Message, Kind: b.Kind, Dismissible: b.Dismissible, Revision: b.Revision}
	}
	return out
}

// bannersChanged tells every open tab to load its banners again.
func (s *Server) bannersChanged() {
	for _, rt := range s.somas.Running() {
		rt.Hub.Publish(events.Event{Name: "banners", Data: map[string]bool{"changed": true}})
	}
}

// handleLoginBanners lists the banners of the sign-in page (public).
func (s *Server) handleLoginBanners(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.LoginBanners(r.Context(), time.Now())
	if err != nil {
		s.internalError(w, r, "listing banners", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"banners": shown(list)})
}

// handleMyBanners lists the banners the account sees in the app now.
func (s *Server) handleMyBanners(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFrom(r.Context())
	list, err := s.store.ActiveBanners(r.Context(), me, time.Now())
	if err != nil {
		s.internalError(w, r, "listing banners", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"banners": shown(list)})
}

// handleDismissBanner closes a banner for the account (until its text
// changes).
func (s *Server) handleDismissBanner(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	switch err := s.store.DismissBanner(r.Context(), id, me.ID); {
	case errors.Is(err, store.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, store.ErrNotDismissible):
		httpx.Error(w, http.StatusBadRequest, "not_dismissible")
	case err != nil:
		s.internalError(w, r, "closing banner", err)
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// --- Administration ---------------------------------------------------------

func (s *Server) handleAdminBanners(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.Banners(r.Context())
	if err != nil {
		s.internalError(w, r, "listing banners", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"banners": list})
}

// bannerBody is what the administration sends for a new or changed banner.
type bannerBody struct {
	Message     string     `json:"message"`
	Kind        string     `json:"kind"`
	OnLogin     bool       `json:"onLogin"`
	InApp       bool       `json:"inApp"`
	Audience    string     `json:"audience"`
	GroupID     int64      `json:"groupId"`
	Dismissible bool       `json:"dismissible"`
	StartsAt    *time.Time `json:"startsAt"`
	EndsAt      *time.Time `json:"endsAt"`
}

// readBanner decodes and checks a banner; false means the answer was
// written.
func (s *Server) readBanner(w http.ResponseWriter, r *http.Request) (store.Banner, bool) {
	var body bannerBody
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.WriteDecodeError(w, err)
		return store.Banner{}, false
	}
	bad := func(code string) (store.Banner, bool) {
		httpx.Error(w, http.StatusBadRequest, code)
		return store.Banner{}, false
	}
	msg, ok := store.CleanBannerText(body.Message)
	if !ok {
		return bad("invalid_banner_text")
	}
	b := store.Banner{Message: msg, Kind: body.Kind, OnLogin: body.OnLogin, InApp: body.InApp, Audience: body.Audience,
		Dismissible: body.Dismissible}
	switch b.Kind {
	case store.BannerInfo, store.BannerWarning, store.BannerCritical:
	default:
		return bad("invalid_banner_kind")
	}
	switch b.Audience {
	case store.AudienceEveryone, store.AudienceAdmins:
	case store.AudienceGroup:
		g, err := s.store.GroupByID(r.Context(), body.GroupID)
		if errors.Is(err, store.ErrNotFound) {
			return bad("unknown_group")
		}
		if err != nil {
			s.internalError(w, r, "group lookup", err)
			return store.Banner{}, false
		}
		if g.Everyone {
			b.Audience = store.AudienceEveryone
		} else {
			b.GroupID = g.ID
		}
	default:
		return bad("invalid_banner_audience")
	}
	if !b.OnLogin && !b.InApp {
		return bad("banner_nowhere")
	}
	// Nobody is signed in on the sign-in page: it shows only banners for
	// everyone.
	if b.OnLogin && b.Audience != store.AudienceEveryone {
		return bad("banner_login_audience")
	}
	if body.StartsAt != nil {
		b.StartsAt = body.StartsAt.UTC().Truncate(time.Second)
	}
	if body.EndsAt != nil {
		b.EndsAt = body.EndsAt.UTC().Truncate(time.Second)
	}
	if !b.StartsAt.IsZero() && !b.EndsAt.IsZero() && !b.EndsAt.After(b.StartsAt) {
		return bad("banner_ends_before_start")
	}
	return b, true
}

func (s *Server) handleAdminCreateBanner(w http.ResponseWriter, r *http.Request) {
	b, ok := s.readBanner(w, r)
	if !ok {
		return
	}
	me, _ := auth.UserFrom(r.Context())
	created, err := s.store.CreateBanner(r.Context(), b, me.ID)
	if err != nil {
		s.internalError(w, r, "creating banner", err)
		return
	}
	s.audit(r, "banner.create", strconv.FormatInt(created.ID, 10), created.Kind+", "+created.Audience)
	s.bannersChanged()
	httpx.WriteJSON(w, http.StatusCreated, created)
}

func (s *Server) handleAdminUpdateBanner(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	b, ok := s.readBanner(w, r)
	if !ok {
		return
	}
	b.ID = id
	updated, err := s.store.UpdateBanner(r.Context(), b)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "updating banner", err)
		return
	}
	s.audit(r, "banner.update", strconv.FormatInt(id, 10), updated.Kind+", "+updated.Audience)
	s.bannersChanged()
	httpx.WriteJSON(w, http.StatusOK, updated)
}

func (s *Server) handleAdminDeleteBanner(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.store.DeleteBanner(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	} else if err != nil {
		s.internalError(w, r, "deleting banner", err)
		return
	}
	s.audit(r, "banner.delete", strconv.FormatInt(id, 10), "")
	s.bannersChanged()
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}
