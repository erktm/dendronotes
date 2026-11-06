package server

import (
	"fmt"
	"mime"
	"net/http"
	"path"
	"time"

	"dendronotes/internal/attach"
	"dendronotes/internal/httpx"
)

// fileCSP applies to files opened directly in the browser. They are
// untrusted: the sandbox gives them an opaque origin without scripts, so an
// SVG (or anything else) can neither run code nor reach the API.
const fileCSP = "default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'; sandbox"

// handleReadFile serves a file from the soma (attachments, images, media).
// The content type follows the extension, never the content; unknown types
// are downloads.
func (s *Server) handleReadFile(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	f, e, err := rt.Soma.OpenFile(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	defer f.Close()

	sv := attach.Serve(e.Path)
	disposition := "attachment"
	if sv.Inline {
		disposition = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", sv.ContentType)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": path.Base(e.Path)}))
	h.Set("Content-Security-Policy", fileCSP)
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", fmt.Sprintf(`W/"%x-%x"`, e.Size, e.MTime))
	// ServeContent answers range requests (seeking in audio and video) and
	// conditional requests.
	http.ServeContent(w, r, "", time.UnixMilli(e.MTime), f)
}

// handleUpload stores the request body as a new file. The body is the raw
// file content; its type is checked against the file name. With ?unique=1 a
// taken name gets a " (n)" suffix; the answer names the stored path.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	max := s.cfg.MaxUploadBytes
	if r.ContentLength > max {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	name := r.PathValue("path")
	body := http.MaxBytesReader(w, r.Body, max+1)
	e, err := rt.Soma.CreateFile(name, body, max, r.URL.Query().Get("unique") == "1", func(head []byte) error {
		return attach.Check(name, head)
	})
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	rt.SyncAfterWrite(r.Context())
	httpx.WriteJSON(w, http.StatusCreated, e)
}
