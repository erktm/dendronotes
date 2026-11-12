package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/httpx"
	"dendronotes/internal/mail"
	"dendronotes/internal/store"
)

// The occasions for email (see mail.go for sending).

// appLink is dendronotes' public address in emails ("" without email).
func (s *Server) appLink() string {
	if s.mail == nil {
		return ""
	}
	return s.mail.base
}

// roleText explains a role in a sharing email.
func roleText(role string) string {
	switch role {
	case store.MemberViewer:
		return "viewer (read and comment)"
	case store.MemberWriter:
		return "writer (read and change notes)"
	case store.MemberContributor:
		return "contributor (also create notes, folders and files)"
	case store.MemberEditor:
		return "editor (also rename, move and delete)"
	case store.MemberOwner:
		return "owner"
	}
	return role
}

// rolesOf returns the current roles of accounts in a soma ("" for none).
func (s *Server) rolesOf(ctx context.Context, somaID string, users []int64) map[int64]string {
	out := make(map[int64]string, len(users))
	if s.mail == nil {
		return out
	}
	for _, id := range users {
		role, err := s.store.EffectiveRole(ctx, somaID, id)
		if err == nil {
			out[id] = role
		}
	}
	return out
}

// groupMemberIDs lists the members of a group; nil for Everyone, which never
// gets sharing emails.
func (s *Server) groupMemberIDs(ctx context.Context, g store.Group) []int64 {
	if s.mail == nil || g.Everyone {
		return nil
	}
	people, err := s.store.GroupMembers(ctx, g.ID)
	if err != nil {
		s.log.Error("reading group members for email", "err", err)
		return nil
	}
	ids := make([]int64, len(people))
	for i, p := range people {
		ids[i] = p.ID
	}
	return ids
}

// mailNewAccess tells everyone who had no access before and has it now
// that the soma was shared with them.
func (s *Server) mailNewAccess(ctx context.Context, actor store.User, sm store.Soma, before map[int64]string) {
	if s.mail == nil {
		return
	}
	for id, was := range before {
		if was != "" || id == actor.ID {
			continue
		}
		role, err := s.store.EffectiveRole(ctx, sm.ID, id)
		if err != nil || role == "" {
			continue
		}
		u, err := s.store.UserByID(ctx, id)
		if err != nil || !u.MailShares {
			continue
		}
		s.queueMail(ctx, u, mail.KindShared, mail.Data{
			Actor: actor.Name(), Soma: sm.Name, Role: roleText(role), URL: s.somaLink(sm.ID), Settings: true,
		})
	}
}

// mailPasswordChanged sends the security notice after a password change.
func (s *Server) mailPasswordChanged(ctx context.Context, u store.User, how string, actor *store.User) {
	d := mail.Data{How: how, URL: s.appLink()}
	if actor != nil {
		d.Actor = actor.Name()
	}
	s.queueMail(ctx, u, mail.KindPasswordChanged, d)
}

// mailEmailChanged tells the old address that the account's address
// changed (by the person or an admin).
func (s *Server) mailEmailChanged(ctx context.Context, before, after store.User, actor *store.User) {
	if before.Email == "" || strings.EqualFold(before.Email, after.Email) {
		return
	}
	d := mail.Data{Username: before.Username, NewEmail: after.Email, URL: s.appLink()}
	if actor != nil && actor.ID != before.ID {
		d.Actor = actor.Name()
	}
	s.queueMailTo(ctx, before.Email, before.Name(), mail.KindEmailChanged, d)
}

// sendResetLink is auth.Config.SendResetLink.
func (s *Server) sendResetLink(ctx context.Context, u store.User, token string) {
	s.queueMail(ctx, u, mail.KindReset, mail.Data{URL: s.resetLink(token), Expires: "1 hour"})
}

// onPasswordSet is auth.Config.OnPasswordSet.
func (s *Server) onPasswordSet(ctx context.Context, u store.User, purpose string) {
	how := "reset"
	if purpose == store.TokenWelcome {
		how = "set"
	}
	s.mailPasswordChanged(ctx, u, how, nil)
}

// resetLink is the page that sets a password with a token; the token is in
// the fragment, which never reaches a server log or a Referer header.
func (s *Server) resetLink(token string) string {
	return s.appLink() + "/reset-password#" + token
}

// mailWelcome sends a new account its user name and, without a password,
// a link to choose one.
func (s *Server) mailWelcome(ctx context.Context, u store.User, actor store.User, withLink bool) error {
	d := mail.Data{Actor: actor.Name(), URL: s.appLink()}
	if withLink {
		token, err := s.store.NewPasswordToken(ctx, u.ID, store.TokenWelcome, auth.WelcomeTTL)
		if err != nil {
			return err
		}
		d.SetPasswordURL, d.URL, d.Expires = s.resetLink(token), s.resetLink(token), "7 days"
	}
	s.queueMail(ctx, u, mail.KindWelcome, d)
	return nil
}

// handleAdminTestMail sends a test email to the admin at once and reports
// what the relay said.
func (s *Server) handleAdminTestMail(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFrom(r.Context())
	if s.mail == nil {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	if me.Email == "" {
		httpx.Error(w, http.StatusBadRequest, "no_email")
		return
	}
	to, err := mail.Recipient(me.Email)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "no_email")
		return
	}
	msg, err := s.mail.render.Render(mail.KindTest, to, mail.Data{
		Name: me.Name(), Username: me.Username, Base: s.mail.base, URL: s.mail.base, Host: s.mail.cfg.Host,
		When: time.Now().UTC().Format("Jan 2, 2006, 15:04 UTC"),
	})
	if err == nil {
		var raw []byte
		if raw, err = mail.Build(s.mail.cfg.FromAddr, msg, time.Now()); err == nil {
			err = s.mail.getSender().Send(r.Context(), to, raw)
		}
	}
	if err != nil {
		s.log.Warn("test email failed", "err", err)
		s.mail.mu.Lock()
		s.mail.lastErr, s.mail.lastAt = err.Error(), time.Now()
		s.mail.mu.Unlock()
		httpx.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "mail_failed", "detail": err.Error()})
		return
	}
	s.mail.mu.Lock()
	s.mail.lastSent = time.Now()
	s.mail.mu.Unlock()
	s.metrics.mails.Inc(mail.KindTest, "sent")
	s.audit(r, "mail.test", me.Username, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
