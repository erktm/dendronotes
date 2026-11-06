// Package attach decides which files may be uploaded into the soma and how
// files are served back.
//
// An upload is accepted only if its extension is on the allow list and its
// first bytes match that type (a ".png" must start like a PNG). Files are
// served with the type that belongs to their extension, never a sniffed one.
package attach

import (
	"bytes"
	"errors"
	"path"
	"strings"
	"unicode/utf8"
)

// Kind groups types by how the UI shows them.
type Kind string

const (
	Image Kind = "image"
	Audio Kind = "audio"
	Video Kind = "video"
	PDF   Kind = "pdf"
	Text  Kind = "text"
	Other Kind = "other"
)

// Type is an allowed file type.
type Type struct {
	MIME string
	Kind Kind
	// Inline types may be shown in the browser; others are downloads.
	Inline bool
	// match checks the first bytes of a file (at most HeadSize).
	match func(head []byte) bool
}

// HeadSize is how many leading bytes Check needs.
const HeadSize = 512

// ErrType means the extension is not allowed or the content does not match it.
var ErrType = errors.New("unsupported file type")

var types = map[string]Type{
	".png":      {"image/png", Image, true, prefix("\x89PNG\r\n\x1a\n")},
	".jpg":      {"image/jpeg", Image, true, prefix("\xff\xd8\xff")},
	".jpeg":     {"image/jpeg", Image, true, prefix("\xff\xd8\xff")},
	".gif":      {"image/gif", Image, true, anyPrefix("GIF87a", "GIF89a")},
	".webp":     {"image/webp", Image, true, riff("WEBP")},
	".avif":     {"image/avif", Image, true, isoBrand("avif", "avis")},
	".bmp":      {"image/bmp", Image, true, prefix("BM")},
	".ico":      {"image/x-icon", Image, true, prefix("\x00\x00\x01\x00")},
	".svg":      {"image/svg+xml", Image, true, isSVG},
	".pdf":      {"application/pdf", PDF, true, prefix("%PDF-")},
	".mp3":      {"audio/mpeg", Audio, true, isMP3},
	".wav":      {"audio/wav", Audio, true, riff("WAVE")},
	".ogg":      {"audio/ogg", Audio, true, prefix("OggS")},
	".flac":     {"audio/flac", Audio, true, prefix("fLaC")},
	".m4a":      {"audio/mp4", Audio, true, isoBrand("M4A ", "M4B ", "mp42", "isom", "dash")},
	".mp4":      {"video/mp4", Video, true, isoBrand("isom", "iso2", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "dash", "M4V ", "MSNV")},
	".webm":     {"video/webm", Video, true, prefix("\x1a\x45\xdf\xa3")},
	".mov":      {"video/quicktime", Video, true, isoBrand("qt  ")},
	".md":       {"text/markdown; charset=utf-8", Text, true, isText},
	".markdown": {"text/markdown; charset=utf-8", Text, true, isText},
	".txt":      {"text/plain; charset=utf-8", Text, true, isText},
	".csv":      {"text/csv; charset=utf-8", Text, true, isText},
	".json":     {"application/json", Text, true, isText},
	".zip":      {"application/zip", Other, false, anyPrefix("PK\x03\x04", "PK\x05\x06")},
}

// Lookup returns the allowed type for a file name.
func Lookup(name string) (Type, bool) {
	t, ok := types[strings.ToLower(path.Ext(name))]
	return t, ok
}

// Check verifies that a file with this name and these first bytes may be
// stored. head is the beginning of the content (up to HeadSize bytes).
func Check(name string, head []byte) error {
	t, ok := Lookup(name)
	if !ok || !t.match(head) {
		return ErrType
	}
	return nil
}

// Serving describes how to deliver a stored file.
type Serving struct {
	ContentType string
	Inline      bool
}

// Serve returns the content type and disposition for a stored file. Files of
// unknown type (put into the soma by other means) are offered as downloads.
func Serve(name string) Serving {
	if t, ok := Lookup(name); ok {
		return Serving{ContentType: t.MIME, Inline: t.Inline}
	}
	return Serving{ContentType: "application/octet-stream"}
}

// Extensions lists the allowed extensions (for documentation and the UI).
func Extensions() []string {
	out := make([]string, 0, len(types))
	for ext := range types {
		out = append(out, ext)
	}
	return out
}

func prefix(sig string) func([]byte) bool {
	return func(h []byte) bool { return bytes.HasPrefix(h, []byte(sig)) }
}

func anyPrefix(sigs ...string) func([]byte) bool {
	return func(h []byte) bool {
		for _, s := range sigs {
			if bytes.HasPrefix(h, []byte(s)) {
				return true
			}
		}
		return false
	}
}

// riff matches RIFF containers of one form type (WEBP, WAVE).
func riff(form string) func([]byte) bool {
	return func(h []byte) bool {
		return len(h) >= 12 && string(h[:4]) == "RIFF" && string(h[8:12]) == form
	}
}

// isoBrand matches ISO base media files (MP4, MOV, AVIF …) whose "ftyp" box
// names one of the brands as major or compatible brand.
func isoBrand(brands ...string) func([]byte) bool {
	return func(h []byte) bool {
		if len(h) < 16 || string(h[4:8]) != "ftyp" {
			return false
		}
		size := int(h[0])<<24 | int(h[1])<<16 | int(h[2])<<8 | int(h[3])
		end := min(max(size, 16), len(h))
		for i := 8; i+4 <= end; i += 4 {
			if i == 12 {
				continue // minor version, not a brand
			}
			for _, b := range brands {
				if string(h[i:i+4]) == b {
					return true
				}
			}
		}
		return false
	}
}

// isMP3 accepts an ID3 tag or an MPEG audio frame header.
func isMP3(h []byte) bool {
	return bytes.HasPrefix(h, []byte("ID3")) || (len(h) >= 2 && h[0] == 0xff && h[1]&0xe0 == 0xe0)
}

// isText accepts UTF-8 without NUL bytes (a cut-off character at the end of
// the head is fine).
func isText(h []byte) bool {
	if bytes.IndexByte(h, 0) >= 0 {
		return false
	}
	for len(h) > 0 {
		r, size := utf8.DecodeRune(h)
		if r == utf8.RuneError && size == 1 {
			return len(h) < utf8.UTFMax && !utf8.FullRune(h)
		}
		h = h[size:]
	}
	return true
}

// isSVG accepts text whose first element is <svg (after an optional XML
// declaration, doctype and comments). SVG is always served sandboxed.
func isSVG(h []byte) bool {
	if !isText(h) {
		return false
	}
	s := strings.TrimPrefix(string(h), "\ufeff")
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		switch {
		case strings.HasPrefix(s, "<?"):
			i := strings.Index(s, "?>")
			if i < 0 {
				return false
			}
			s = s[i+2:]
		case strings.HasPrefix(s, "<!--"):
			i := strings.Index(s, "-->")
			if i < 0 {
				return false
			}
			s = s[i+3:]
		case strings.HasPrefix(s, "<!"):
			i := strings.IndexByte(s, '>')
			if i < 0 {
				return false
			}
			s = s[i+1:]
		default:
			return strings.HasPrefix(s, "<svg") && len(s) > 4 && strings.ContainsRune(" \t\r\n>/", rune(s[4]))
		}
	}
}
