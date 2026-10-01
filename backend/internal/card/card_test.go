package card

import (
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/design"
)

func TestCropToAspect(t *testing.T) {
	tests := []struct {
		name string
		src  image.Rectangle
		want image.Rectangle
	}{
		{"横長は左右を削る", image.Rect(0, 0, 800, 600), image.Rect(175, 0, 625, 600)},
		{"縦長は上下を削る", image.Rect(0, 0, 300, 600), image.Rect(0, 100, 300, 500)},
		{"ちょうど3:4", image.Rect(0, 0, 300, 400), image.Rect(0, 0, 300, 400)},
		{"原点がずれていても中央", image.Rect(10, 20, 810, 620), image.Rect(185, 20, 635, 620)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CropToAspect(tt.src, 3, 4); got != tt.want {
				t.Errorf("CropToAspect(%v) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

// TestRender は合成が最後まで通ることを確かめる。
// CARD_SAMPLE_DIR を指定すると、目視確認用のサンプル画像をそこへ書き出す。
func TestRender(t *testing.T) {
	samples := map[string]Data{
		"basic": {
			EmployeeNumber: "000001",
			Name:           "山田 太郎",
			Birthdate:      time.Date(2000, 4, 1, 0, 0, 0, 0, time.UTC),
			Department:     "技術開発部",
			Remarks:        "趣味は登山とコーヒーの焙煎です。よろしくお願いします。",
			Photo:          gradient(800, 600),
		},
		"long": {
			EmployeeNumber: "000123",
			Name:           "アレクサンドラ・フォン・エインズワース＝ミュンヒハウゼン・ヴィルヘルミーナ",
			Birthdate:      time.Date(1876, 12, 31, 0, 0, 0, 0, time.UTC),
			Department:     "業務統括部クレーム対応室",
			Remarks:        strings.Repeat("備考欄に長い文章を書いた場合の折り返しと省略を確認します。", 8),
			Photo:          gradient(300, 900),
		},
		"empty-remarks": {
			EmployeeNumber: "999999",
			Name:           "佐藤 花子",
			Birthdate:      time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC),
			Department:     "営業部",
			Photo:          gradient(64, 64),
		},
	}

	dir := os.Getenv("CARD_SAMPLE_DIR")
	for name, d := range samples {
		t.Run(name, func(t *testing.T) {
			img := Render(d)
			if got := img.Bounds(); got != image.Rect(0, 0, design.Width, design.Height) {
				t.Fatalf("bounds = %v", got)
			}
			b, err := EncodePNG(img)
			if err != nil {
				t.Fatal(err)
			}
			if dir != "" {
				if err := os.WriteFile(dir+"/"+name+".png", b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func gradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(255 * x / w), uint8(255 * y / h), 160, 255})
		}
	}
	return img
}
