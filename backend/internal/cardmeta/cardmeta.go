// Package cardmeta は社員証の PNG に埋め込むメタデータ（社員番号・生年月日・署名）を扱う。
//
// 署名は社員番号と生年月日の組を HMAC-SHA256 で署名したもので、サーバー B が発行し、
// サーバー C が検証する。秘密鍵を知らない第三者は、任意の画像に有効なメタデータを書き込めない。
package cardmeta

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/u-diary/illuse-corp/backend/internal/pngmeta"
)

// tEXt チャンクのキー。
const (
	KeyEmployeeNumber = "EmployeeNumber" // 例: 000001
	KeyBirthdate      = "Birthdate"      // 入力された生年月日（YYYY-MM-DD）
	KeySignature      = "Signature"      // 例: v1.<HMAC-SHA256 の 16 進数>
)

const (
	signatureVersion = "v1."
	// MinKeyBytes は秘密鍵の最小の長さ。
	MinKeyBytes = 32
)

// ErrInvalid はメタデータがない・形式が正しくない・署名が一致しない場合のエラー。
var ErrInvalid = errors.New("cardmeta: 社員証のメタデータが正しくありません")

var employeeNumberPattern = regexp.MustCompile(`^[0-9]{6,}$`)

// Identity は署名で保証された社員番号と生年月日。
type Identity struct {
	EmployeeNumber string
	Birthdate      string // YYYY-MM-DD
}

// Signer は社員証のメタデータに署名し、検証する。
type Signer struct {
	key []byte
}

// NewSigner は秘密鍵から Signer を作る。
func NewSigner(key []byte) (*Signer, error) {
	if len(key) < MinKeyBytes {
		return nil, fmt.Errorf("cardmeta: 秘密鍵は %d バイト以上必要です", MinKeyBytes)
	}
	return &Signer{key: append([]byte(nil), key...)}, nil
}

// LoadSigner は 16 進数で書かれた秘密鍵のファイルを読み込む（`openssl rand -hex 32` で作れる）。
func LoadSigner(path string) (*Signer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cardmeta: 秘密鍵を読み込めません: %w", err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("cardmeta: 秘密鍵は 16 進数で記述してください: %w", err)
	}
	return NewSigner(key)
}

func (s *Signer) sign(id Identity) string {
	mac := hmac.New(sha256.New, s.key)
	// 区切りに NUL を使い、値の境界をずらした別の組と同じ署名にならないようにする。
	mac.Write([]byte("illuse-card\x00" + id.EmployeeNumber + "\x00" + id.Birthdate))
	return signatureVersion + hex.EncodeToString(mac.Sum(nil))
}

// Chunks は社員証の PNG に埋め込むチャンク（社員番号・生年月日・署名）を返す。
func (s *Signer) Chunks(id Identity) []pngmeta.Chunk {
	return []pngmeta.Chunk{
		{Key: KeyEmployeeNumber, Value: id.EmployeeNumber},
		{Key: KeyBirthdate, Value: id.Birthdate},
		{Key: KeySignature, Value: s.sign(id)},
	}
}

// Verify はチャンクから社員番号と生年月日を取り出し、署名を検証する。
// いずれかが欠けている・重複している・形式が正しくない・署名が一致しない場合は ErrInvalid を返す。
func (s *Signer) Verify(chunks []pngmeta.Chunk) (Identity, error) {
	number, ok1 := pngmeta.Lookup(chunks, KeyEmployeeNumber)
	birthdate, ok2 := pngmeta.Lookup(chunks, KeyBirthdate)
	sig, ok3 := pngmeta.Lookup(chunks, KeySignature)
	if !ok1 || !ok2 || !ok3 {
		return Identity{}, ErrInvalid
	}
	if !employeeNumberPattern.MatchString(number) {
		return Identity{}, ErrInvalid
	}
	if _, err := time.Parse(time.DateOnly, birthdate); err != nil {
		return Identity{}, ErrInvalid
	}
	id := Identity{EmployeeNumber: number, Birthdate: birthdate}
	if !hmac.Equal([]byte(sig), []byte(s.sign(id))) {
		return Identity{}, ErrInvalid
	}
	return id, nil
}
