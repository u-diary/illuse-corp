// genbg は社員証の背景画像を生成する。生成した画像はリポジトリに含めて
// internal/card に埋め込むため、デザインを変えたときだけ実行すればよい。
//
//	go generate ./internal/card
package main

import (
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"

	"github.com/u-diary/illuse-corp/backend/internal/design"
)

func main() {
	out := flag.String("o", "background.png", "出力先の PNG ファイル")
	flag.Parse()

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, render()); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func render() *image.RGBA {
	const w, h = design.Width, design.Height
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(img, img.Bounds(), design.Paper)

	// 透かし：右側に同心円の模様を薄く敷く。
	cx, cy := float64(w-40), 300.0
	for y := design.HeaderHeight; y < h; y++ {
		for x := w / 2; x < w; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d >= 28 && d < 300 && math.Mod(d, 28) < 2 {
				img.SetRGBA(x, y, design.Watermark)
			}
		}
	}

	// ヘッダー帯。
	fill(img, image.Rect(0, 0, w, design.HeaderHeight), design.Sky)
	fill(img, image.Rect(0, design.HeaderHeight, w, design.HeaderHeight+design.StripeHeight), design.Gold)
	// 社名は 1 行なので帯の上下中央に置く。
	design.DrawText(img, design.Face(36, true), design.White, 48, 66, "ILLUSE Corp.")
	design.DrawTextRight(img, design.Face(34, true), design.White, w-48, 58, "社員証")
	design.DrawTextRight(img, design.Face(15, false), design.White, w-48, 88, "EMPLOYEE  ID  CARD")

	// 顔写真の枠。
	fill(img, design.PhotoRect.Inset(-2), design.Rule)
	fill(img, design.PhotoRect, design.PhotoBack)

	// 各項目のラベルと下線。
	labelFace := design.Face(design.LabelSize, false)
	for _, f := range design.Fields {
		design.DrawText(img, labelFace, design.Label, design.LabelX, f.Baseline, f.Label)
		y := f.Baseline + design.RuleOffset
		fill(img, image.Rect(design.LabelX, y, design.ValueMaxX, y+1), design.Rule)
	}

	// 備考欄。
	box := design.RemarksBox
	fill(img, box, design.Rule)
	fill(img, box.Inset(1), design.White)
	fill(img, image.Rect(box.Min.X, box.Min.Y, box.Min.X+4, box.Max.Y), design.Sky)
	design.DrawText(img, design.Face(design.RemarksLabelSize, true), design.SkyInk,
		box.Min.X+design.RemarksPadding, design.RemarksLabelBaseline, design.RemarksLabel)

	// フッター帯。
	fill(img, image.Rect(0, design.FooterTop, w, h), design.Sky)

	roundCorners(img, design.CornerRadius)
	return img
}

func fill(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
}

// roundCorners は四隅を半径 r で切り抜き、外側を透明にする（縁は簡易的にアンチエイリアス）。
func roundCorners(img *image.RGBA, r int) {
	b := img.Bounds()
	rf := float64(r)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			// 最寄りの角の円の中心。
			cx, cy := -1.0, -1.0
			switch {
			case x < b.Min.X+r:
				cx = float64(b.Min.X + r)
			case x >= b.Max.X-r:
				cx = float64(b.Max.X - r)
			}
			switch {
			case y < b.Min.Y+r:
				cy = float64(b.Min.Y + r)
			case y >= b.Max.Y-r:
				cy = float64(b.Max.Y - r)
			}
			if cx < 0 || cy < 0 {
				continue
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			coverage := math.Max(0, math.Min(1, rf-d+0.5))
			if coverage == 1 {
				continue
			}
			c := img.RGBAAt(x, y)
			// RGBA はアルファ乗算済みなので全成分に掛ける。
			c.R = uint8(float64(c.R) * coverage)
			c.G = uint8(float64(c.G) * coverage)
			c.B = uint8(float64(c.B) * coverage)
			c.A = uint8(float64(c.A) * coverage)
			img.SetRGBA(x, y, c)
		}
	}
}
