package avatar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withText inserts a tEXt chunk (metadata) right after the PNG header chunk.
func withText(data []byte, text string) []byte {
	chunk := func(kind string, body []byte) []byte {
		var c bytes.Buffer
		_ = binary.Write(&c, binary.BigEndian, uint32(len(body)))
		c.WriteString(kind)
		c.Write(body)
		_ = binary.Write(&c, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(kind), body...)))
		return c.Bytes()
	}
	const ihdrEnd = 8 + 4 + 4 + 13 + 4 // signature, length, type, data, CRC
	out := append([]byte{}, data[:ihdrEnd]...)
	out = append(out, chunk("tEXt", []byte("Comment\x00"+text))...)
	return append(out, data[ihdrEnd:]...)
}

func TestNormalize(t *testing.T) {
	src := withText(encodePNG(t, 256, 256), "taken at home")
	if !bytes.Contains(src, []byte("taken at home")) {
		t.Fatal("test photo lacks its metadata")
	}
	out, err := Normalize(src)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("taken at home")) || bytes.Contains(out, []byte("tEXt")) {
		t.Error("metadata survived")
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil || img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatalf("result: %v %v", img.Bounds(), err)
	}
	if got := color.NRGBAModel.Convert(img.At(10, 20)).(color.NRGBA); got != (color.NRGBA{10, 20, 120, 255}) {
		t.Errorf("pixels changed: %v", got)
	}
}

func TestNormalizeRefuses(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 128, 128)), nil); err != nil {
		t.Fatal(err)
	}
	big := encodePNG(t, 128, 128)
	for name, data := range map[string][]byte{
		"not square":   encodePNG(t, 128, 100),
		"too small":    encodePNG(t, MinSize-1, MinSize-1),
		"too large":    encodePNG(t, MaxSize+1, MaxSize+1),
		"jpeg":         jpg.Bytes(),
		"garbage":      []byte("<svg onload=alert(1)>"),
		"truncated":    big[:len(big)/2],
		"empty":        nil,
		"over the cap": append(big, make([]byte, MaxBytes)...),
	} {
		if _, err := Normalize(data); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}
