package config

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"
)

// Email through an SMTP relay (notifications, security notices, password
// reset). Everything stays off while SMTP_HOST is empty.
const (
	EnvSMTPHost         = Prefix + "SMTP_HOST"
	EnvSMTPPort         = Prefix + "SMTP_PORT"
	EnvSMTPTLS          = Prefix + "SMTP_TLS"
	EnvSMTPUsername     = Prefix + "SMTP_USERNAME"
	EnvSMTPPassword     = Prefix + "SMTP_PASSWORD_FILE"
	EnvMailFrom         = Prefix + "MAIL_FROM"
	EnvMailDigestDelay  = Prefix + "MAIL_DIGEST_DELAY"
	DefaultSMTPPort     = 587
	DefaultSMTPTLS      = "starttls"
	DefaultDigestDelay  = 10 * time.Minute
	SMTPTLSStartTLS     = "starttls"
	SMTPTLSImplicit     = "tls"
	SMTPTLSNone         = "none"
	maxMailFromAddrSize = 254
)

var mailVars = []string{
	EnvSMTPHost, EnvSMTPPort, EnvSMTPTLS, EnvSMTPUsername, EnvSMTPPassword, EnvMailFrom, EnvMailDigestDelay,
}

func init() { Vars = append(Vars, mailVars...) }

// Mail configures sending email. It is enabled when Host is set.
type Mail struct {
	Host     string // the relay, e.g. in-v3.mailjet.com
	Port     int
	TLS      string // starttls, tls (implicit) or none
	Username string
	Password Secret
	// From is the sender as configured ("dendronotes <no-reply@example.org>").
	From     string
	FromAddr *mail.Address
	// DigestDelay is how long a comment notification may stay unread before
	// it is sent by email.
	DigestDelay time.Duration
}

// Enabled reports whether email is configured.
func (m Mail) Enabled() bool { return m.Host != "" }

// loadMail reads the email settings; they need the public URL for the links
// in the messages.
func (c *Config) loadMail(getenv func(string) string) error {
	m := &c.Mail
	m.Port, m.TLS, m.DigestDelay = DefaultSMTPPort, DefaultSMTPTLS, DefaultDigestDelay
	var errs []error
	if v := getenv(EnvMailDigestDelay); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			errs = append(errs, fmt.Errorf("%s: %q is not a duration (e.g. 10m)", EnvMailDigestDelay, v))
		} else {
			m.DigestDelay = d
		}
	}
	m.Host = strings.TrimSpace(getenv(EnvSMTPHost))
	if m.Host == "" {
		return errors.Join(errs...)
	}
	if strings.ContainsAny(m.Host, "/: \t") && net.ParseIP(m.Host) == nil {
		errs = append(errs, fmt.Errorf("%s: %q must be a host name or IP address (the port goes into %s)", EnvSMTPHost, m.Host, EnvSMTPPort))
	}
	if v := getenv(EnvSMTPPort); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			errs = append(errs, fmt.Errorf("%s: %q is not a valid port (1-65535)", EnvSMTPPort, v))
		} else {
			m.Port = p
		}
	}
	if v := strings.ToLower(strings.TrimSpace(getenv(EnvSMTPTLS))); v != "" {
		switch v {
		case SMTPTLSStartTLS, SMTPTLSImplicit, SMTPTLSNone:
			m.TLS = v
		default:
			errs = append(errs, fmt.Errorf("%s: invalid value %q (starttls, tls, none)", EnvSMTPTLS, v))
		}
	}
	m.Username = strings.TrimSpace(getenv(EnvSMTPUsername))
	if f := getenv(EnvSMTPPassword); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvSMTPPassword, err))
		}
		m.Password = Secret(strings.TrimRight(string(b), "\r\n"))
	}
	switch {
	case m.Username != "" && m.Password == "":
		errs = append(errs, fmt.Errorf("%s needs %s", EnvSMTPUsername, EnvSMTPPassword))
	case m.Username == "" && m.Password != "":
		errs = append(errs, fmt.Errorf("%s needs %s", EnvSMTPPassword, EnvSMTPUsername))
	case m.Username != "" && m.TLS == SMTPTLSNone:
		errs = append(errs, fmt.Errorf("%s=none would send the SMTP password unencrypted; use starttls or tls", EnvSMTPTLS))
	}
	m.From = strings.TrimSpace(getenv(EnvMailFrom))
	if m.From == "" {
		errs = append(errs, fmt.Errorf("%s is required with %s, e.g. \"dendronotes <no-reply@example.org>\"", EnvMailFrom, EnvSMTPHost))
	} else if a, err := mail.ParseAddress(m.From); err != nil || strings.ContainsAny(m.From, "\r\n") || len(a.Address) > maxMailFromAddrSize {
		errs = append(errs, fmt.Errorf("%s: %q is not an email address", EnvMailFrom, m.From))
	} else {
		m.FromAddr = a
	}
	if c.Auth.PublicURL == "" {
		errs = append(errs, fmt.Errorf("%s is required with %s (the links in the emails)", EnvPublicURL, EnvSMTPHost))
	}
	return errors.Join(errs...)
}

// mailSetting is the effective value of an email variable (admin page).
func (c Config) mailSetting(name string) (string, bool) {
	m := c.Mail
	switch name {
	case EnvSMTPHost:
		return m.Host, true
	case EnvSMTPPort:
		if !m.Enabled() {
			return "", true
		}
		return strconv.Itoa(m.Port), true
	case EnvSMTPTLS:
		if !m.Enabled() {
			return "", true
		}
		return m.TLS, true
	case EnvSMTPUsername:
		return m.Username, true
	case EnvSMTPPassword:
		if m.Password != "" {
			return "(set)", true
		}
		return "", true
	case EnvMailFrom:
		return m.From, true
	case EnvMailDigestDelay:
		return m.DigestDelay.String(), true
	}
	return "", false
}
