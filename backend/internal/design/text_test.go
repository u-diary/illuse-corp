package design

import (
	"strings"
	"testing"
)

func TestWrap(t *testing.T) {
	face := Face(20, false)
	// 全角 1 文字 = 20px なので、幅 100px にはちょうど 5 文字入る。
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"折り返し", "あいうえおかきくけこさし", []string{"あいうえお", "かきくけこ", "さし"}},
		{"改行を保つ", "あい\nうえ\r\nお", []string{"あい", "うえ", "お"}},
		{"行頭禁則は前の行にぶら下げる", "あいうえお。かき", []string{"あいうえお。", "かき"}},
		{"末尾の空行は落とす", "あい\n\n", []string{"あい"}},
		{"空文字列", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Wrap(face, tt.in, 100)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") || len(got) != len(tt.want) {
				t.Errorf("Wrap(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFitLine(t *testing.T) {
	_, s := FitLine("短い", 200, 32, true)
	if s != "短い" {
		t.Errorf("短い文字列はそのまま: got %q", s)
	}

	long := strings.Repeat("長", 100)
	face, s := FitLine(long, 200, 32, true)
	if !strings.HasSuffix(s, Ellipsis) {
		t.Errorf("収まらない文字列は省略される: got %q", s)
	}
	if w := TextWidth(face, s); w > 200 {
		t.Errorf("省略後の幅 %d が 200 を超えている", w)
	}
}

func TestFitParagraph(t *testing.T) {
	_, size, lines := FitParagraph("あいう", 100, 100, []float64{20, 16})
	if size != 20 || len(lines) != 1 {
		t.Errorf("収まるなら最大サイズ: size=%v lines=%q", size, lines)
	}

	face, size, lines := FitParagraph(strings.Repeat("あ", 100), 100, 100, []float64{20, 16})
	if size != 16 {
		t.Errorf("収まらなければ最小サイズ: size=%v", size)
	}
	if max := 100 / LineHeight(16); len(lines) != max {
		t.Errorf("行数は枠の上限で切り詰め: got %d, want %d", len(lines), max)
	}
	last := lines[len(lines)-1]
	if !strings.HasSuffix(last, Ellipsis) || TextWidth(face, last) > 100 {
		t.Errorf("最終行は … で終わり幅に収まる: %q (%dpx)", last, TextWidth(face, last))
	}
}
