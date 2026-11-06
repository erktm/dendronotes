package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dendronotes/internal/soma"
)

const pngBytes = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR some image data"

func (c *client) upload(path, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "image/png")
	req.AddCookie(c.cookie)
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

func TestUploadAndServeFiles(t *testing.T) {
	s := newIndexedServer(t, map[string]string{"Projects/Plan.md": "x"})
	s.cfg.MaxUploadBytes = 1024
	next := subscribe(t, s)
	c := newClient(t, s)
	c.login()

	rec := c.upload("/api/somas/testsoma01/files/Projects/photo.png", pngBytes)
	expect(t, rec, 201, "")
	if e := decode[soma.Entry](t, rec); e.Path != "Projects/photo.png" || e.Size != int64(len(pngBytes)) {
		t.Errorf("entry: %+v", e)
	}
	if ch := next(); !ch.Tree {
		t.Errorf("upload not published as tree change: %+v", ch)
	}
	expect(t, c.upload("/api/somas/testsoma01/files/Projects/photo.png", pngBytes), 409, "exists")
	rec = c.upload("/api/somas/testsoma01/files/Projects/photo.png?unique=1", pngBytes)
	expect(t, rec, 201, "")
	if e := decode[soma.Entry](t, rec); e.Path != "Projects/photo (1).png" {
		t.Errorf("unique name: %+v", e)
	}

	expect(t, c.upload("/api/somas/testsoma01/files/fake.png", "<html><script>alert(1)</script>"), 415, "unsupported_type")
	expect(t, c.upload("/api/somas/testsoma01/files/page.html", "<html>"), 415, "unsupported_type")
	expect(t, c.upload("/api/somas/testsoma01/files/big.png", pngBytes+strings.Repeat("x", 2000)), 413, "too_large")
	expect(t, c.upload("/api/somas/testsoma01/files/.git/x.png", pngBytes), 400, "")
	expect(t, c.upload("/api/somas/testsoma01/files/..%2Fescape.png", pngBytes), 400, "")

	rec = c.do("GET", "/api/somas/testsoma01/files/Projects/photo.png", "")
	expect(t, rec, 200, "")
	h := rec.Header()
	if rec.Body.String() != pngBytes || h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") || !strings.HasPrefix(h.Get("Content-Disposition"), "inline") {
		t.Errorf("served file: %q %v", rec.Body.String(), h)
	}

	// Range requests (seeking in media) and revalidation.
	rec = c.do("GET", "/api/somas/testsoma01/files/Projects/photo.png", "", "Range", "bytes=0-3")
	if rec.Code != 206 || rec.Body.String() != pngBytes[:4] {
		t.Errorf("range: %d %q", rec.Code, rec.Body.String())
	}
	if etag := h.Get("ETag"); c.do("GET", "/api/somas/testsoma01/files/Projects/photo.png", "", "If-None-Match", etag).Code != 304 {
		t.Error("If-None-Match did not answer 304")
	}

	expect(t, c.do("GET", "/api/somas/testsoma01/files/missing.png", ""), 404, "not_found")
	expect(t, c.do("GET", "/api/somas/testsoma01/files/Projects", ""), 400, "is_directory")
	expect(t, (&client{t: t, h: s.Handler()}).do("GET", "/api/somas/testsoma01/files/Projects/photo.png", ""), 401, "unauthorized")
}

func TestServedFilesOfUnknownTypeAreDownloads(t *testing.T) {
	s := newIndexedServer(t, map[string]string{"page.html": "<script>alert(1)</script>", "Café notes.docx": "PK"})
	c := newClient(t, s)
	c.login()
	for path, name := range map[string]string{"page.html": "page.html", "Caf%C3%A9%20notes.docx": "Café notes.docx"} {
		rec := c.do("GET", "/api/somas/testsoma01/files/"+path, "")
		expect(t, rec, 200, "")
		h := rec.Header()
		if h.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") {
			t.Errorf("%s: %v", name, h)
		}
		if !strings.Contains(h.Get("Content-Disposition"), "filename") {
			t.Errorf("%s: no file name in %q", name, h.Get("Content-Disposition"))
		}
	}
}

func TestUploadedNoteIsIndexed(t *testing.T) {
	s := newIndexedServer(t, nil)
	c := newClient(t, s)
	c.login()
	expect(t, c.upload("/api/somas/testsoma01/files/Imported.md", "Imported note about quokkas"), 201, "")
	if hits, _ := rtOf(t, s).Index.Search(t.Context(), "quokka", 5); len(hits) != 1 {
		t.Errorf("uploaded note not searchable: %+v", hits)
	}
	if _, err := os.Stat(filepath.Join(somaDir(s), "Imported.md")); err != nil {
		t.Error(err)
	}
}
