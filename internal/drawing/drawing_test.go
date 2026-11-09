package drawing

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const good = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="-10 -10 420 320" width="420" height="320">
<style>.k-ink{stroke:#1f2328}@media (prefers-color-scheme: dark){.k-ink{stroke:#e6e6e6}}</style>
<metadata id="dendronotes-drawing">{"version":1,"elements":[{"id":"a","type":"rect","text":"&lt;b&gt;"}]}</metadata>
<g class="el"><rect x="0" y="0" width="100" height="60" rx="8" class="k-ink" fill="none" stroke-width="2"/>
<text x="50" y="30" text-anchor="middle" font-family="ui-sans-serif, system-ui, sans-serif" font-size="16">a &lt; b</text>
<path d="M0 0 L10 10" stroke-dasharray="6 4"/>
<image href="data:image/png;base64,iVBORw0KGgo=" x="0" y="0" width="32" height="32" preserveAspectRatio="xMidYMid meet"/>
<rect fill="url(#grad)" width="1" height="1"/></g>
</svg>`

func TestCheckAcceptsDrawings(t *testing.T) {
	if err := Check([]byte(good)); err != nil {
		t.Fatal(err)
	}
	if err := Check(Empty()); err != nil {
		t.Errorf("empty drawing: %v", err)
	}
	// Drawings written by the editor in the browser (shared test files,
	// written by web/src/lib/drawing/svg.test.ts).
	files, err := filepath.Glob("../../web/src/lib/testdata/drawing*.svg")
	if err != nil || len(files) < 2 {
		t.Fatalf("shared drawings: %v %v", files, err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := Check(data); err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
		}
	}
}

func TestCheckRefusesActiveContent(t *testing.T) {
	wrap := func(inner string) string {
		return `<svg xmlns="http://www.w3.org/2000/svg"><metadata>{}</metadata>` + inner + `</svg>`
	}
	for name, svg := range map[string]string{
		"script":              wrap(`<script>alert(1)</script>`),
		"event handler":       wrap(`<rect onload="alert(1)"/>`),
		"external image":      wrap(`<image href="https://evil.example/x.png"/>`),
		"xlink external":      `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><metadata/><image xlink:href="https://evil.example/x.png"/></svg>`,
		"javascript href":     wrap(`<image href="javascript:alert(1)"/>`),
		"foreignObject":       wrap(`<foreignObject><div xmlns="http://www.w3.org/1999/xhtml">x</div></foreignObject>`),
		"html element":        wrap(`<iframe xmlns="http://www.w3.org/1999/xhtml" src="x"/>`),
		"css url":             wrap(`<style>.a{fill:url(https://evil.example/x)}</style>`),
		"css url in cdata":    wrap(`<style><![CDATA[.a{fill:ur]]><![CDATA[l(https://evil.example/)}]]></style>`),
		"css import":          wrap(`<style>@import "x.css";</style>`),
		"style attribute":     wrap(`<rect style="fill:red"/>`),
		"fill to another url": wrap(`<rect fill="url(https://evil.example/#a)"/>`),
		"doctype":             `<!DOCTYPE svg [<!ENTITY x "y">]>` + wrap(``),
		"use outside":         wrap(`<use href="https://evil.example/s.svg#a"/>`),
		"animate":             wrap(`<rect><set attributeName="fill" to="red"/></rect>`),
		"two roots":           wrap(``) + `<svg/>`,
		"not svg":             `<html/>`,
		"no scene":            `<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"broken":              `<svg xmlns="http://www.w3.org/2000/svg"><metadata>`,
	} {
		if err := Check([]byte(svg)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: accepted (%v)", name, err)
		}
	}
	if err := Check(bytes.Repeat([]byte(" "), MaxBytes+1)); !errors.Is(err, ErrInvalid) {
		t.Error("too large accepted")
	}
}

func TestIsDrawing(t *testing.T) {
	for p, want := range map[string]bool{"Plan.drawing.svg": true, "a/B.DRAWING.SVG": true, "Plan.svg": false, "drawing.svg": false} {
		if IsDrawing(p) != want {
			t.Errorf("IsDrawing(%q) = %v", p, !want)
		}
	}
}

func TestNormalizeIcon(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 64, 32))); err != nil {
		t.Fatal(err)
	}
	if mime, out, err := NormalizeIcon(b.Bytes()); err != nil || mime != "image/png" || !bytes.HasPrefix(out, []byte("\x89PNG")) {
		t.Errorf("png: %s %v", mime, err)
	}
	b.Reset()
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 600, 10))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NormalizeIcon(b.Bytes()); err == nil {
		t.Error("too wide png accepted")
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M2 2h20v20H2z"/></svg>`
	if mime, _, err := NormalizeIcon([]byte(svg)); err != nil || mime != "image/svg+xml" {
		t.Errorf("svg icon: %s %v", mime, err)
	}
	if _, _, err := NormalizeIcon([]byte(strings.Replace(svg, "<path", `<path onclick="x()"`, 1))); err == nil {
		t.Error("svg icon with an event handler accepted")
	}
	if _, _, err := NormalizeIcon([]byte("GIF89a")); err == nil {
		t.Error("gif accepted")
	}
}
