package drawing

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"
)

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const goodSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M2 2h20v20H2z"/></svg>`

func TestReadPack(t *testing.T) {
	data := zipOf(t, map[string][]byte{
		"Office/Cloud/server.svg":  []byte(goodSVG),
		"Office/Cloud/db.png":      pngOf(t, 32, 32),
		"Office/printer.svg":       []byte(goodSVG),
		"Office/readme.txt":        []byte("hello"),
		"Office/evil.svg":          []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"Office/huge.png":          pngOf(t, 600, 10),
		"Office/.DS_Store":         []byte("x"),
		"__MACOSX/Office/._db.png": []byte("x"),
	})
	icons, skipped, err := ReadPack(data, "Office pack")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, i := range icons {
		got[i.Section+"|"+i.Name] = i.Mime
	}
	want := map[string]string{"Cloud|server": "image/svg+xml", "Cloud|db": "image/png", "Office|printer": "image/svg+xml"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("icons = %v, want %v", got, want)
	}
	reasons := map[string]bool{}
	for _, s := range skipped {
		reasons[s.File] = true
	}
	for _, f := range []string{"Office/readme.txt", "Office/evil.svg", "Office/huge.png"} {
		if !reasons[f] {
			t.Errorf("%s was not reported as skipped: %+v", f, skipped)
		}
	}
	if reasons["Office/.DS_Store"] || reasons["__MACOSX/Office/._db.png"] {
		t.Errorf("hidden files were reported: %+v", skipped)
	}
}

func TestReadPackLimits(t *testing.T) {
	if _, _, err := ReadPack([]byte("not a zip"), "x"); !errors.Is(err, ErrPack) {
		t.Errorf("not a zip: %v", err)
	}
	many := map[string][]byte{}
	for i := 0; i <= MaxPackIcons; i++ {
		many[fmt.Sprintf("i%d.svg", i)] = []byte(goodSVG)
	}
	if _, _, err := ReadPack(zipOf(t, many), "x"); !errors.Is(err, ErrPack) {
		t.Errorf("too many icons: %v", err)
	}
	// A picture that unpacks larger than an icon may be is skipped unread.
	big := zipOf(t, map[string][]byte{"big.svg": []byte(goodSVG[:len(goodSVG)-6] + strings.Repeat(" ", MaxIconBytes) + "</svg>")})
	icons, skipped, err := ReadPack(big, "x")
	if err != nil || len(icons) != 0 || len(skipped) != 1 {
		t.Errorf("big icon: %v %v %v", icons, skipped, err)
	}
}
