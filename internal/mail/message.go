// Package mail writes and sends the emails of dendronotes: messages in
// RFC 5322 form (plain text and HTML), delivered through an SMTP relay.
package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
)

// Message is one email to one person.
type Message struct {
	To      string // the address
	ToName  string // shown with the address; may be empty
	Subject string
	Text    string
	HTML    string // optional; sent as the alternative to Text
}

// ErrHeader rejects values that would break out of their header line.
var ErrHeader = errors.New("mail: line breaks in a header value")

// Recipient checks an address and returns its plain form (addr-spec).
func Recipient(addr string) (string, error) {
	if strings.ContainsAny(addr, "\r\n") {
		return "", ErrHeader
	}
	a, err := mail.ParseAddress(addr)
	if err != nil {
		return "", fmt.Errorf("mail: invalid address %q", addr)
	}
	return a.Address, nil
}

// Build writes the message with all headers; now and the sender's domain
// make the Date and Message-ID.
func Build(from *mail.Address, m Message, now time.Time) ([]byte, error) {
	if strings.ContainsAny(m.ToName, "\r\n") || strings.ContainsAny(m.Subject, "\r\n") {
		return nil, ErrHeader
	}
	to, err := Recipient(m.To)
	if err != nil {
		return nil, err
	}
	domain := "localhost"
	if i := strings.LastIndexByte(from.Address, '@'); i >= 0 {
		domain = from.Address[i+1:]
	}
	var b bytes.Buffer
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", from.String())
	header("To", (&mail.Address{Name: m.ToName, Address: to}).String())
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+random(16)+"@"+domain+">")
	header("MIME-Version", "1.0")
	// Out-of-office replies and the like stay away from the sender.
	header("Auto-Submitted", "auto-generated")
	header("X-Auto-Response-Suppress", "All")
	if m.HTML == "" {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		writeQP(&b, m.Text)
		return b.Bytes(), nil
	}
	boundary := "dn-" + random(12)
	header("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	for _, part := range []struct{ typ, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + part.typ + "; charset=utf-8\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		writeQP(&b, part.body)
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return b.Bytes(), nil
}

// writeQP encodes a body as quoted-printable with CRLF line ends.
func writeQP(b *bytes.Buffer, s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	w := quotedprintable.NewWriter(b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
}

func random(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
