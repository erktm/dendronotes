package server

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/time/rate"

	"dendronotes/internal/auth"
	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
	"dendronotes/internal/soma"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// Comments on notes. Everyone with access to a soma comments and answers;
// authors change and delete their own comments, writers mark threads as
// done, editors delete any comment. @user name in a comment notifies that
// account if it has access to the soma.

const (
	maxCommentRunes = 5000
	maxQuoteRunes   = 2000
	maxContextRunes = 64
	maxMentions     = 20
)

// commentLimiter allows bursts of comments but not floods (per account).
func (s *Server) commentLimiter(userID int64) *rate.Limiter {
	l, _ := s.commentLimits.LoadOrStore(userID, rate.NewLimiter(rate.Every(2*time.Second), 20))
	return l.(*rate.Limiter)
}

// cleanCommentText trims a comment and checks its length and characters.
func cleanCommentText(body string, maxRunes int, emptyOK bool) (string, bool) {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	n := utf8.RuneCountInString(body)
	if !utf8.ValidString(body) || n > maxRunes || (n == 0 && !emptyOK) {
		return "", false
	}
	for _, r := range body {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", false
		}
	}
	return body, true
}

// anchorText checks the quote and its context; they are kept as they are
// (the client looks for them in the note).
func anchorText(s string, maxRunes int) (string, bool) {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxRunes || strings.ContainsRune(s, 0) {
		return "", false
	}
	return s, true
}

// mentions returns the user names written as @name in a comment.
func mentions(body string) []string {
	var out []string
	rs := []rune(body)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '@' || (i > 0 && isNameRune(rs[i-1])) || i+1 >= len(rs) ||
			!(unicode.IsLetter(rs[i+1]) || unicode.IsDigit(rs[i+1])) {
			continue
		}
		j := i + 1
		for j < len(rs) && isNameRune(rs[j]) {
			j++
		}
		name := strings.TrimRight(string(rs[i+1:j]), ".-@_")
		if utf8.RuneCountInString(name) >= 2 && !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, name) }) {
			out = append(out, name)
		}
		i = j - 1
	}
	if len(out) > maxMentions {
		out = out[:maxMentions]
	}
	return out
}

func isNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._-@", r)
}

// commentsChanged tells the clients of a soma that the comments on a note
// changed (a hint: they reload).
func commentsChanged(rt *somas.Runtime, path string) {
	rt.Hub.Publish(events.Event{Name: "comments", Data: map[string]string{"path": path}})
}

// notifyLive tells the open tabs of these accounts (in any soma) that they
// have something new.
func (s *Server) notifyLive(users []int64) {
	for _, rt := range s.somas.Running() {
		for _, u := range users {
			if ids := rt.Collab.StreamsOf(u); len(ids) > 0 {
				rt.Hub.Send(ids, events.Event{Name: "notification", Data: map[string]int64{"user": u}})
			}
		}
	}
}

// commentPeople are the authors of the comments, for names and pictures.
func (s *Server) commentPeople(ctx context.Context, list []store.Comment) map[int64]store.Person {
	var ids []int64
	for _, c := range list {
		if c.AuthorID != 0 {
			ids = append(ids, c.AuthorID)
		}
		if c.ResolvedBy != 0 {
			ids = append(ids, c.ResolvedBy)
		}
	}
	return s.people(ctx, ids)
}

func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	p, err := soma.CleanPath(r.URL.Query().Get("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	list, err := s.store.Comments(r.Context(), rt.ID, p)
	if err != nil {
		s.internalError(w, r, "listing comments", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"comments": list, "people": s.commentPeople(r.Context(), list)})
}

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	me, _ := auth.UserFrom(r.Context())
	var body struct {
		Path     string `json:"path"`
		Body     string `json:"body"`
		Quote    string `json:"quote"`
		Prefix   string `json:"prefix"`
		Suffix   string `json:"suffix"`
		ThreadID int64  `json:"threadId"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 64<<10); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	text, ok := cleanCommentText(body.Body, maxCommentRunes, false)
	quote, ok2 := anchorText(body.Quote, maxQuoteRunes)
	prefix, ok3 := anchorText(body.Prefix, maxContextRunes)
	suffix, ok4 := anchorText(body.Suffix, maxContextRunes)
	if !ok || !ok2 || !ok3 || !ok4 {
		httpx.Error(w, http.StatusBadRequest, "invalid_comment")
		return
	}
	if !s.commentLimiter(me.ID).Allow() {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	c := store.Comment{SomaID: rt.ID, AuthorID: me.ID, Body: text}
	if body.ThreadID != 0 {
		// A reply: it belongs where the thread is; the anchor is the thread's.
		root, err := s.store.CommentByID(r.Context(), rt.ID, body.ThreadID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && root.ThreadID != 0) {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			s.internalError(w, r, "reading thread", err)
			return
		}
		c.ThreadID, c.Path = root.ID, root.Path
	} else {
		p, err := soma.CleanPath(body.Path)
		if err != nil {
			s.somaError(w, r, err)
			return
		}
		if e, err := rt.Soma.Stat(p); err != nil || e.Type != "file" || !soma.IsNote(p) {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
		c.Path, c.Quote, c.Prefix, c.Suffix = p, quote, prefix, suffix
	}
	c, err := s.store.AddComment(r.Context(), c)
	if err != nil {
		s.internalError(w, r, "adding comment", err)
		return
	}
	s.notifyAbout(r.Context(), rt, c, me.ID)
	commentsChanged(rt, c.Path)
	httpx.WriteJSON(w, http.StatusCreated, c)
}

// notifyAbout tells the mentioned accounts with access (and, for replies,
// the others in the thread) about a new comment.
func (s *Server) notifyAbout(ctx context.Context, rt *somas.Runtime, c store.Comment, actor int64) {
	var mentioned []int64
	for _, name := range mentions(c.Body) {
		u, err := s.store.UserByUsername(ctx, name)
		if err != nil || u.ID == actor || u.Disabled || slices.Contains(mentioned, u.ID) {
			continue
		}
		if role, err := s.store.EffectiveRole(ctx, rt.ID, u.ID); err != nil || role == "" {
			continue // no access: no notification
		}
		mentioned = append(mentioned, u.ID)
	}
	var answered []int64
	if c.ThreadID != 0 {
		authors, err := s.store.ThreadAuthors(ctx, rt.ID, c.ThreadID)
		if err != nil {
			s.log.Error("reading thread authors", "err", err)
		}
		for _, u := range authors {
			if u == actor || slices.Contains(mentioned, u) {
				continue
			}
			if role, err := s.store.EffectiveRole(ctx, rt.ID, u); err == nil && role != "" {
				answered = append(answered, u)
			}
		}
	}
	for kind, users := range map[string][]int64{store.NotifyMention: mentioned, store.NotifyReply: answered} {
		if len(users) == 0 {
			continue
		}
		if err := s.store.Notify(ctx, kind, users, c, actor); err != nil {
			s.log.Error("storing notifications", "err", err)
		}
	}
	s.notifyLive(append(mentioned, answered...))
}

// pathComment reads the comment {id} of the soma.
func (s *Server) pathComment(w http.ResponseWriter, r *http.Request) (store.Comment, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return store.Comment{}, false
	}
	c, err := s.store.CommentByID(r.Context(), runtimeOf(r).ID, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && strings.HasPrefix(c.Path, soma.TrashDir+"/")) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return store.Comment{}, false
	}
	if err != nil {
		s.internalError(w, r, "reading comment", err)
		return store.Comment{}, false
	}
	return c, true
}

func (s *Server) handleEditComment(w http.ResponseWriter, r *http.Request) {
	c, ok := s.pathComment(w, r)
	if !ok {
		return
	}
	if me, _ := auth.UserFrom(r.Context()); c.AuthorID != me.ID {
		httpx.Error(w, http.StatusForbidden, "not_author")
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 64<<10); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	text, ok := cleanCommentText(body.Body, maxCommentRunes, false)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid_comment")
		return
	}
	c, err := s.store.EditComment(r.Context(), c.SomaID, c.ID, text)
	if err != nil {
		s.internalError(w, r, "editing comment", err)
		return
	}
	commentsChanged(runtimeOf(r), c.Path)
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	c, ok := s.pathComment(w, r)
	if !ok {
		return
	}
	me, _ := auth.UserFrom(r.Context())
	_, role := somaOf(r)
	if c.AuthorID != me.ID && store.RoleRank(role) < store.RoleRank("editor") {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.store.DeleteComment(r.Context(), c.SomaID, c.ID); err != nil {
		s.internalError(w, r, "deleting comment", err)
		return
	}
	commentsChanged(runtimeOf(r), c.Path)
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"id": c.ID})
}

// handleResolveComment marks a thread as done ({"resolved": true}) or open.
func (s *Server) handleResolveComment(w http.ResponseWriter, r *http.Request) {
	c, ok := s.pathComment(w, r)
	if !ok {
		return
	}
	var body struct {
		Resolved bool `json:"resolved"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if c.ThreadID != 0 {
		httpx.Error(w, http.StatusBadRequest, "not_a_thread")
		return
	}
	var by int64
	if body.Resolved {
		me, _ := auth.UserFrom(r.Context())
		by = me.ID
	}
	c, err := s.store.ResolveThread(r.Context(), c.SomaID, c.ID, by)
	if err != nil {
		s.internalError(w, r, "resolving thread", err)
		return
	}
	commentsChanged(runtimeOf(r), c.Path)
	httpx.WriteJSON(w, http.StatusOK, c)
}

// handleSomaPeople lists people with access to the soma (mentions).
func (s *Server) handleSomaPeople(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 64 {
		httpx.Error(w, http.StatusBadRequest, "query_too_long")
		return
	}
	people, err := s.store.SomaPeople(r.Context(), runtimeOf(r).ID, q, 8)
	if err != nil {
		s.internalError(w, r, "listing people", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"people": people})
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFrom(r.Context())
	list, unread, err := s.store.Notifications(r.Context(), me.ID, 50)
	if err != nil {
		s.internalError(w, r, "listing notifications", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"notifications": list, "unread": unread})
}

// handleReadNotifications marks {"ids": […]} (or all without ids) as read.
func (s *Server) handleReadNotifications(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFrom(r.Context())
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if len(body.IDs) > 200 {
		httpx.Error(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.store.MarkNotificationsRead(r.Context(), me.ID, body.IDs); err != nil {
		s.internalError(w, r, "marking notifications", err)
		return
	}
	_, unread, err := s.store.Notifications(r.Context(), me.ID, 0)
	if err != nil {
		s.internalError(w, r, "counting notifications", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"unread": unread})
}
