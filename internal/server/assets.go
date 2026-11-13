package server

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"dendronotes/internal/httpx"
)

// noncePlaceholder appears in web/index.html and is replaced per response.
const noncePlaceholder = "__CSP_NONCE__"

func init() {
	// The distroless image has no /etc/mime.types and Go does not know these
	// types by itself. With nosniff the Content-Type has to be right.
	for ext, typ := range map[string]string{
		".woff2":       "font/woff2",
		".woff":        "font/woff",
		".ico":         "image/x-icon",
		".webmanifest": "application/manifest+json",
		".txt":         "text/plain; charset=utf-8",
	} {
		_ = mime.AddExtensionType(ext, typ)
	}
}

// assets serves the built frontend from the embedded file system.
//
//   - /assets/*   → content-hashed files, cacheable for a year; missing → 404
//   - /<file>     → files in the root directory (favicon …), except dotfiles
//   - everything else → index.html (SPA routing) with a fresh CSP nonce
type assets struct {
	fsys  fs.FS
	index []byte // nil if the frontend has not been built
}

func newAssets(fsys fs.FS) (*assets, error) {
	a := &assets{fsys: fsys}
	b, err := fs.ReadFile(fsys, "index.html")
	switch {
	case err == nil:
		a.index = b
	case errors.Is(err, fs.ErrNotExist):
	default:
		return nil, err
	}
	return a, nil
}

func (a *assets) hasIndex() bool { return a.index != nil }

func (a *assets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpx.Error(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	// The ServeMux has already cleaned the path (no "..", no duplicate "/").
	p := strings.TrimPrefix(r.URL.Path, "/")

	switch {
	case p == "" || p == "index.html":
		a.serveIndex(w, r)
	case strings.HasPrefix(p, "assets/"):
		if !a.serveFile(w, r, p, "public, max-age=31536000, immutable") {
			http.NotFound(w, r)
		}
	case !strings.Contains(p, "/") && !strings.HasPrefix(p, "."):
		if !a.serveFile(w, r, p, "public, max-age=3600") {
			a.serveIndex(w, r)
		}
	default:
		a.serveIndex(w, r)
	}
}

func (a *assets) serveIndex(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	// Pages of public share links stay out of search engines.
	if strings.HasPrefix(r.URL.Path, "/p/") {
		h.Set("X-Robots-Tag", "noindex, nofollow")
	}
	if a.index == nil {
		h.Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "Frontend not built. Run `make web`.\n")
		return
	}
	body := bytes.ReplaceAll(a.index, []byte(noncePlaceholder), []byte(nonceFrom(r.Context())))
	h.Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(body))
}

// serveFile serves name if it is a regular file.
func (a *assets) serveFile(w http.ResponseWriter, r *http.Request, name, cacheControl string) bool {
	if !fs.ValidPath(name) {
		return false
	}
	f, err := a.fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return false
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", cacheControl)
	http.ServeContent(w, r, name, st.ModTime(), rs)
	return true
}
