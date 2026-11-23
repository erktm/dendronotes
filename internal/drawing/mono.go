package drawing

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Monochrome reports whether an icon (already through NormalizeIcon) is drawn
// in one dark, neutral color only, as line icons and black logos are. Such a
// picture shows only its shape: drawings take it in a color of theirs (ink by
// default), so it stays visible on a dark page as well. Anything with another
// color, white parts, a gradient or an embedded picture keeps its own colors.
func Monochrome(mime string, data []byte) bool {
	switch mime {
	case "image/svg+xml":
		return monoSVG(data)
	case "image/png":
		return monoPNG(data)
	}
	return false
}

// paints are the attributes and style properties that give a color.
var paints = map[string]bool{"fill": true, "stroke": true, "stop-color": true}

// cssPaint finds the colors a style sheet gives.
var cssPaint = regexp.MustCompile(`(?i)(?:^|[;{\s])(fill|stroke|stop-color)\s*:\s*([^;}]+)`)

func monoSVG(data []byte) bool {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	inStyle := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "linearGradient", "radialGradient", "pattern", "image":
				return false
			case "style":
				inStyle = true
			}
			for _, a := range t.Attr {
				if a.Name.Space == "" && paints[a.Name.Local] && !darkPaint(a.Value) {
					return false
				}
			}
		case xml.EndElement:
			if t.Name.Local == "style" {
				inStyle = false
			}
		case xml.CharData:
			if inStyle {
				for _, m := range cssPaint.FindAllStringSubmatch(string(t), -1) {
					if !darkPaint(m[2]) {
						return false
					}
				}
			}
		}
	}
}

// darkPaint reports whether a paint value draws nothing or a dark neutral.
func darkPaint(v string) bool {
	v = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important")))
	switch v {
	case "none", "transparent", "inherit", "currentcolor", "black":
		return true // currentColor is black where an icon is shown as a picture
	}
	c, ok := parseColor(v)
	return ok && darkNeutral(c)
}

// darkNeutral: dark enough to vanish on a dark page, and gray rather than a
// color of its own (a navy logo stays navy).
func darkNeutral(c color.NRGBA) bool {
	hi := max(c.R, c.G, c.B)
	lo := min(c.R, c.G, c.B)
	return luminance(c) < 0.1 && hi-lo <= 40
}

func luminance(c color.NRGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

var rgbFunc = regexp.MustCompile(`^rgba?\(\s*([\d.]+%?)[\s,]+([\d.]+%?)[\s,]+([\d.]+%?)(?:[\s,/]+[\d.]+%?)?\s*\)$`)

// parseColor reads hex and rgb() colors (the rest counts as another color).
func parseColor(v string) (color.NRGBA, bool) {
	if strings.HasPrefix(v, "#") {
		h := v[1:]
		if len(h) == 3 || len(h) == 4 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		} else if len(h) == 8 {
			h = h[:6]
		}
		n, err := strconv.ParseUint(h, 16, 32)
		if len(h) != 6 || err != nil {
			return color.NRGBA{}, false
		}
		return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, true
	}
	m := rgbFunc.FindStringSubmatch(v)
	if m == nil {
		return color.NRGBA{}, false
	}
	var out [3]uint8
	for i, s := range m[1:4] {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return color.NRGBA{}, false
		}
		if strings.HasSuffix(s, "%") {
			f = f * 255 / 100
		}
		out[i] = uint8(math.Max(0, math.Min(255, math.Round(f))))
	}
	return color.NRGBA{R: out[0], G: out[1], B: out[2], A: 255}, true
}

func monoPNG(data []byte) bool {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	b := img.Bounds()
	seen := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A < 24 {
				continue // (almost) transparent: the page shows through
			}
			if !darkNeutral(c) {
				return false
			}
			seen = true
		}
	}
	return seen && hasAlpha(img)
}

// hasAlpha: a picture without transparency is a dark rectangle, not a shape.
func hasAlpha(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xffff {
				return true
			}
		}
	}
	return false
}
