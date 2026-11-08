// Package avatar checks uploaded profile photos. The browser crops a photo to
// a small square PNG; the server accepts only that and encodes it anew, so
// what is stored and served is a plain PNG of known size without metadata
// (no location, camera or text chunks).
package avatar

import (
	"bytes"
	"errors"
	"image/png"
)

const (
	// MaxBytes limits an upload.
	MaxBytes = 1 << 20
	// MinSize and MaxSize bound the width (= height) of a photo in pixels.
	MinSize = 64
	MaxSize = 512
)

// ErrInvalid reports an upload that is not a square PNG of allowed size.
var ErrInvalid = errors.New("invalid photo")

// Normalize checks a photo and returns it freshly encoded as PNG.
func Normalize(data []byte) ([]byte, error) {
	if len(data) > MaxBytes {
		return nil, ErrInvalid
	}
	// The header first: the size is known before any pixel is decoded.
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != cfg.Height || cfg.Width < MinSize || cfg.Width > MaxSize {
		return nil, ErrInvalid
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalid
	}
	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
