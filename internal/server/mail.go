package server

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"dendronotes/internal/config"
	"dendronotes/internal/mail"
	"dendronotes/internal/store"
)

// Email: handlers put rendered messages into the outbox (state.db); one
// worker hands them to the relay, retrying with growing pauses, and turns
// comment notifications that stay unread into digests. Nothing here blocks
// a request on the relay.

const (
	mailBatch     = 20               // messages per delivery round
	mailPoll      = 30 * time.Second // how often the outbox is checked anyway
	mailGiveUp    = 24 * time.Hour   // after this, a message is dropped
	excerptLength = 300              // characters of a comment in a digest
)

// mailRetry are the pauses after the first, second, … failed attempt; the
// last one repeats.
var mailRetry = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, time.Hour}

type mailer struct {
	cfg    config.Mail
	base   string // public URL without trailing slash
	render *mail.Renderer
	wake   chan struct{}

	mu       sync.Mutex
	sender   mail.Sender
	deliver  sync.Mutex // one delivery round at a time
	lastErr  string
	lastAt   time.Time // last failure
	lastSent time.Time
}

func newMailer(cfg config.Mail, base string) (*mailer, error) {
	r, err := mail.NewRenderer()
	if err != nil {
		return nil, err
	}
	hello := "localhost"
	if u, err := url.Parse(base); err == nil && u.Hostname() != "" {
		hello = u.Hostname()
	}
	return &mailer{
		cfg:    cfg,
		base:   strings.TrimRight(base, "/"),
		render: r,
		wake:   make(chan struct{}, 1),
		sender: &mail.SMTP{
			Host: cfg.Host, Port: cfg.Port, TLS: cfg.TLS, Username: cfg.Username, Password: cfg.Password.Reveal(),
			From: cfg.FromAddr, HelloName: hello,
		},
	}, nil
}

func (m *mailer) getSender() mail.Sender {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sender
}

// setSender replaces the relay (tests).
func (m *mailer) setSender(s mail.Sender) {
	m.mu.Lock()
	m.sender = s
	m.mu.Unlock()
}

// mailEnabled reports whether email is configured.
func (s *Server) mailEnabled() bool { return s.mail != nil }

// queueMail sends an email of a kind to an account: rendered now, delivered
// by the worker. Accounts without an address and disabled ones get nothing.
func (s *Server) queueMail(ctx context.Context, u store.User, kind string, d mail.Data) {
	if s.mail == nil || u.Disabled || u.Email == "" {
		return
	}
	if d.Username == "" {
		d.Username = u.Username
	}
	s.queueMailTo(ctx, u.Email, u.Name(), kind, d)
}

// queueMailTo sends an email to an address (the old one after a change).
func (s *Server) queueMailTo(ctx context.Context, addr, name, kind string, d mail.Data) {
	if s.mail == nil || addr == "" {
		return
	}
	to, err := mail.Recipient(addr)
	if err != nil {
		s.log.Warn("email not sent: invalid address", "kind", kind)
		return
	}
	d.Name, d.Base = name, s.mail.base
	if d.When == "" {
		d.When = time.Now().UTC().Format("Jan 2, 2006, 15:04 UTC")
	}
	msg, err := s.mail.render.Render(kind, to, d)
	if err != nil {
		s.log.Error("rendering email", "kind", kind, "err", err)
		return
	}
	// The request may be over by now; the outbox write must not be.
	ctx = context.WithoutCancel(ctx)
	if _, err := s.store.EnqueueMail(ctx, store.OutMail{Kind: kind, To: msg.To, ToName: msg.ToName, Subject: msg.Subject,
		Text: msg.Text, HTML: msg.HTML}); err != nil {
		s.log.Error("queueing email", "kind", kind, "err", err)
		return
	}
	select {
	case s.mail.wake <- struct{}{}:
	default:
	}
}

// mailLoop delivers the outbox and writes the digests until ctx ends.
func (s *Server) mailLoop(ctx context.Context) {
	poll := time.NewTicker(mailPoll)
	defer poll.Stop()
	digest := time.NewTicker(time.Minute)
	defer digest.Stop()
	for {
		s.deliverMails(ctx)
		select {
		case <-ctx.Done():
			return
		case <-s.mail.wake:
		case <-poll.C:
		case <-digest.C:
			s.sendDigests(ctx, time.Now())
			if err := s.store.CleanPasswordTokens(ctx); err != nil {
				s.log.Error("cleaning password tokens", "err", err)
			}
		}
	}
}

// deliverMails hands every due message to the relay.
func (s *Server) deliverMails(ctx context.Context) {
	m := s.mail
	m.deliver.Lock()
	defer m.deliver.Unlock()
	for ctx.Err() == nil {
		due, err := s.store.DueMails(ctx, mailBatch)
		if err != nil {
			s.log.Error("reading the outbox", "err", err)
			return
		}
		if len(due) == 0 {
			return
		}
		for _, om := range due {
			if ctx.Err() != nil {
				return
			}
			s.deliverOne(ctx, om)
		}
		if len(due) < mailBatch {
			return
		}
	}
}

func (s *Server) deliverOne(ctx context.Context, om store.OutMail) {
	m := s.mail
	raw, err := mail.Build(m.cfg.FromAddr, mail.Message{To: om.To, ToName: om.ToName, Subject: om.Subject, Text: om.Text, HTML: om.HTML}, time.Now())
	if err == nil {
		err = m.getSender().Send(ctx, om.To, raw)
	}
	if err == nil {
		if err := s.store.MailDone(ctx, om.ID); err != nil {
			s.log.Error("removing a sent email", "err", err)
		}
		m.mu.Lock()
		m.lastSent = time.Now()
		m.mu.Unlock()
		s.metrics.mails.Inc(om.Kind, "sent")
		s.log.Info("email sent", "kind", om.Kind, "id", om.ID)
		return
	}
	if ctx.Err() != nil {
		return // shutting down: try again after the restart
	}
	m.mu.Lock()
	m.lastErr, m.lastAt = err.Error(), time.Now()
	m.mu.Unlock()
	if mail.IsPermanent(err) || time.Since(om.CreatedAt) > mailGiveUp {
		s.log.Error("email given up", "kind", om.Kind, "id", om.ID, "attempts", om.Attempts+1, "err", err)
		s.metrics.mails.Inc(om.Kind, "dropped")
		if err := s.store.MailDone(ctx, om.ID); err != nil {
			s.log.Error("removing an email", "err", err)
		}
		return
	}
	pause := mailRetry[min(om.Attempts, len(mailRetry)-1)]
	s.log.Warn("email not sent, will retry", "kind", om.Kind, "id", om.ID, "in", pause.String(), "err", err)
	s.metrics.mails.Inc(om.Kind, "failed")
	if err := s.store.MailRetry(ctx, om.ID, time.Now().Add(pause), err.Error()); err != nil {
		s.log.Error("recording an email failure", "err", err)
	}
}

// sendDigests mails the comment notifications that stayed unread for the
// configured delay, one message per account.
func (s *Server) sendDigests(ctx context.Context, now time.Time) {
	before := now.Add(-s.mail.cfg.DigestDelay)
	entries, err := s.store.DueDigests(ctx, before)
	if err != nil {
		s.log.Error("reading notifications for email", "err", err)
		return
	}
	for i := 0; i < len(entries); {
		j := i
		for j < len(entries) && entries[j].UserID == entries[i].UserID {
			j++
		}
		s.sendDigest(ctx, entries[i:j])
		i = j
	}
	if err := s.store.DigestsHandled(ctx, before); err != nil {
		s.log.Error("marking notifications as mailed", "err", err)
	}
}

func (s *Server) sendDigest(ctx context.Context, entries []store.DigestEntry) {
	u, err := s.store.UserByID(ctx, entries[0].UserID)
	if err != nil {
		return
	}
	items := make([]mail.Item, 0, len(entries))
	for _, e := range entries {
		excerpt := strings.Join(strings.Fields(e.Body), " ")
		if r := []rune(excerpt); len(r) > excerptLength {
			excerpt = string(r[:excerptLength-1]) + "…"
		}
		actor := e.Actor
		if actor == "" {
			actor = "Someone"
		}
		items = append(items, mail.Item{
			Actor: actor, Mention: e.Kind == store.NotifyMention, Soma: e.SomaName,
			Note: noteTitle(e.Path), Excerpt: excerpt, URL: s.noteLink(e.SomaID, e.Path),
		})
	}
	s.queueMail(ctx, u, mail.KindDigest, mail.Data{Items: items, Settings: true})
}

// noteTitle is a note's name without folders and ".md".
func noteTitle(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	return strings.TrimSuffix(p, ".md")
}

// somaLink and noteLink are the addresses the app opens (see router.svelte.ts).
func (s *Server) somaLink(soma string) string {
	return s.mail.base + "/s/" + url.PathEscape(soma)
}

func (s *Server) noteLink(soma, p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return s.somaLink(soma) + "/n/" + strings.Join(parts, "/")
}

// mailStatus is the email part of the administration area.
type mailStatus struct {
	Enabled   bool   `json:"enabled"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	TLS       string `json:"tls,omitempty"`
	From      string `json:"from,omitempty"`
	Waiting   int    `json:"waiting"`
	LastError string `json:"lastError,omitempty"`
	LastErrAt string `json:"lastErrorAt,omitempty"`
	LastSent  string `json:"lastSentAt,omitempty"`
}

func (s *Server) mailStatusOf(ctx context.Context) mailStatus {
	if s.mail == nil {
		return mailStatus{}
	}
	m := s.mail
	st := mailStatus{Enabled: true, Host: m.cfg.Host, Port: m.cfg.Port, TLS: m.cfg.TLS, From: m.cfg.From}
	waiting, lastErr, err := s.store.MailQueue(ctx)
	if err == nil {
		st.Waiting, st.LastError = waiting, lastErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if st.LastError == "" && m.lastErr != "" && m.lastAt.After(m.lastSent) {
		st.LastError = m.lastErr
	}
	if !m.lastAt.IsZero() && st.LastError != "" {
		st.LastErrAt = m.lastAt.UTC().Format(time.RFC3339)
	}
	if !m.lastSent.IsZero() {
		st.LastSent = m.lastSent.UTC().Format(time.RFC3339)
	}
	return st
}
