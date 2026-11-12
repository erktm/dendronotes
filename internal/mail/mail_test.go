package mail

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

var from = &mail.Address{Name: "dendronotes", Address: "no-reply@example.org"}

// --- Messages ---------------------------------------------------------------

func TestBuildMultipart(t *testing.T) {
	m := Message{To: "anna@example.org", ToName: "Anna Bérg", Subject: "Über “Team” – ok", Text: "Hi Anna,\nline two = fine\n", HTML: "<p>Hi</p>"}
	raw, err := Build(from, m, time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("not a valid message: %v\n%s", err, raw)
	}
	dec := new(mime.WordDecoder)
	if s, _ := dec.DecodeHeader(msg.Header.Get("Subject")); s != m.Subject {
		t.Errorf("subject %q", s)
	}
	to, err := msg.Header.AddressList("To")
	if err != nil || len(to) != 1 || to[0].Address != "anna@example.org" || to[0].Name != "Anna Bérg" {
		t.Errorf("to %v %v", to, err)
	}
	for _, h := range []string{"Date", "Message-ID", "From"} {
		if msg.Header.Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	if !strings.HasSuffix(msg.Header.Get("Message-ID"), "@example.org>") {
		t.Errorf("message id %q", msg.Header.Get("Message-ID"))
	}
	if msg.Header.Get("Auto-Submitted") != "auto-generated" {
		t.Error("missing Auto-Submitted")
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var parts []string
	for {
		p, err := mr.NextRawPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(quotedprintable.NewReader(p))
		parts = append(parts, p.Header.Get("Content-Type")+"|"+string(body))
	}
	want := []string{"text/plain; charset=utf-8|Hi Anna,\r\nline two = fine\r\n", "text/html; charset=utf-8|<p>Hi</p>"}
	if len(parts) != 2 || parts[0] != want[0] || parts[1] != want[1] {
		t.Errorf("parts %q", parts)
	}
}

func TestBuildRejectsHeaderInjection(t *testing.T) {
	for _, m := range []Message{
		{To: "a@example.org\r\nBcc: x@example.org", Subject: "s", Text: "t"},
		{To: "a@example.org", ToName: "A\nBcc: x@example.org", Subject: "s", Text: "t"},
		{To: "a@example.org", Subject: "s\r\nBcc: x@example.org", Text: "t"},
		{To: "not an address", Subject: "s", Text: "t"},
	} {
		if _, err := Build(from, m, time.Now()); err == nil {
			t.Errorf("accepted %+v", m)
		}
	}
}

func TestRenderEveryKind(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	evil := `<script>alert(1)</script> & "q"`
	d := Data{
		Name: evil, Username: "anna", Base: "https://notes.example.org", URL: "https://notes.example.org/s/abc",
		When: "Nov 12, 2026, 10:00 UTC", Actor: evil, Soma: evil, Role: "editor",
		Items: []Item{
			{Actor: "Jonas", Mention: true, Soma: "Team", Note: "Plan", Excerpt: evil, URL: "https://notes.example.org/s/abc/n/Plan.md"},
			{Actor: "Mia", Soma: "Team", Note: "Ideas", Excerpt: "short", URL: "https://notes.example.org/s/abc/n/Ideas.md"},
		},
		How: "admin", TokenName: evil, Scope: "read", NewEmail: "new@example.org",
		SetPasswordURL: "https://notes.example.org/reset-password#t", Expires: "7 days", Host: "smtp.example.org",
	}
	for _, k := range Kinds {
		m, err := r.Render(k, "anna@example.org", d)
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		if m.Subject == "" || strings.ContainsAny(m.Subject, "\r\n") {
			t.Errorf("%s: subject %q", k, m.Subject)
		}
		if strings.Contains(m.HTML, "<script>") {
			t.Errorf("%s: HTML not escaped:\n%s", k, m.HTML)
		}
		if !strings.Contains(m.Text, "Hi ") || !strings.Contains(m.HTML, "dendronotes") {
			t.Errorf("%s: incomplete:\n%s", k, m.Text)
		}
		if _, err := Build(from, m, time.Now()); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
	m, _ := r.Render(KindDigest, "anna@example.org", Data{Name: "Anna", Base: "https://x", Items: d.Items[:1]})
	if m.Subject != "Jonas mentioned you in Plan" {
		t.Errorf("digest subject %q", m.Subject)
	}
	m, _ = r.Render(KindDigest, "anna@example.org", Data{Name: "Anna", Base: "https://x", Items: d.Items})
	if m.Subject != "2 new comments for you" {
		t.Errorf("digest subject %q", m.Subject)
	}
	if _, err := r.Render("nope", "a@example.org", d); err == nil {
		t.Error("unknown kind rendered")
	}
}

// --- SMTP -------------------------------------------------------------------

type received struct {
	from, to, user, pass string
	data                 []byte
	tls                  bool
}

// fakeSMTP is just enough of an SMTP server for net/smtp: EHLO, STARTTLS,
// AUTH PLAIN, MAIL, RCPT, DATA, QUIT.
type fakeSMTP struct {
	ln       net.Listener
	cfg      *tls.Config
	implicit bool
	refuse   int // answer RCPT with this code (e.g. 550)
	mu       sync.Mutex
	got      []received
}

func newFakeSMTP(t *testing.T, implicit bool) (*fakeSMTP, *x509.CertPool) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, implicit: implicit, cfg: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f, pool
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	isTLS := false
	if f.implicit {
		conn, isTLS = tls.Server(conn, f.cfg), true
	}
	tp := textproto.NewConn(conn)
	var r received
	_ = tp.PrintfLine("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		cmd, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(cmd) {
		case "EHLO", "HELO":
			_ = tp.PrintfLine("250-fake")
			if !isTLS {
				_ = tp.PrintfLine("250-STARTTLS")
			} else {
				_ = tp.PrintfLine("250-AUTH PLAIN")
			}
			_ = tp.PrintfLine("250 8BITMIME")
		case "STARTTLS":
			_ = tp.PrintfLine("220 go ahead")
			tc := tls.Server(conn, f.cfg)
			if tc.Handshake() != nil {
				return
			}
			conn, isTLS = tc, true
			tp = textproto.NewConn(conn)
		case "AUTH":
			_, b64, _ := strings.Cut(arg, " ")
			raw, _ := base64.StdEncoding.DecodeString(b64)
			fields := strings.Split(string(raw), "\x00")
			if len(fields) == 3 {
				r.user, r.pass = fields[1], fields[2]
			}
			_ = tp.PrintfLine("235 ok")
		case "MAIL":
			r.from = strings.Trim(strings.TrimPrefix(arg, "FROM:"), "<>")
			if i := strings.Index(r.from, ">"); i >= 0 {
				r.from = r.from[:i]
			}
			_ = tp.PrintfLine("250 ok")
		case "RCPT":
			if f.refuse != 0 {
				_ = tp.PrintfLine("%d no such user", f.refuse)
				continue
			}
			r.to = strings.Trim(strings.TrimPrefix(arg, "TO:"), "<>")
			_ = tp.PrintfLine("250 ok")
		case "DATA":
			_ = tp.PrintfLine("354 go")
			r.data, _ = tp.ReadDotBytes()
			r.tls = isTLS
			f.mu.Lock()
			f.got = append(f.got, r)
			f.mu.Unlock()
			_ = tp.PrintfLine("250 queued")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 unknown")
		}
	}
}

func (f *fakeSMTP) messages() []received {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]received(nil), f.got...)
}

func TestSMTPStartTLSAndImplicit(t *testing.T) {
	for _, mode := range []string{StartTLS, Implicit} {
		t.Run(mode, func(t *testing.T) {
			f, pool := newFakeSMTP(t, mode == Implicit)
			s := &SMTP{Host: "127.0.0.1", Port: f.port(), TLS: mode, Username: "key", Password: "secret", From: from,
				HelloName: "notes.example.org", TLSConfig: &tls.Config{RootCAs: pool}}
			raw, _ := Build(from, Message{To: "anna@example.org", Subject: "Hi", Text: "Body"}, time.Now())
			if err := s.Send(context.Background(), "anna@example.org", raw); err != nil {
				t.Fatal(err)
			}
			got := f.messages()
			if len(got) != 1 {
				t.Fatalf("got %d messages", len(got))
			}
			g := got[0]
			if !g.tls || g.user != "key" || g.pass != "secret" || g.from != "no-reply@example.org" || g.to != "anna@example.org" {
				t.Errorf("received %+v", g)
			}
			if !bytes.Contains(g.data, []byte("Subject: Hi")) {
				t.Errorf("data %s", g.data)
			}
		})
	}
}

func TestSMTPRefusesUntrustedCertificate(t *testing.T) {
	f, _ := newFakeSMTP(t, false)
	s := &SMTP{Host: "127.0.0.1", Port: f.port(), TLS: StartTLS, Username: "key", Password: "secret", From: from, Timeout: 5 * time.Second}
	if err := s.Send(context.Background(), "anna@example.org", []byte("x")); err == nil {
		t.Fatal("sent over a certificate nobody trusts")
	}
	if len(f.messages()) != 0 {
		t.Error("message arrived")
	}
}

func TestSMTPPermanentRefusal(t *testing.T) {
	f, pool := newFakeSMTP(t, false)
	f.refuse = 550
	s := &SMTP{Host: "127.0.0.1", Port: f.port(), TLS: StartTLS, From: from, TLSConfig: &tls.Config{RootCAs: pool}}
	err := s.Send(context.Background(), "nobody@example.org", []byte("x"))
	if err == nil || !IsPermanent(err) {
		t.Fatalf("want a permanent error, got %v", err)
	}
	f.refuse = 451
	if err := s.Send(context.Background(), "later@example.org", []byte("x")); err == nil || IsPermanent(err) {
		t.Fatalf("want a temporary error, got %v", err)
	}
}

func TestSMTPUnreachable(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	s := &SMTP{Host: "127.0.0.1", Port: port, TLS: StartTLS, From: from, Timeout: 2 * time.Second}
	if err := s.Send(context.Background(), "a@example.org", []byte("x")); err == nil || IsPermanent(err) {
		t.Fatalf("want a temporary error, got %v", err)
	}
}
