// Package httpx contains small JSON helpers shared by the HTTP handlers.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
)

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes {"error": code} with the given status.
func Error(w http.ResponseWriter, status int, code string) {
	WriteJSON(w, status, map[string]string{"error": code})
}

// DecodeError describes why a request body was rejected.
type DecodeError struct {
	Status int
	Code   string
	Err    error
}

func (e *DecodeError) Error() string { return fmt.Sprintf("%s: %v", e.Code, e.Err) }

// DecodeJSON reads exactly one JSON object of at most maxBytes into dst.
// Unknown fields are rejected, and the Content-Type must be application/json
// (which browsers cannot send cross-origin without a CORS preflight).
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return &DecodeError{http.StatusUnsupportedMediaType, "unsupported_media_type", errors.New("expected application/json")}
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &DecodeError{http.StatusRequestEntityTooLarge, "too_large", err}
		}
		return &DecodeError{http.StatusBadRequest, "bad_request", err}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return &DecodeError{http.StatusBadRequest, "bad_request", errors.New("trailing data after JSON object")}
	}
	return nil
}

// WriteDecodeError answers with the status from a DecodeJSON error.
func WriteDecodeError(w http.ResponseWriter, err error) {
	var de *DecodeError
	if errors.As(err, &de) {
		Error(w, de.Status, de.Code)
		return
	}
	Error(w, http.StatusBadRequest, "bad_request")
}
