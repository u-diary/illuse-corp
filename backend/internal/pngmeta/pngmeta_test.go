package pngmeta

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 80, 60))
	for y := range 60 {
		for x := range 80 {
			img.Set(x, y, color.RGBA{uint8(x * 3), uint8(y * 4), 160, 255})
		}
	}
	return img
}

func TestEncodeAndRead(t *testing.T) {
	img := testImage()
	chunks := []Chunk{{"EmployeeNumber", "000042"}, {"Birthdate", "2001-03-15"}}
	b, err := Encode(img, chunks...)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Read(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != chunks[0] || got[1] != chunks[1] {
		t.Errorf("Read = %+v, want %+v", got, chunks)
	}

	// チャンクは IHDR の直後（IDAT より前）に入り、画像としても読める。
	if typ := string(b[headerLen+4 : headerLen+8]); typ != "tEXt" {
		t.Errorf("IHDR の次のチャンク = %q, want tEXt", typ)
	}
	decoded, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("メタデータ入りの PNG を読めない: %v", err)
	}
	if decoded.Bounds() != img.Bounds() {
		t.Errorf("bounds = %v", decoded.Bounds())
	}
}

func TestEncodeRejectsInvalidChunks(t *testing.T) {
	for _, c := range []Chunk{
		{"", "x"},
		{strings.Repeat("k", 80), "x"},
		{"Name", "山田"}, // tEXt は Latin-1 のみ
		{"Key", "a\x00b"},
	} {
		if _, err := Encode(testImage(), c); err == nil {
			t.Errorf("%q=%q がエラーにならない", c.Key, c.Value)
		}
	}
}

func TestReadRejectsBrokenData(t *testing.T) {
	b, err := Encode(testImage(), Chunk{"Key", "value"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Read([]byte("GIF89a...")); err != ErrNotPNG {
		t.Errorf("PNG 以外: err = %v, want ErrNotPNG", err)
	}
	if _, err := Read(b[:headerLen+10]); err == nil {
		t.Error("途中で切れたデータがエラーにならない")
	}

	// tEXt の値を書き換えると CRC が合わなくなる。
	tampered := bytes.Replace(b, []byte("value"), []byte("VALUE"), 1)
	if _, err := Read(tampered); err == nil {
		t.Error("改ざんされたチャンクがエラーにならない")
	}
}

func TestLookup(t *testing.T) {
	chunks := []Chunk{{"A", "1"}, {"B", "2"}, {"B", "3"}}
	if v, ok := Lookup(chunks, "A"); !ok || v != "1" {
		t.Errorf("A = %q, %v", v, ok)
	}
	if _, ok := Lookup(chunks, "B"); ok {
		t.Error("重複したキーは ok=false")
	}
	if _, ok := Lookup(chunks, "C"); ok {
		t.Error("ないキーは ok=false")
	}
}
