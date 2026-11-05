package vault

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	maxPathBytes = 4096
	maxNameBytes = 255
)

// CleanPath validates a vault-relative, "/"-separated path that refers to
// an existing entry. It is deliberately lenient about characters (files may
// have been created by other tools), but strict about structure:
//
//   - not empty, no leading or trailing "/", no empty segments
//   - no "." or ".." segments and no hidden segments (leading ".")
//   - valid UTF-8, no control characters, no backslashes
//
// Hidden entries (.git, .obsidian, .trash …) are never reachable via the API.
// CleanPath is the first line of defense; os.Root is the second.
func CleanPath(p string) (string, error) {
	if p == "" || len(p) > maxPathBytes || !utf8.ValidString(p) {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return "", ErrInvalidPath
	}
	for seg := range strings.SplitSeq(p, "/") {
		if !validSegment(seg) {
			return "", ErrInvalidPath
		}
	}
	return p, nil
}

func validSegment(seg string) bool {
	if seg == "" || len(seg) > maxNameBytes || seg[0] == '.' {
		return false
	}
	for _, r := range seg {
		if r < 0x20 || r == 0x7f || r == '\\' || r == utf8.RuneError {
			return false
		}
	}
	return true
}

// forbiddenNameChars cannot be used in names created through leafnotes, so
// that vaults stay portable to Windows/macOS when synced (e.g. Syncthing).
const forbiddenNameChars = `\/:*?"<>|`

var reservedWindowsNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// ValidateName checks a single new file or folder name and returns it in
// Unicode NFC form. The rules are stricter than CleanPath's.
func ValidateName(name string) (string, error) {
	name = norm.NFC.String(name)
	if !validSegment(name) {
		return "", ErrInvalidName
	}
	if strings.ContainsAny(name, forbiddenNameChars) {
		return "", ErrInvalidName
	}
	first, _ := utf8.DecodeRuneInString(name)
	last, _ := utf8.DecodeLastRuneInString(name)
	if unicode.IsSpace(first) || unicode.IsSpace(last) || last == '.' {
		return "", ErrInvalidName
	}
	stem, _, _ := strings.Cut(name, ".")
	if reservedWindowsNames[strings.ToLower(stem)] {
		return "", ErrInvalidName
	}
	return name, nil
}

// cleanNewPath validates the path of an entry that is about to be created:
// the parent part must be a valid path, the last segment a valid new name.
func cleanNewPath(p string) (string, error) {
	if _, err := CleanPath(p); err != nil {
		return "", err
	}
	dir, base := path.Split(p)
	name, err := ValidateName(base)
	if err != nil {
		return "", err
	}
	return dir + name, nil
}

// noteExtensions are the file types the editor can open.
var noteExtensions = map[string]bool{".md": true, ".markdown": true, ".txt": true}

// IsNote reports whether p has an editable text extension.
func IsNote(p string) bool {
	return noteExtensions[strings.ToLower(path.Ext(p))]
}
