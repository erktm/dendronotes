package server

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// TestCSPStyleAttributeException pins the single inline style attribute the
// CSP allows: CodeMirror's style="tab-size: 4". Any other inline style
// attribute stays blocked.
func TestCSPStyleAttributeException(t *testing.T) {
	sum := sha256.Sum256([]byte("tab-size: 4"))
	want := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
	if codeMirrorTabSizeHash != want {
		t.Fatalf("hash %s does not match tab-size: 4 (%s)", codeMirrorTabSizeHash, want)
	}

	s := newTestServer(t, testFS())
	csp := get(t, s.Handler(), "GET", "/").Header().Get("Content-Security-Policy")
	var attr string
	for _, d := range strings.Split(csp, ";") {
		if d = strings.TrimSpace(d); strings.HasPrefix(d, "style-src-attr ") {
			attr = d
		}
	}
	if attr != "style-src-attr 'unsafe-hashes' '"+want+"'" {
		t.Fatalf("style-src-attr = %q", attr)
	}
}
