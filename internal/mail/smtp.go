package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"time"
)

// Sender delivers a built message to one recipient.
type Sender interface {
	Send(ctx context.Context, to string, msg []byte) error
}

// TLS modes of the relay connection.
const (
	StartTLS = "starttls" // plain connection upgraded with STARTTLS (port 587)
	Implicit = "tls"      // TLS from the start (port 465)
	NoTLS    = "none"     // unencrypted, only for relays without a password
)

// SMTP sends through a relay such as Mailjet.
type SMTP struct {
	Host     string
	Port     int
	TLS      string
	Username string
	Password string
	From     *mail.Address
	// HelloName is sent with EHLO (the public host name of dendronotes).
	HelloName string
	// Timeout limits a whole delivery; 0 means 30 seconds.
	Timeout time.Duration
	// TLSConfig is for tests (extra root certificates); nil uses the system's.
	TLSConfig *tls.Config
}

// PermanentError is a refusal the relay will repeat (5xx for a recipient or
// the message): retrying does not help.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// IsPermanent reports whether err is a PermanentError.
func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

// Send delivers msg to the address to.
func (s *SMTP) Send(ctx context.Context, to string, msg []byte) error {
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()

	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	d := &net.Dialer{Timeout: timeout}
	var conn net.Conn
	var err error
	if s.TLS == Implicit {
		conn, err = (&tls.Dialer{NetDialer: d, Config: s.tlsConfig()}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", addr, err)
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("greeting from %s: %w", addr, err)
	}
	defer c.Close()
	hello := s.HelloName
	if hello == "" {
		hello = "localhost"
	}
	if err := c.Hello(hello); err != nil {
		return fmt.Errorf("EHLO: %w", err)
	}
	if s.TLS == StartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("%s does not offer STARTTLS", addr)
		}
		if err := c.StartTLS(s.tlsConfig()); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if s.Username != "" {
		// PlainAuth refuses to send the password over a connection without TLS.
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("signing in to %s: %w", addr, err)
		}
	}
	if err := c.Mail(s.From.Address); err != nil {
		// Usually the relay's configuration (sender not confirmed): worth
		// retrying once it is fixed, so never permanent.
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return classify("RCPT TO", err)
	}
	w, err := c.Data()
	if err != nil {
		return classify("DATA", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("sending the message: %w", err)
	}
	if err := w.Close(); err != nil {
		return classify("message", err)
	}
	_ = c.Quit()
	return nil
}

func (s *SMTP) tlsConfig() *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if s.TLSConfig != nil {
		cfg = s.TLSConfig.Clone()
	}
	cfg.ServerName = s.Host
	return cfg
}

// classify marks refusals with a 5xx code as permanent.
func classify(step string, err error) error {
	err = fmt.Errorf("%s: %w", step, err)
	var tp *textproto.Error
	if errors.As(err, &tp) && tp.Code >= 500 {
		return &PermanentError{err}
	}
	return err
}
