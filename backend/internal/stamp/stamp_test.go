package stamp

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func solid(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 0xf7, 0xf8, 0xfa, 0xff
	}
	return img
}

func isReddish(c color.RGBA) bool {
	return int(c.R)-int(c.G) > 60 && int(c.R)-int(c.B) > 60
}

// redBounds は赤くなった画素の外接矩形を返す。
func redBounds(img *image.RGBA) image.Rectangle {
	var r image.Rectangle
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isReddish(img.RGBAAt(x, y)) {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

var sample = Content{Title: "懲戒解雇", DateTime: "2026/10/01 21:45", Note: "事由：犯罪行為の発覚"}

// TestApply はスタンプが右下に収まり、元の画像を変更しないことを確かめる。
// STAMP_SAMPLE_DIR を指定すると、目視確認用のサンプル画像を書き出す。
func TestApply(t *testing.T) {
	for name, size := range map[string]image.Point{
		"card":   {1012, 638},
		"small":  {320, 200},
		"tall":   {600, 1600},
		"square": {2000, 2000},
	} {
		t.Run(name, func(t *testing.T) {
			src := solid(size.X, size.Y)
			before := append([]byte(nil), src.Pix...)

			out := Apply(src, sample)

			if string(src.Pix) != string(before) {
				t.Error("元の画像が変更されている")
			}
			if out.Bounds().Size() != size {
				t.Fatalf("サイズが変わった: %v", out.Bounds())
			}
			red := redBounds(out)
			if red.Empty() {
				t.Fatal("スタンプが押されていない")
			}
			// 右下の領域に収まり、はみ出していない。
			if red.Min.X < size.X/3 || red.Min.Y < size.Y/3 {
				t.Errorf("スタンプが右下にない: %v（画像 %v）", red, size)
			}
			if red.Max.X >= size.X || red.Max.Y >= size.Y {
				t.Errorf("スタンプが画像の端にかかっている: %v（画像 %v）", red, size)
			}
			if dir := os.Getenv("STAMP_SAMPLE_DIR"); dir != "" {
				f, err := os.Create(filepath.Join(dir, name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				png.Encode(f, out)
				f.Close()
			}
		})
	}
}

func TestApplyKeepsAspectWithOffsetBounds(t *testing.T) {
	src := solid(400, 300).SubImage(image.Rect(50, 50, 350, 250))
	out := Apply(src, sample)
	if out.Bounds() != image.Rect(0, 0, 300, 200) {
		t.Errorf("bounds = %v", out.Bounds())
	}
}
