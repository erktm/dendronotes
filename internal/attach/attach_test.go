package attach

import (
	"strings"
	"testing"
)

func iso(brand string, compat ...string) string {
	boxSize := 16 + 4*len(compat)
	s := string([]byte{0, 0, 0, byte(boxSize)}) + "ftyp" + brand + "\x00\x00\x00\x00"
	for _, c := range compat {
		s += c
	}
	return s + "....mdat"
}

func TestCheckAcceptsMatchingContent(t *testing.T) {
	for name, head := range map[string]string{
		"photo.png":   "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
		"photo.JPG":   "\xff\xd8\xff\xe0\x00\x10JFIF",
		"anim.gif":    "GIF89a\x01\x00",
		"pic.webp":    "RIFF\x24\x00\x00\x00WEBPVP8 ",
		"pic.avif":    iso("avif", "mif1", "miaf"),
		"clip.mp4":    iso("isom", "iso2", "avc1", "mp41"),
		"clip.mov":    iso("qt  ", "qt  "),
		"voice.m4a":   iso("M4A ", "M4A ", "mp42", "isom"),
		"song.mp3":    "ID3\x04\x00",
		"raw.mp3":     "\xff\xfb\x90\x64",
		"sound.wav":   "RIFF\x24\x00\x00\x00WAVEfmt ",
		"sound.ogg":   "OggS\x00\x02",
		"sound.flac":  "fLaC\x00",
		"video.webm":  "\x1a\x45\xdf\xa3\x01\x00",
		"doc.pdf":     "%PDF-1.7\n",
		"icon.ico":    "\x00\x00\x01\x00\x01\x00",
		"pic.bmp":     "BM\x36\x00",
		"logo.svg":    "<?xml version=\"1.0\"?>\n<!-- made by hand -->\n<!DOCTYPE svg>\n<svg xmlns=\"http://www.w3.org/2000/svg\">",
		"plain.svg":   "\ufeff<svg viewBox=\"0 0 1 1\"/>",
		"Note.md":     "# Über\n\nText",
		"empty.md":    "",
		"data.csv":    "a,b\n1,2",
		"data.json":   `{"a": 1}`,
		"archive.zip": "PK\x03\x04\x14\x00",
		"cut-off.txt": "ok \xc3", // a character cut by the head limit
	} {
		if err := Check(name, []byte(head)); err != nil {
			t.Errorf("Check(%q) = %v", name, err)
		}
	}
}

func TestCheckRejectsMismatchesAndUnknownTypes(t *testing.T) {
	for name, head := range map[string]string{
		"photo.png":  "\xff\xd8\xff\xe0", // a JPEG named .png
		"page.html":  "<html><script>alert(1)</script>",
		"script.js":  "alert(1)",
		"tool.exe":   "MZ\x90\x00",
		"logo.svg":   "<html><svg>",
		"fake.svg":   "<svgx>",
		"binary.md":  "text\x00more",
		"latin1.txt": "caf\xe9 au lait",
		"doc.pdf":    "<html>%PDF-",
		"clip.mp4":   iso("qt  "),
		"pic.avif":   iso("isom", "mp41"),
		"noext":      "\x89PNG\r\n\x1a\n",
		"photo.png.": "\x89PNG\r\n\x1a\n",
		"pic.webp":   "RIFF\x24\x00\x00\x00WAVE",
		"short.mp4":  "\x00\x00\x00\x10ftyp",
	} {
		if err := Check(name, []byte(head)); err == nil {
			t.Errorf("Check(%q) accepted %q", name, head)
		}
	}
}

func TestServe(t *testing.T) {
	cases := map[string]Serving{
		"a.png":  {"image/png", true},
		"a.SVG":  {"image/svg+xml", true},
		"a.pdf":  {"application/pdf", true},
		"a.zip":  {"application/zip", false},
		"a.html": {"application/octet-stream", false},
		"a.docx": {"application/octet-stream", false},
		"a":      {"application/octet-stream", false},
	}
	for name, want := range cases {
		if got := Serve(name); got != want {
			t.Errorf("Serve(%q) = %+v, want %+v", name, got, want)
		}
	}
	for _, ext := range Extensions() {
		if ext != strings.ToLower(ext) || !strings.HasPrefix(ext, ".") {
			t.Errorf("odd extension %q", ext)
		}
		if ext == ".html" || ext == ".htm" || ext == ".js" || ext == ".xhtml" {
			t.Errorf("%s must never be allowed", ext)
		}
	}
}

func FuzzCheck(f *testing.F) {
	f.Add("a.svg", "<svg>")
	f.Add("a.mp4", iso("isom"))
	f.Add("a.txt", "\xc3")
	f.Fuzz(func(t *testing.T, name, head string) {
		_ = Check(name, []byte(head)) // must not panic
	})
}
