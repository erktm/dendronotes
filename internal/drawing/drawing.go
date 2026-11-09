// Package drawing checks the files of drawings ("Name.drawing.svg"): plain
// SVG that any image viewer shows, with the editable scene as JSON in
// <metadata>. The editor in the browser writes them; the server accepts only
// what such a file needs – a short list of elements and attributes, images
// only as data: URLs, no scripts, no event handlers, no external references –
// so a drawing stays a harmless picture wherever it is opened. Icons in SVG
// are checked by the same rules.
package drawing

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image/png"
	"io"
	"regexp"
	"strings"
)

// Suffix marks drawing files.
const Suffix = ".drawing.svg"

// IsDrawing reports whether a path names a drawing.
func IsDrawing(p string) bool { return strings.HasSuffix(strings.ToLower(p), Suffix) }

// MaxBytes bounds a drawing (embedded icons included).
const MaxBytes = 8 << 20

// ErrInvalid reports a drawing (or icon) that breaks the rules.
var ErrInvalid = errors.New("invalid drawing")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

var elements = map[string]bool{
	"svg": true, "g": true, "defs": true, "metadata": true, "style": true, "title": true, "desc": true,
	"rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true, "path": true,
	"text": true, "tspan": true, "image": true, "symbol": true, "use": true, "clipPath": true,
	"linearGradient": true, "radialGradient": true, "stop": true, "mask": true, "pattern": true,
}

var attributes = map[string]bool{
	"id": true, "class": true, "version": true, "viewBox": true, "width": true, "height": true,
	"x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true, "cx": true, "cy": true, "r": true, "rx": true, "ry": true,
	"d": true, "points": true, "transform": true, "preserveAspectRatio": true, "href": true,
	"fill": true, "fill-opacity": true, "fill-rule": true, "clip-rule": true, "stroke": true, "stroke-width": true,
	"stroke-opacity": true, "stroke-dasharray": true, "stroke-dashoffset": true, "stroke-linecap": true,
	"stroke-linejoin": true, "stroke-miterlimit": true, "opacity": true, "vector-effect": true,
	"font-family": true, "font-size": true, "font-weight": true, "font-style": true, "text-anchor": true,
	"dominant-baseline": true, "alignment-baseline": true, "letter-spacing": true, "white-space": true,
	"clip-path": true, "mask": true, "offset": true, "stop-color": true, "stop-opacity": true,
	"gradientUnits": true, "gradientTransform": true, "patternUnits": true, "clipPathUnits": true,
	"maskUnits": true, "fx": true, "fy": true, "dx": true, "dy": true, "role": true, "aria-label": true,
	"data-name": true,
}

// dataImage is the only form of an image reference: an inline picture.
var dataImage = regexp.MustCompile(`^data:image/(png|jpeg|webp|gif|svg\+xml);base64,[A-Za-z0-9+/=\s]+$`)

// localRef is a reference within the file (url(#id), #id).
var localRef = regexp.MustCompile(`^#[A-Za-z0-9_.:-]+$`)

// cssForbidden may not appear in style sheets or presentation attributes.
var cssForbidden = []string{"url(", "@import", "expression", "javascript:", "\\", "<", "behavior", "-moz-binding"}

// Check verifies a drawing written by the editor.
func Check(data []byte) error {
	if len(data) > MaxBytes {
		return invalid("larger than %d bytes", MaxBytes)
	}
	return checkSVG(data, true)
}

// CheckIcon verifies an SVG icon for the icon library.
func CheckIcon(data []byte) error {
	if len(data) > MaxIconBytes {
		return invalid("larger than %d bytes", MaxIconBytes)
	}
	return checkSVG(data, false)
}

func checkSVG(data []byte, drawing bool) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	depth, root, metadata := 0, false, false
	var stack []string
	var css strings.Builder // the text of the current <style>, checked as a whole
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return invalid("not well-formed XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg" && !(len(stack) > 0 && stack[len(stack)-1] == "metadata") {
				return invalid("element %s in namespace %s", name, t.Name.Space)
			}
			if depth == 0 {
				if name != "svg" || root {
					return invalid("the root must be one <svg>")
				}
				root = true
			}
			inMetadata := len(stack) > 0 && stack[len(stack)-1] == "metadata"
			if !elements[name] && !inMetadata {
				return invalid("element <%s> is not allowed", name)
			}
			if name == "metadata" {
				metadata = true
			}
			for _, a := range t.Attr {
				if err := checkAttr(name, a); err != nil {
					return err
				}
			}
			stack = append(stack, name)
			depth++
		case xml.EndElement:
			if t.Name.Local == "style" {
				if err := checkCSS(css.String()); err != nil {
					return err
				}
				css.Reset()
			}
			depth--
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] == "style" {
				css.Write(t) // CDATA sections arrive in pieces
			}
		case xml.Directive:
			return invalid("DOCTYPE and other directives are not allowed")
		case xml.ProcInst:
			if t.Target != "xml" {
				return invalid("processing instruction %s", t.Target)
			}
		}
	}
	if !root {
		return invalid("no <svg> element")
	}
	if drawing && !metadata {
		return invalid("no scene (<metadata>)")
	}
	return nil
}

func checkAttr(element string, a xml.Attr) error {
	name := a.Name.Local
	switch {
	case a.Name.Space == "xmlns" || (a.Name.Space == "" && name == "xmlns"):
		return nil // namespace declarations
	case a.Name.Space == "http://www.w3.org/1999/xlink" && name == "href":
		// legacy xlink:href – same rules as href
	case a.Name.Space == "http://www.w3.org/XML/1998/namespace":
		return nil // xml:space, xml:lang
	case a.Name.Space != "":
		return invalid("attribute %s:%s", a.Name.Space, name)
	case !attributes[name]:
		return invalid("attribute %s on <%s>", name, element)
	}
	v := strings.TrimSpace(a.Value)
	if name == "href" {
		if element == "use" && localRef.MatchString(v) {
			return nil
		}
		if element != "image" || !dataImage.MatchString(v) {
			return invalid("<%s> may only show inline images", element)
		}
		return nil
	}
	if strings.HasPrefix(v, "url(") {
		// fill="url(#gradient)", clip-path="url(#c)": only within the file.
		inner := strings.TrimSuffix(strings.TrimPrefix(v, "url("), ")")
		if localRef.MatchString(strings.Trim(inner, `'"`)) {
			return nil
		}
		return invalid("attribute %s refers outside the file", name)
	}
	return checkCSS(v)
}

func checkCSS(s string) error {
	low := strings.ToLower(s)
	for _, bad := range cssForbidden {
		if strings.Contains(low, bad) {
			return invalid("style contains %q", bad)
		}
	}
	return nil
}

// Empty is a new, empty drawing (the editor adds to it).
func Empty() []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 300" width="400" height="300">` +
		`<metadata id="dendronotes-drawing">{"version":1,"elements":[],"sketchy":false}</metadata></svg>` + "\n")
}

// MaxIconBytes bounds an icon of the library.
const MaxIconBytes = 256 << 10

// MaxIconSize bounds the width and height of a raster icon in pixels.
const MaxIconSize = 512

// NormalizeIcon checks an uploaded icon: PNG (the browser converts other
// pictures) is encoded anew without metadata, SVG must pass CheckIcon. It
// returns the media type and the bytes to keep.
func NormalizeIcon(data []byte) (string, []byte, error) {
	if len(data) > MaxIconBytes {
		return "", nil, invalid("icon larger than %d bytes", MaxIconBytes)
	}
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxIconSize || cfg.Height > MaxIconSize {
			return "", nil, invalid("PNG icons are at most %d pixels wide and high", MaxIconSize)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return "", nil, invalid("broken PNG")
		}
		var out bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img); err != nil {
			return "", nil, err
		}
		return "image/png", out.Bytes(), nil
	}
	if err := CheckIcon(data); err != nil {
		return "", nil, err
	}
	return "image/svg+xml", data, nil
}
