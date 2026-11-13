package mail

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed templates
var templateFS embed.FS

// The kinds of email dendronotes sends; each has templates/<kind>.tmpl with
// the blocks subject, button, text and html.
const (
	KindShared          = "shared"
	KindDigest          = "digest"
	KindPasswordChanged = "password-changed"
	KindTokenCreated    = "token-created"
	KindEmailChanged    = "email-changed"
	KindWelcome         = "welcome"
	KindReset           = "reset"
	KindTest            = "test"
	KindSignupConfirm   = "signup-confirm"
	KindSignupExists    = "signup-exists"
)

// Kinds lists every kind (the renderer parses them all at start).
var Kinds = []string{
	KindShared, KindDigest, KindPasswordChanged, KindTokenCreated, KindEmailChanged, KindWelcome, KindReset, KindTest,
	KindSignupConfirm, KindSignupExists,
}

// Data fills the templates. Common fields come first; the others belong to
// single kinds. Everything is escaped in the HTML part.
type Data struct {
	Name     string // the recipient's name
	Username string
	Base     string // dendronotes' public URL
	URL      string // where the button leads; empty: no button
	Settings bool   // the person can turn this kind off in the profile
	When     string
	Actor    string // who did it

	Soma, Role       string // shared
	Items            []Item // digest
	How              string // password-changed: self, admin, reset or set
	TokenName, Scope string // token-created
	NewEmail         string // email-changed
	SetPasswordURL   string // welcome (optional)
	Expires          string // welcome, reset, signup-confirm
	Host             string // test

	Subject string // set by Render for the HTML title
}

// Item is one comment in a digest.
type Item struct {
	Actor   string
	Mention bool // else a reply
	Soma    string
	Note    string
	Excerpt string
	URL     string
}

// Renderer turns Data into the parts of a Message.
type Renderer struct {
	text map[string]*texttemplate.Template
	html map[string]*htmltemplate.Template
}

// NewRenderer parses the templates of every kind.
func NewRenderer() (*Renderer, error) {
	r := &Renderer{text: map[string]*texttemplate.Template{}, html: map[string]*htmltemplate.Template{}}
	for _, k := range Kinds {
		t, err := texttemplate.New(k).ParseFS(templateFS, "templates/layout.txt", "templates/"+k+".tmpl")
		if err != nil {
			return nil, fmt.Errorf("mail template %s: %w", k, err)
		}
		h, err := htmltemplate.New(k).ParseFS(templateFS, "templates/layout.html", "templates/"+k+".tmpl")
		if err != nil {
			return nil, fmt.Errorf("mail template %s: %w", k, err)
		}
		r.text[k], r.html[k] = t, h
	}
	return r, nil
}

// Render fills in the subject and both bodies of a message to the address
// to.
func (r *Renderer) Render(kind, to string, d Data) (Message, error) {
	t, h := r.text[kind], r.html[kind]
	if t == nil || h == nil {
		return Message{}, fmt.Errorf("mail: unknown kind %q", kind)
	}
	var subject, text, html bytes.Buffer
	if err := t.ExecuteTemplate(&subject, "subject", d); err != nil {
		return Message{}, err
	}
	// One line, whatever the names contained.
	d.Subject = strings.Join(strings.Fields(subject.String()), " ")
	if err := t.ExecuteTemplate(&text, "layout", d); err != nil {
		return Message{}, err
	}
	if err := h.ExecuteTemplate(&html, "layout", d); err != nil {
		return Message{}, err
	}
	return Message{To: to, ToName: oneLine(d.Name), Subject: d.Subject, Text: text.String(), HTML: html.String()}, nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
