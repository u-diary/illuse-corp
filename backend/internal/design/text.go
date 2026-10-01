package design

import (
	"image"
	"image/color"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Ellipsis は収まらない文字列の末尾に付ける記号。
const Ellipsis = "…"

// DrawText は (x, baseline) を左端・ベースラインとして文字列を描く。
func DrawText(dst *image.RGBA, face font.Face, c color.Color, x, baseline int, s string) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, baseline)}
	d.DrawString(s)
}

// DrawTextRight は right を右端として文字列を描く。
func DrawTextRight(dst *image.RGBA, face font.Face, c color.Color, right, baseline int, s string) {
	DrawText(dst, face, c, right-TextWidth(face, s), baseline, s)
}

// TextWidth は文字列の描画幅（ピクセル、切り上げ）を返す。
func TextWidth(face font.Face, s string) int {
	return font.MeasureString(face, s).Ceil()
}

// FitLine は s を maxWidth に収まる最大のサイズで描くフェイスを返す。
// MinValueSize まで縮めても収まらない場合は、末尾を「…」で省略した文字列を返す。
func FitLine(s string, maxWidth int, size float64, isBold bool) (font.Face, string) {
	for ; size > MinValueSize; size -= 2 {
		face := Face(size, isBold)
		if TextWidth(face, s) <= maxWidth {
			return face, s
		}
	}
	face := Face(MinValueSize, isBold)
	return face, Truncate(face, s, maxWidth)
}

// Truncate は s が maxWidth に収まるよう末尾を「…」で省略する。
func Truncate(face font.Face, s string, maxWidth int) string {
	if TextWidth(face, s) <= maxWidth {
		return s
	}
	return withEllipsis(face, s, maxWidth)
}

// noLineStart は行頭に置かない文字（行頭禁則）。
const noLineStart = "、。，．,.)）」』】〕〉》！？!?ー～ァィゥェォッャュョヮヵヶぁぃぅぇぉっゃゅょゎゕゖ・：；"

// Wrap は s を maxWidth ごとに折り返した行を返す。改行文字はそのまま改行として扱う。
// 行頭禁則の文字は前の行の末尾にぶら下げる（その行だけ maxWidth をわずかに超えうる）。
func Wrap(face font.Face, s string, maxWidth int) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		var line []rune
		width := fixed.Int26_6(0)
		limit := fixed.I(maxWidth)
		for _, r := range para {
			adv, ok := face.GlyphAdvance(r)
			if !ok {
				adv, _ = face.GlyphAdvance('□')
			}
			if len(line) > 0 && width+adv > limit && !strings.ContainsRune(noLineStart, r) {
				lines = append(lines, string(line))
				line, width = nil, 0
			}
			line = append(line, r)
			width += adv
		}
		lines = append(lines, string(line))
	}
	// 末尾の空行は描画しても意味がないので落とす。
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// LineHeight は文字サイズに対する行の高さを返す。
func LineHeight(size float64) int {
	return int(size*1.5 + 0.5)
}

// FitParagraph は備考のような複数行テキストを、幅 maxWidth・高さ maxHeight の枠に
// 収まる最大のサイズ（sizes の先頭から順に試す）で折り返す。
// どのサイズでも収まらない場合は最小サイズで行数を切り詰め、最終行の末尾を「…」にする。
func FitParagraph(s string, maxWidth, maxHeight int, sizes []float64) (font.Face, float64, []string) {
	for i, size := range sizes {
		face := Face(size, false)
		lines := Wrap(face, s, maxWidth)
		maxLines := maxHeight / LineHeight(size)
		if len(lines) <= maxLines {
			return face, size, lines
		}
		if i == len(sizes)-1 {
			lines = lines[:maxLines]
			lines[maxLines-1] = withEllipsis(face, lines[maxLines-1], maxWidth)
			return face, size, lines
		}
	}
	panic("design: sizes が空です")
}

// withEllipsis は s の末尾に「…」を付け、maxWidth に収まるまで本文を削る。
func withEllipsis(face font.Face, s string, maxWidth int) string {
	for {
		t := s + Ellipsis
		if s == "" || TextWidth(face, t) <= maxWidth {
			return t
		}
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
}
