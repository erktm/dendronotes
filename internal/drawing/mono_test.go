package drawing

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestMonochromeSVG(t *testing.T) {
	cases := []struct {
		name string
		svg  string
		want bool
	}{
		{"no colors (black)", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M1 1h4v4z"/></svg>`, true},
		{"near-black fills", `<svg xmlns="http://www.w3.org/2000/svg"><path fill="#231E21" d="M0 0h1"/><path fill="#000" stroke="black" d="M0 0h1"/></svg>`, true},
		{"line icon", `<svg xmlns="http://www.w3.org/2000/svg"><path fill="none" stroke="#1a1a1a" d="M0 0h1"/></svg>`, true},
		{"style sheet", `<svg xmlns="http://www.w3.org/2000/svg"><style>.a{fill:#222;stroke:none}</style><path class="a" d="M0 0h1"/></svg>`, true},
		{"rgb()", `<svg xmlns="http://www.w3.org/2000/svg"><path fill="rgb(20, 20, 22)" d="M0 0h1"/></svg>`, true},
		{"white parts", `<svg xmlns="http://www.w3.org/2000/svg"><rect fill="#000" width="4" height="4"/><path fill="#fff" d="M0 0h1"/></svg>`, false},
		{"a color", `<svg xmlns="http://www.w3.org/2000/svg"><path fill="#326ce5" d="M0 0h1"/></svg>`, false},
		{"navy is a color", `<svg xmlns="http://www.w3.org/2000/svg"><path fill="#0f2546" d="M0 0h1"/></svg>`, false},
		{"mid gray stays", `<svg xmlns="http://www.w3.org/2000/svg"><path fill="#717074" d="M0 0h1"/></svg>`, false},
		{"color in the style sheet", `<svg xmlns="http://www.w3.org/2000/svg"><style>.a{fill:#000}.b{fill:#e00}</style><path d="M0 0h1"/></svg>`, false},
		{"gradient", `<svg xmlns="http://www.w3.org/2000/svg"><linearGradient id="g"><stop offset="0" stop-color="#000"/></linearGradient><path fill="url(#g)" d="M0 0h1"/></svg>`, false},
		{"named color", `<svg xmlns="http://www.w3.org/2000/svg"><path fill="red" d="M0 0h1"/></svg>`, false},
	}
	for _, c := range cases {
		if err := CheckIcon([]byte(c.svg)); err != nil {
			t.Fatalf("%s: not a valid icon: %v", c.name, err)
		}
		if got := Monochrome("image/svg+xml", []byte(c.svg)); got != c.want {
			t.Errorf("%s: Monochrome = %v, want %v", c.name, got, c.want)
		}
	}
}

func paintedPNG(t *testing.T, paint func(x, y int) color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, paint(x, y))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMonochromePNG(t *testing.T) {
	clear := color.NRGBA{}
	black := color.NRGBA{R: 10, G: 10, B: 12, A: 255}
	shape := func(c color.NRGBA) func(x, y int) color.NRGBA {
		return func(x, y int) color.NRGBA {
			if x > 1 && x < 6 && y > 1 && y < 6 {
				return c
			}
			return clear
		}
	}
	if !Monochrome("image/png", paintedPNG(t, shape(black))) {
		t.Error("a black shape on transparency is monochrome")
	}
	if Monochrome("image/png", paintedPNG(t, shape(color.NRGBA{R: 220, G: 30, B: 30, A: 255}))) {
		t.Error("a red shape keeps its color")
	}
	if Monochrome("image/png", paintedPNG(t, func(int, int) color.NRGBA { return black })) {
		t.Error("a picture without transparency keeps its look")
	}
	if Monochrome("image/png", paintedPNG(t, func(int, int) color.NRGBA { return clear })) {
		t.Error("an empty picture is not monochrome")
	}
}
