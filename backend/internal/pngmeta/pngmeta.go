// Package pngmeta は PNG の tEXt チャンク（テキストのメタデータ）を読み書きする。
// 標準の image/png はテキストチャンクを扱えないため、チャンク列を直接操作する。
package pngmeta

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
)

// Chunk は tEXt チャンク 1 つ分のキーと値。
// Key は 1〜79 文字、Key・Value とも ASCII の印字可能文字に限る。
type Chunk struct {
	Key   string
	Value string
}

// Signature は PNG ファイルの先頭 8 バイト。
const Signature = "\x89PNG\r\n\x1a\n"

// headerLen は PNG シグネチャ（8 バイト）と IHDR チャンク（長さ 4 + 種別 4 + データ 13 + CRC 4）の長さ。
const headerLen = 8 + 4 + 4 + 13 + 4

// ErrNotPNG は PNG ではないデータを渡されたときのエラー。
var ErrNotPNG = errors.New("pngmeta: PNG ではありません")

// IsPNG は b が PNG のシグネチャで始まるかを返す。
func IsPNG(b []byte) bool {
	return bytes.HasPrefix(b, []byte(Signature))
}

// Encode は画像を PNG にエンコードし、chunks を tEXt チャンクとして IHDR の直後に埋め込む。
func Encode(img image.Image, chunks ...Chunk) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if len(chunks) == 0 {
		return b, nil
	}

	var out bytes.Buffer
	out.Grow(len(b) + 256)
	out.Write(b[:headerLen])
	for _, c := range chunks {
		if err := writeText(&out, c); err != nil {
			return nil, err
		}
	}
	out.Write(b[headerLen:])
	return out.Bytes(), nil
}

func writeText(w *bytes.Buffer, c Chunk) error {
	if len(c.Key) == 0 || len(c.Key) > 79 || !isPrintableASCII(c.Key) || !isPrintableASCII(c.Value) {
		return fmt.Errorf("pngmeta: tEXt チャンクに使えないキーまたは値です: %q=%q", c.Key, c.Value)
	}
	data := make([]byte, 0, len(c.Key)+1+len(c.Value))
	data = append(data, c.Key...)
	data = append(data, 0) // キーと値の区切り
	data = append(data, c.Value...)

	crc := crc32.NewIEEE()
	crc.Write([]byte("tEXt"))
	crc.Write(data)

	w.Write(binary.BigEndian.AppendUint32(nil, uint32(len(data))))
	w.WriteString("tEXt")
	w.Write(data)
	w.Write(binary.BigEndian.AppendUint32(nil, crc.Sum32()))
	return nil
}

// Read は PNG から tEXt チャンクを順に読み出す。各チャンクの CRC も検証する。
func Read(b []byte) ([]Chunk, error) {
	if !IsPNG(b) {
		return nil, ErrNotPNG
	}
	var chunks []Chunk
	for p := len(Signature); p < len(b); {
		if p+12 > len(b) {
			return nil, errors.New("pngmeta: チャンクが途中で切れています")
		}
		n := int(binary.BigEndian.Uint32(b[p:]))
		if n < 0 || n > len(b)-p-12 {
			return nil, errors.New("pngmeta: チャンクが途中で切れています")
		}
		typ, data := b[p+4:p+8], b[p+8:p+8+n]
		if crc32.ChecksumIEEE(b[p+4:p+8+n]) != binary.BigEndian.Uint32(b[p+8+n:]) {
			return nil, fmt.Errorf("pngmeta: %s チャンクの CRC が一致しません", typ)
		}
		if string(typ) == "tEXt" {
			key, value, ok := bytes.Cut(data, []byte{0})
			if !ok {
				return nil, errors.New("pngmeta: tEXt チャンクの形式が正しくありません")
			}
			chunks = append(chunks, Chunk{Key: string(key), Value: string(value)})
		}
		if string(typ) == "IEND" {
			break
		}
		p += 12 + n
	}
	return chunks, nil
}

// Lookup は chunks から key の値を探す。同じキーが複数ある場合は ok=false を返す
// （どちらを信じるべきか決められないため）。
func Lookup(chunks []Chunk, key string) (value string, ok bool) {
	found := 0
	for _, c := range chunks {
		if c.Key == key {
			value = c.Value
			found++
		}
	}
	return value, found == 1
}

func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}
