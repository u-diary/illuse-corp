package api

import (
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // image.Decode で JPEG を扱うため
	_ "image/png"  // image.Decode で PNG を扱うため
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// JST は日本標準時（日本には夏時間がないので固定オフセットで足りる）。
var JST = time.FixedZone("Asia/Tokyo", 9*60*60)

// 入力の上限。
const (
	MaxUploadBytes   = 6 << 20 // リクエスト全体
	MaxNameLength    = 40      // 文字数
	MaxRemarksLength = 200     // 文字数
	MinPhotoSide     = 64      // ピクセル
	MaxPhotoPixels   = 24_000_000
)

// departments は選択肢の値と、社員証・DB に記録する部署名の対応。
var departments = map[string]string{
	"sales":       "営業部",
	"engineering": "技術開発部",
	"operations":  "業務統括部",
	"none":        "業務統括部クレーム対応室",
}

var minBirthdate = time.Date(1900, 1, 1, 0, 0, 0, 0, JST)

// Input は検証済みの入力内容。
type Input struct {
	Name       string
	Birthdate  time.Time
	Department string // 社員証に印字する部署名
	Remarks    string

	// photo はヘッダーまで検証済みで、まだデコードしていない顔写真。
	photo multipart.File
}

// Close はアップロードされた写真を閉じる。
func (in *Input) Close() error {
	return in.photo.Close()
}

// DecodePhoto は顔写真をデコードする。メモリを多く使うので、呼び出し側で同時実行数を絞ること。
func (in *Input) DecodePhoto() (image.Image, error) {
	img, _, err := image.Decode(in.photo)
	if err != nil {
		return nil, invalid("photo", "顔写真を読み込めませんでした。別の画像をお試しください。")
	}
	return img, nil
}

// ValidationError は利用者に見せる入力エラー。
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"error"`
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// parseInput はマルチパートフォームを読み取って検証する。
// 成功した場合、呼び出し側は Input.Close を呼ぶこと。
func parseInput(r *http.Request, now time.Time) (*Input, error) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if errors.Is(err, http.ErrNotMultipart) || errors.Is(err, multipart.ErrMessageTooLarge) {
			return nil, invalid("", "フォームの形式が正しくありません。")
		}
		return nil, err
	}

	var in Input
	var err error

	if in.Name, err = parseName(r.FormValue("name")); err != nil {
		return nil, err
	}
	if in.Birthdate, err = parseBirthdate(r.FormValue("birthdate"), now); err != nil {
		return nil, err
	}
	var ok bool
	if in.Department, ok = departments[r.FormValue("department")]; !ok {
		return nil, invalid("department", "所属部署を選択してください。")
	}
	if in.Remarks, err = parseRemarks(r.FormValue("remarks")); err != nil {
		return nil, err
	}
	if v := r.FormValue("agreement"); v != "true" && v != "on" {
		return nil, invalid("agreement", "誓約事項への同意が必要です。")
	}

	f, _, err := r.FormFile("photo")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, invalid("photo", "顔写真をアップロードしてください。")
		}
		return nil, err
	}
	if err := checkPhoto(f); err != nil {
		f.Close()
		return nil, err
	}
	in.photo = f
	return &in, nil
}

func parseName(s string) (string, error) {
	s = strings.TrimSpace(s) // 全角スペースも除去される
	switch {
	case s == "":
		return "", invalid("name", "氏名を入力してください。")
	case !utf8.ValidString(s) || strings.IndexFunc(s, unicode.IsControl) >= 0:
		return "", invalid("name", "氏名に使用できない文字が含まれています。")
	case utf8.RuneCountInString(s) > MaxNameLength:
		return "", invalid("name", "氏名は%d文字以内で入力してください。", MaxNameLength)
	}
	return s, nil
}

func parseBirthdate(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, invalid("birthdate", "生年月日を入力してください。")
	}
	d, err := time.ParseInLocation(time.DateOnly, s, JST)
	if err != nil {
		return time.Time{}, invalid("birthdate", "生年月日の形式が正しくありません。")
	}
	if d.Before(minBirthdate) || d.After(now.In(JST)) {
		return time.Time{}, invalid("birthdate", "生年月日が正しくありません。")
	}
	return d, nil
}

func parseRemarks(s string) (string, error) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRightFunc(s, unicode.IsSpace)
	if !utf8.ValidString(s) || strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) >= 0 {
		return "", invalid("remarks", "備考に使用できない文字が含まれています。")
	}
	if utf8.RuneCountInString(s) > MaxRemarksLength {
		return "", invalid("remarks", "備考は%d文字以内で入力してください。", MaxRemarksLength)
	}
	return s, nil
}

// checkPhoto は写真のヘッダーだけを読んで形式と寸法を確かめ、読み取り位置を先頭に戻す。
// 展開後のサイズが巨大な画像でメモリを使い果たさないよう、デコード前に行う。
func checkPhoto(f io.ReadSeeker) error {
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || (format != "jpeg" && format != "png") {
		return invalid("photo", "顔写真は JPEG または PNG 形式でアップロードしてください。")
	}
	if cfg.Width < MinPhotoSide || cfg.Height < MinPhotoSide {
		return invalid("photo", "顔写真が小さすぎます（%dピクセル以上にしてください）。", MinPhotoSide)
	}
	if cfg.Width*cfg.Height > MaxPhotoPixels {
		return invalid("photo", "顔写真の解像度が大きすぎます。")
	}
	_, err = f.Seek(0, io.SeekStart)
	return err
}
