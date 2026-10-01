// Package design は社員証のレイアウト（座標・色）とフォントを定義する。
// 背景画像の生成（cmd/genbg）と社員証の合成（internal/card）の双方が
// ここを参照することで、背景の枠と差し込む値の位置がずれないようにする。
package design

import (
	_ "embed"
	"image"
	"image/color"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// 社員証のサイズ（クレジットカードと同じ縦横比）。
const (
	Width  = 1012
	Height = 638

	// CornerRadius は社員証の角丸の半径。
	CornerRadius = 24
)

// 色。
var (
	Navy      = color.RGBA{0x14, 0x21, 0x3d, 0xff}
	Gold      = color.RGBA{0xc9, 0xa2, 0x27, 0xff}
	Paper     = color.RGBA{0xf7, 0xf8, 0xfa, 0xff}
	Watermark = color.RGBA{0xe8, 0xec, 0xf2, 0xff}
	Rule      = color.RGBA{0xdd, 0xe2, 0xea, 0xff}
	Label     = color.RGBA{0x6b, 0x72, 0x80, 0xff}
	Ink       = color.RGBA{0x1f, 0x29, 0x37, 0xff}
	PhotoBack = color.RGBA{0xe3, 0xe7, 0xee, 0xff}
	White     = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// ヘッダー帯。
const (
	HeaderHeight = 104
	StripeHeight = 6
	FooterTop    = Height - 8
)

// PhotoRect は顔写真（3:4）を配置する領域。
var PhotoRect = image.Rect(48, 140, 48+216, 140+288)

// Field は社員証の 1 行分の項目。
type Field struct {
	Label     string
	Baseline  int // ラベルと値で共通のベースライン Y 座標
	ValueSize float64
	ValueBold bool
}

// 項目の X 座標。値は ValueX から ValueMaxX までに収める。
const (
	LabelX    = 296
	LabelSize = 18
	ValueX    = 420
	ValueMaxX = Width - 48
	// RuleOffset はベースラインから下線までの距離。
	RuleOffset = 18
)

// 各項目。並び順は社員証の上から。
var (
	FieldNumber     = Field{Label: "社員番号", Baseline: 182, ValueSize: 34, ValueBold: true}
	FieldName       = Field{Label: "氏　　名", Baseline: 252, ValueSize: 32, ValueBold: true}
	FieldBirthdate  = Field{Label: "生年月日", Baseline: 322, ValueSize: 28}
	FieldDepartment = Field{Label: "所　　属", Baseline: 392, ValueSize: 28}

	Fields = []Field{FieldNumber, FieldName, FieldBirthdate, FieldDepartment}
)

// MinValueSize は値が長いときに縮小できる下限の文字サイズ。
const MinValueSize = 18

// 備考欄。
var RemarksBox = image.Rect(48, 452, Width-48, 612)

const (
	RemarksLabel         = "備考"
	RemarksLabelSize     = 18
	RemarksLabelBaseline = 480
	RemarksPadding       = 16
	// RemarksTextTop は備考本文の 1 行目の上端。
	RemarksTextTop = 492
)

// RemarksSizes は備考本文に試す文字サイズ（大きい順）。
// 収まらなければ最小サイズで末尾を「…」にする。
var RemarksSizes = []float64{20, 18, 16}

//go:embed fonts/NotoSansJP-Regular.otf
var regularOTF []byte

//go:embed fonts/NotoSansJP-Bold.otf
var boldOTF []byte

var regular, bold = mustParse(regularOTF), mustParse(boldOTF)

func mustParse(b []byte) *opentype.Font {
	f, err := opentype.Parse(b)
	if err != nil {
		panic("design: 埋め込みフォントを読み込めません: " + err.Error())
	}
	return f
}

// Face は指定サイズのフォントフェイスを返す。
// font.Face は並行利用できないため、描画のたびに新しく作ること。
func Face(size float64, isBold bool) font.Face {
	f := regular
	if isBold {
		f = bold
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic("design: フォントフェイスを作成できません: " + err.Error())
	}
	return face
}
