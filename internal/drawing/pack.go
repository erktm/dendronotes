package drawing

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Icon packs are ZIP files of SVG and PNG pictures; their folders become
// sections. Every picture goes through NormalizeIcon like a single upload.

const (
	// MaxPackBytes bounds the ZIP and everything in it, unpacked.
	MaxPackBytes = 20 << 20
	// MaxPackIcons bounds the pictures in one pack.
	MaxPackIcons = 500
)

// ErrPack means the file is no usable icon pack.
var ErrPack = errors.New("not an icon pack")

// PackIcon is one picture of a pack, checked and normalized.
type PackIcon struct {
	Section string
	Name    string
	Mime    string
	Data    []byte
}

// PackSkip is a file of a pack that was left out, and why.
type PackSkip struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// ReadPack reads an icon pack. Pictures in folders go to a section named
// after the folder (nested folders joined with " / "); pictures at the top
// go to a section named fallback (the pack's file name), or after the one
// folder around everything when there is one.
func ReadPack(data []byte, fallback string) ([]PackIcon, []PackSkip, error) {
	if len(data) > MaxPackBytes {
		return nil, nil, ErrPack
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, ErrPack
	}
	type entry struct {
		f    *zip.File
		name string
	}
	var files []entry
	var skipped []PackSkip
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if f.FileInfo().IsDir() || hiddenInPack(name) {
			continue
		}
		ext := strings.ToLower(path.Ext(name))
		if ext != ".svg" && ext != ".png" {
			skipped = append(skipped, PackSkip{File: name, Reason: "not an SVG or PNG picture"})
			continue
		}
		if !utf8.ValidString(name) || strings.HasPrefix(name, "/") || strings.Contains("/"+name+"/", "/../") {
			skipped = append(skipped, PackSkip{File: name, Reason: "unusable file name"})
			continue
		}
		files = append(files, entry{f, path.Clean(name)})
	}
	if len(files) > MaxPackIcons {
		return nil, nil, ErrPack
	}
	// A folder around everything is the pack itself, not a section.
	root := ""
	if len(files) > 0 {
		first := files[0].name
		if i := strings.IndexByte(first, '/'); i > 0 {
			root = first[:i+1]
			for _, e := range files {
				if !strings.HasPrefix(e.name, root) {
					root = ""
					break
				}
			}
		}
	}
	// That folder names the pictures directly in it.
	if root != "" {
		fallback = strings.TrimSuffix(root, "/")
	}
	var out []PackIcon
	var total int64
	for _, e := range files {
		rel := strings.TrimPrefix(e.name, root)
		section := fallback
		if dir := path.Dir(rel); dir != "." {
			section = strings.Join(strings.Split(dir, "/"), " / ")
		}
		if e.f.UncompressedSize64 > MaxIconBytes {
			skipped = append(skipped, PackSkip{File: e.name, Reason: "larger than 256 KB"})
			continue
		}
		rc, err := e.f.Open()
		if err != nil {
			skipped = append(skipped, PackSkip{File: e.name, Reason: "cannot be read"})
			continue
		}
		// The sizes in a ZIP can lie: read at most one byte more than allowed.
		raw, err := io.ReadAll(io.LimitReader(rc, MaxIconBytes+1))
		rc.Close()
		if err != nil {
			skipped = append(skipped, PackSkip{File: e.name, Reason: "cannot be read"})
			continue
		}
		total += int64(len(raw))
		if total > MaxPackBytes {
			return nil, nil, ErrPack
		}
		mime, clean, err := NormalizeIcon(raw)
		if err != nil {
			skipped = append(skipped, PackSkip{File: e.name, Reason: err.Error()})
			continue
		}
		out = append(out, PackIcon{Section: cleanLabel(section, fallback), Name: cleanLabel(strings.TrimSuffix(path.Base(rel), path.Ext(rel)), "Icon"), Mime: mime, Data: clean})
	}
	return out, skipped, nil
}

// hiddenInPack reports files a pack carries by accident (macOS metadata,
// hidden files and folders).
func hiddenInPack(name string) bool {
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") || seg == "__MACOSX" {
			return true
		}
	}
	return false
}

// cleanLabel makes a name for a section or an icon: no control characters,
// at most 80 characters, else fallback.
func cleanLabel(s, fallback string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if utf8.RuneCountInString(s) > 80 {
		s = string([]rune(s)[:80])
	}
	if s == "" {
		return fallback
	}
	return s
}
