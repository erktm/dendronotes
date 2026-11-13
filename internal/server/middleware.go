package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"dendronotes/internal/httpx"
)

type ctxKey int

const nonceKey ctxKey = iota

// cspTemplate is the Content-Security-Policy for every response.
// Scripts only from our own origin, no inline JS, no eval. Inline <style>
// only with a nonce (for CodeMirror's fallback without adoptedStyleSheets).
// Inline style attributes are blocked, except exactly one: CodeMirror sets
// style="tab-size: 4" on its content element (allowed by its hash).
// img-src and media-src allow no foreign hosts: external images in notes stay
// blocked (air-gap friendly, no tracking pixels); attachments come from
// /api/somas/{soma}/files/.
const cspTemplate = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self' 'nonce-%s'; " +
	"style-src-attr 'unsafe-hashes' '" + codeMirrorTabSizeHash + "'; " +
	"img-src 'self' data: blob:; " +
	"media-src 'self'; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"manifest-src 'self'; " +
	"base-uri 'none'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'; " +
	"object-src 'none'"

// codeMirrorTabSizeHash is the CSP hash of the attribute value "tab-size: 4".
const codeMirrorTabSizeHash = "sha256-qokx773Tl626SVTuhxQSccr23ixTSkbnBXWRqLjyuIg="

const permissionsPolicy = "accelerometer=(), camera=(), geolocation=(), gyroscope=(), " +
	"magnetometer=(), microphone=(), payment=(), usb=(), browsing-topics=()"

// securityHeaders sets the security headers and puts a fresh nonce for
// this request into the context.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := newNonce()
		h := w.Header()
		h.Set("Content-Security-Policy", fmt.Sprintf(cspTemplate, nonce))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", permissionsPolicy)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), nonceKey, nonce)))
	})
}

func newNonce() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b) // crypto/rand.Read never fails, per its documentation
	return base64.RawURLEncoding.EncodeToString(b)
}

func nonceFrom(ctx context.Context) string {
	n, _ := ctx.Value(nonceKey).(string)
	return n
}

// recoverPanics catches panics in handlers, logs them and answers with 500
// instead of silently dropping the connection.
func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v)
			}
			s.log.Error("panic in handler", "panic", fmt.Sprint(v), "path", r.URL.Path, "stack", string(debug.Stack()))
			if rw, ok := w.(*statusRecorder); !ok || !rw.wroteHeader {
				httpx.Error(w, http.StatusInternalServerError, "internal_error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// accessLog logs every request as one structured line.
// Query strings are deliberately not logged (search terms can be private).
// Health checks are logged at debug level only, so probes don't flood the log.
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		r, label := withRouteLabel(r)
		next.ServeHTTP(rec, r)
		s.metrics.observe(r, label, rec.status, time.Since(start))

		lvl := slog.LevelInfo
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			lvl = slog.LevelDebug
		}
		if rec.status >= 500 {
			lvl = slog.LevelError
		}
		s.log.Log(r.Context(), lvl, "request",
			"method", r.Method,
			"path", logPath(r.URL.Path),
			"status", rec.status,
			"bytes", rec.bytes,
			"dur_ms", float64(time.Since(start).Microseconds())/1000,
			"remote", r.RemoteAddr,
		)
	})
}

// statusRecorder remembers status and size of the response. Unwrap gives
// http.ResponseController access to Flush/deadlines (needed for SSE).
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logPath is a request path for the log, without the token of a public link
// (/p/<token>, /api/public/<token>/…): whoever reads the log does not get to
// open what was shared.
func logPath(p string) string {
	for _, prefix := range []string{"/p/", "/api/public/"} {
		if rest, ok := strings.CutPrefix(p, prefix); ok {
			if i := strings.IndexByte(rest, '/'); i >= 0 {
				return prefix + "…" + rest[i:]
			}
			return prefix + "…"
		}
	}
	return p
}
