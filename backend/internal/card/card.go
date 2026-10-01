// Package card は背景画像に入力内容を差し込んで社員証の画像を合成する。
package card

//go:generate go run ../../cmd/genbg -o assets/background.png

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/draw"
	"image/png"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/design"
)

//go:embed assets/background.png
var backgroundPNG []byte

var background = sync.OnceValue(func() *image.RGBA {
	img, err := png.Decode(bytes.NewReader(backgroundPNG))
	if err != nil {
		panic("card: 背景画像を読み込めません: " + err.Error())
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return rgba
})

// Data は社員証に載せる内容。
type Data struct {
	EmployeeNumber string
	Name           string
	Birthdate      time.Time
	Department     string
	Remarks        string
	Photo          image.Image
}

// Render は社員証の画像を合成する。
func Render(d Data) *image.RGBA {
	bg := background()
	img := image.NewRGBA(bg.Bounds())
	copy(img.Pix, bg.Pix)

	drawPhoto(img, d.Photo)

	for _, v := range []struct {
		field design.Field
		text  string
	}{
		{design.FieldNumber, d.EmployeeNumber},
		{design.FieldName, d.Name},
		{design.FieldBirthdate, d.Birthdate.Format("2006年01月02日")},
		{design.FieldDepartment, d.Department},
	} {
		face, s := design.FitLine(v.text, design.ValueMaxX-design.ValueX, v.field.ValueSize, v.field.ValueBold)
		design.DrawText(img, face, design.Ink, design.ValueX, v.field.Baseline, s)
	}

	drawRemarks(img, d.Remarks)
	return img
}

// TextChunk は PNG に tEXt チャンクとして埋め込むメタデータ。
// Key は 1〜79 文字、Key・Value とも ASCII の印字可能文字に限る。
type TextChunk struct {
	Key   string
	Value string
}

// pngHeaderLen は PNG シグネチャ（8 バイト）と IHDR チャンク（長さ 4 + 種別 4 + データ 13 + CRC 4）の長さ。
const pngHeaderLen = 8 + 4 + 4 + 13 + 4

// EncodePNG は画像を PNG にエンコードし、meta を tEXt チャンクとして埋め込む。
// image/png はテキストチャンクを書けないため、IHDR の直後に差し込む。
func EncodePNG(img image.Image, meta ...TextChunk) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if len(meta) == 0 {
		return b, nil
	}

	var out bytes.Buffer
	out.Grow(len(b) + 128)
	out.Write(b[:pngHeaderLen])
	for _, m := range meta {
		if err := writeTextChunk(&out, m); err != nil {
			return nil, err
		}
	}
	out.Write(b[pngHeaderLen:])
	return out.Bytes(), nil
}

func writeTextChunk(w *bytes.Buffer, m TextChunk) error {
	if len(m.Key) == 0 || len(m.Key) > 79 || !isPrintableASCII(m.Key) || !isPrintableASCII(m.Value) {
		return fmt.Errorf("card: tEXt チャンクに使えないキーまたは値です: %q=%q", m.Key, m.Value)
	}
	data := make([]byte, 0, len(m.Key)+1+len(m.Value))
	data = append(data, m.Key...)
	data = append(data, 0) // キーと値の区切り
	data = append(data, m.Value...)

	crc := crc32.NewIEEE()
	crc.Write([]byte("tEXt"))
	crc.Write(data)

	w.Write(binary.BigEndian.AppendUint32(nil, uint32(len(data))))
	w.WriteString("tEXt")
	w.Write(data)
	w.Write(binary.BigEndian.AppendUint32(nil, crc.Sum32()))
	return nil
}

// ReadTextChunks は PNG から tEXt チャンクを読み出す。各チャンクの CRC も検証する。
func ReadTextChunks(b []byte) ([]TextChunk, error) {
	if !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		return nil, errors.New("card: PNG ではありません")
	}
	var chunks []TextChunk
	for p := 8; p < len(b); {
		if p+12 > len(b) {
			return nil, errors.New("card: チャンクが途中で切れています")
		}
		n := int(binary.BigEndian.Uint32(b[p:]))
		if p+12+n > len(b) {
			return nil, errors.New("card: チャンクが途中で切れています")
		}
		typ, data := b[p+4:p+8], b[p+8:p+8+n]
		if crc32.ChecksumIEEE(b[p+4:p+8+n]) != binary.BigEndian.Uint32(b[p+8+n:]) {
			return nil, fmt.Errorf("card: %s チャンクの CRC が一致しません", typ)
		}
		if string(typ) == "tEXt" {
			key, value, ok := bytes.Cut(data, []byte{0})
			if !ok {
				return nil, errors.New("card: tEXt チャンクの形式が正しくありません")
			}
			chunks = append(chunks, TextChunk{Key: string(key), Value: string(value)})
		}
		p += 12 + n
	}
	return chunks, nil
}

func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// drawPhoto は写真を中央基準で 3:4 に切り抜き、写真枠に合わせて縮小して貼る。
// 透過 PNG でも社員証が透けないよう、枠の背景色の上に重ねる。
func drawPhoto(dst *image.RGBA, photo image.Image) {
	dr := design.PhotoRect
	xdraw.CatmullRom.Scale(dst, dr, photo, CropToAspect(photo.Bounds(), dr.Dx(), dr.Dy()), draw.Over, nil)
}

// CropToAspect は src の中央から w:h の比率で切り抜く範囲を返す。
func CropToAspect(src image.Rectangle, w, h int) image.Rectangle {
	sw, sh := src.Dx(), src.Dy()
	if sw*h > sh*w {
		// 横長すぎるので左右を削る。
		cw := sh * w / h
		x0 := src.Min.X + (sw-cw)/2
		return image.Rect(x0, src.Min.Y, x0+cw, src.Max.Y)
	}
	// 縦長すぎるので上下を削る。
	ch := sw * h / w
	y0 := src.Min.Y + (sh-ch)/2
	return image.Rect(src.Min.X, y0, src.Max.X, y0+ch)
}

func drawRemarks(dst *image.RGBA, remarks string) {
	if remarks == "" {
		return
	}
	box := design.RemarksBox
	left := box.Min.X + design.RemarksPadding
	maxWidth := box.Dx() - design.RemarksPadding*2
	maxHeight := box.Max.Y - design.RemarksPadding/2 - design.RemarksTextTop

	face, size, lines := design.FitParagraph(remarks, maxWidth, maxHeight, design.RemarksSizes)
	lh := design.LineHeight(size)
	// 行の高さの中で全角文字の枠（ベースラインの 0.88em 上から 0.12em 下まで）を上下中央に置く。
	baseline := design.RemarksTextTop + int(float64(lh)-size)/2 + int(size*0.88)
	for _, line := range lines {
		design.DrawText(dst, face, design.Ink, left, baseline, line)
		baseline += lh
	}
}
