// Package stamp は画像の右下に判子風の赤いスタンプを押す。
package stamp

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand/v2"

	"golang.org/x/image/font"
	"golang.org/x/image/math/f64"

	xdraw "golang.org/x/image/draw"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/design"
)

// Content はスタンプに書く 3 行。
type Content struct {
	Title    string // 1 行目（大きく）: 例「退職済」
	DateTime string // 2 行目: 例「2026/10/01 21:45」
	Note     string // 3 行目: 例「退職届を受理しました」
}

// スタンプの原寸（この大きさで描いてから、押す画像に合わせて縮小・回転する）。
const (
	baseWidth  = 600
	baseHeight = 330
)

// 押し方。
const (
	// WidthRatio は押す画像の幅に対するスタンプの幅。
	WidthRatio = 0.32
	// MinWidth は小さな画像でも文字が読めるようにする下限の幅。
	MinWidth = 180
	// Angle はスタンプの傾き（度、反時計回りが正）。
	Angle = 8.0
	// MarginRatio は画像の短辺に対する、右端・下端からの余白。
	MarginRatio = 0.04
)

// Red はスタンプの朱色。
var Red = color.RGBA{0xd7, 0x19, 0x2c, 0xff}

// Apply は src の右下に c のスタンプを押した画像を返す（src は変更しない）。
func Apply(src image.Image, c Content) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)

	stamp := render(c)

	// 縮小率と配置。回転後の外接矩形が画像の右下に収まるようにする。
	w := math.Max(MinWidth, float64(b.Dx())*WidthRatio)
	w = math.Min(w, float64(b.Dx())*0.9)
	scale := w / baseWidth
	rad := Angle * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	sw, sh := baseWidth*scale, baseHeight*scale
	boundW := sw*cos + sh*sin
	boundH := sw*sin + sh*cos
	margin := math.Min(float64(b.Dx()), float64(b.Dy())) * MarginRatio
	cx := float64(b.Dx()) - margin - boundW/2
	cy := float64(b.Dy()) - margin - boundH/2

	// スタンプの中心を (cx, cy) に置き、scale 倍して反時計回りに回転する変換。
	// 画像の y 軸は下向きなので、反時計回りは sin の符号が通常と逆になる。
	m := f64.Aff3{
		scale * cos, scale * sin, 0,
		-scale * sin, scale * cos, 0,
	}
	m[2] = cx - (m[0]*baseWidth/2 + m[1]*baseHeight/2)
	m[5] = cy - (m[3]*baseWidth/2 + m[4]*baseHeight/2)
	xdraw.CatmullRom.Transform(dst, m, stamp, stamp.Bounds(), draw.Over, nil)
	return dst
}

// render は原寸のスタンプを透明な背景に描く。
func render(c Content) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, baseWidth, baseHeight))
	w, h := float64(baseWidth), float64(baseHeight)

	// 二重の角丸枠。
	strokeRoundRect(img, 8, 8, w-8, h-8, 34, 11)
	strokeRoundRect(img, 26, 26, w-26, h-26, 18, 3.5)

	// 1 行目（文字の間隔を空けて判子らしく）。
	title := design.Face(92, true)
	drawCentered(img, title, 138, c.Title, 14)

	// 区切り線。
	fillRect(img, 64, 160, w-64, 164)
	fillRect(img, 64, 226, w-64, 229)

	// 2 行目・3 行目。
	drawCentered(img, design.Face(36, true), 208, c.DateTime, 0)
	drawCentered(img, design.Face(36, true), 276, c.Note, 2)

	weather(img)
	return img
}

// drawCentered は文字列を水平方向の中央に描く。spacing は文字の間に足すピクセル数。
func drawCentered(img *image.RGBA, face font.Face, baseline int, s string, spacing int) {
	runes := []rune(s)
	width := 0
	for i, r := range runes {
		width += design.TextWidth(face, string(r))
		if i > 0 {
			width += spacing
		}
	}
	x := (baseWidth - width) / 2
	for _, r := range runes {
		design.DrawText(img, face, Red, x, baseline, string(r))
		x += design.TextWidth(face, string(r)) + spacing
	}
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 float64) {
	draw.Draw(img, image.Rect(int(x0), int(y0), int(x1), int(y1)), image.NewUniform(Red), image.Point{}, draw.Over)
}

// strokeRoundRect は (x0, y0)-(x1, y1) の角丸矩形の枠線を、内側に thickness の太さで描く（縁はアンチエイリアス）。
func strokeRoundRect(img *image.RGBA, x0, y0, x1, y1, radius, thickness float64) {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hw, hh := (x1-x0)/2, (y1-y0)/2
	for y := int(y0); y < int(math.Ceil(y1)); y++ {
		for x := int(x0); x < int(math.Ceil(x1)); x++ {
			d := roundRectDistance(float64(x)+0.5-cx, float64(y)+0.5-cy, hw, hh, radius)
			// 枠線は -thickness <= d <= 0 の帯。帯の両端を 1px でぼかす。
			coverage := clamp01(0.5-d) * clamp01(d+thickness+0.5)
			if coverage > 0 {
				blend(img, x, y, coverage)
			}
		}
	}
}

// roundRectDistance は中心を原点とする角丸矩形（半幅 hw, 半高 hh, 角の半径 r）までの符号付き距離（内側が負）。
func roundRectDistance(px, py, hw, hh, r float64) float64 {
	qx := math.Abs(px) - (hw - r)
	qy := math.Abs(py) - (hh - r)
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	inside := math.Min(math.Max(qx, qy), 0)
	return outside + inside - r
}

// blend は (x, y) に朱色を coverage の割合で重ねる。
func blend(img *image.RGBA, x, y int, coverage float64) {
	dst := img.RGBAAt(x, y)
	a := coverage
	mix := func(d, s uint8) uint8 { return uint8(float64(s)*a + float64(d)*(1-a) + 0.5) }
	img.SetRGBA(x, y, color.RGBA{mix(dst.R, Red.R), mix(dst.G, Red.G), mix(dst.B, Red.B), mix(dst.A, 0xff)})
}

// weather はインクのかすれを表現するため、濃さに細かなむらと小さな欠けを付ける。
// 同じ内容なら毎回同じ見た目になるよう、乱数の種は固定する。
func weather(img *image.RGBA) {
	rng := rand.New(rand.NewPCG(20261001, 1))
	b := img.Bounds()

	// 欠け：ところどころに小さな円形の穴を開ける。
	type hole struct{ x, y, r float64 }
	holes := make([]hole, 260)
	for i := range holes {
		holes[i] = hole{rng.Float64() * baseWidth, rng.Float64() * baseHeight, 0.6 + rng.Float64()*2.2}
	}

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			// 全体を少し透かし、細かなむらを付ける。
			k := 0.86 + 0.14*rng.Float64()
			for _, h := range holes {
				if d := math.Hypot(float64(x)-h.x, float64(y)-h.y); d < h.r {
					k *= d / h.r
				}
			}
			img.SetRGBA(x, y, color.RGBA{scale8(c.R, k), scale8(c.G, k), scale8(c.B, k), scale8(c.A, k)})
		}
	}
}

func scale8(v uint8, k float64) uint8 { return uint8(float64(v)*k + 0.5) }

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
