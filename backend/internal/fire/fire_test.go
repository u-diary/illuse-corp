package fire

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/cardmeta"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/pngmeta"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/ratelimit"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/store"
)

const origin = "https://illuse-corp.u-diary.art"

var fixedNow = time.Date(2026, 10, 2, 10, 15, 30, 0, jst)

func signer(t *testing.T, seed byte) *cardmeta.Signer {
	t.Helper()
	s, err := cardmeta.NewSigner(bytes.Repeat([]byte{seed}, cardmeta.MinKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type env struct {
	handler http.Handler
	store   *store.Store
	signer  *cardmeta.Signer
}

// newEnv は社員を n 人発行済みの DB でハンドラーを作る。
func newEnv(t *testing.T, employees int) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for range employees {
		if _, err := st.Issue(context.Background(), "営業部", fixedNow, func(int64) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	sg := signer(t, 7)
	h := NewHandler(Config{
		Store:          st,
		Signer:         sg,
		Limiter:        ratelimit.New(100, time.Minute),
		AllowedOrigins: []string{origin},
		Now:            func() time.Time { return fixedNow },
	})
	return &env{handler: h, store: st, signer: sg}
}

func cardImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 1012, 638))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 0xf7, 0xf8, 0xfa, 0xff
	}
	return img
}

// cardPNG は onboard が発行するのと同じ形式（署名済みメタデータ入り）の PNG を作る。
func cardPNG(t *testing.T, chunks ...pngmeta.Chunk) []byte {
	t.Helper()
	b, err := pngmeta.Encode(cardImage(), chunks...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *env) validCard(t *testing.T, number string) []byte {
	return cardPNG(t, e.signer.Chunks(cardmeta.Identity{EmployeeNumber: number, Birthdate: "2001-03-15"})...)
}

func (e *env) post(t *testing.T, action, datetime string, img []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("action", action)
	mw.WriteField("client_datetime", datetime)
	if img != nil {
		fw, _ := mw.CreateFormFile("image", "card.png")
		fw.Write(img)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/stamps", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	req.RemoteAddr = "127.0.0.1:50000"
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var v struct{ Error string }
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("JSON を読めない: %v (status %d)", err, rec.Code)
	}
	return v.Error
}

func TestStamp(t *testing.T) {
	e := newEnv(t, 3)
	for i, a := range Actions {
		number := store.FormatNumber(int64(i + 1))
		rec := e.post(t, a.Code, "2026/10/02 19:05", e.validCard(t, number))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", a.Code, rec.Code, rec.Body)
		}
		if got := rec.Header().Get("Content-Type"); got != "image/png" {
			t.Errorf("%s: Content-Type = %q", a.Code, got)
		}
		if got := rec.Header().Get(EmployeeNumberHeader); got != number {
			t.Errorf("%s: 社員番号 = %q", a.Code, got)
		}

		out := rec.Body.Bytes()
		chunks, err := pngmeta.Read(out)
		if err != nil {
			t.Fatal(err)
		}
		if id, err := e.signer.Verify(chunks); err != nil || id.EmployeeNumber != number {
			t.Errorf("%s: メタデータが引き継がれていない: %+v, %v", a.Code, id, err)
		}
		img, _, err := image.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Size() != (image.Point{1012, 638}) {
			t.Errorf("%s: サイズ = %v", a.Code, img.Bounds())
		}
		if !hasRed(img, image.Rect(506, 319, 1012, 638)) {
			t.Errorf("%s: 右下にスタンプがない", a.Code)
		}

		r, err := e.store.Get(context.Background(), int64(i+1))
		if err != nil {
			t.Fatal(err)
		}
		if r.Action != a.Label || r.ActionAt != "2026-10-02T10:15:30+09:00" {
			t.Errorf("%s: DB = %+v", a.Code, r)
		}
	}
}

func TestStampAlreadyProcessed(t *testing.T) {
	e := newEnv(t, 1)
	card := e.validCard(t, "000001")
	first := e.post(t, "resignation", "2026/10/02 19:05", card)
	if first.Code != http.StatusOK {
		t.Fatalf("1 回目: status = %d, body = %s", first.Code, first.Body)
	}

	// 同じ社員証も、スタンプを押した後の画像も、処理済みとして受け付けない。
	for name, img := range map[string][]byte{"同じ社員証": card, "スタンプ済みの画像": first.Body.Bytes()} {
		rec := e.post(t, "crime", "2026/10/02 19:06", img)
		if rec.Code != http.StatusConflict || errorMessage(t, rec) != MsgAlreadyProcessed {
			t.Errorf("%s: status = %d", name, rec.Code)
		}
	}
	if r, _ := e.store.Get(context.Background(), 1); r.Action != "退職届の受理" {
		t.Errorf("1 回目の記録が上書きされた: %+v", r)
	}
}

func TestStampRejectsInvalidImages(t *testing.T) {
	e := newEnv(t, 1)
	id := cardmeta.Identity{EmployeeNumber: "000001", Birthdate: "2001-03-15"}
	valid := e.signer.Chunks(id)

	var jpg bytes.Buffer
	jpeg.Encode(&jpg, cardImage(), nil)

	tampered := append([]pngmeta.Chunk(nil), valid...)
	tampered[1].Value = "2001-03-16" // 生年月日を書き換え

	tests := map[string][]byte{
		"JPEG":          jpg.Bytes(),
		"画像でない":         []byte("hello"),
		"メタデータなし":       cardPNG(t),
		"社員番号と生年月日だけ":   cardPNG(t, valid[0], valid[1]),
		"別の鍵で署名":        cardPNG(t, signer(t, 9).Chunks(id)...),
		"生年月日を書き換え":     cardPNG(t, tampered...),
		"DB にいない社員番号":   e.validCard(t, "000099"),
		"PNG の途中で切れている": e.validCard(t, "000001")[:200],
	}
	for name, img := range tests {
		t.Run(name, func(t *testing.T) {
			rec := e.post(t, "crime", "2026/10/02 19:05", img)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
			}
			if msg := errorMessage(t, rec); msg != MsgInvalidImage {
				t.Errorf("message = %q", msg)
			}
		})
	}
	if r, _ := e.store.Get(context.Background(), 1); r.Action != "" {
		t.Errorf("不正な画像で記録された: %+v", r)
	}
}

func TestStampRejectsInvalidForm(t *testing.T) {
	e := newEnv(t, 1)
	card := e.validCard(t, "000001")
	tests := []struct {
		name, action, datetime string
		img                    []byte
	}{
		{"処理内容なし", "", "2026/10/02 19:05", card},
		{"処理内容が選択肢にない", "promotion", "2026/10/02 19:05", card},
		{"日時なし", "crime", "", card},
		{"日時の形式違い", "crime", "2026-10-02T19:05", card},
		{"画像なし", "crime", "2026/10/02 19:05", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := e.post(t, tt.action, tt.datetime, tt.img); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, body = %s", rec.Code, rec.Body)
			}
		})
	}
	if r, _ := e.store.Get(context.Background(), 1); r.Action != "" {
		t.Errorf("入力エラーで記録された: %+v", r)
	}
}

func TestStampForbiddenOrigin(t *testing.T) {
	e := newEnv(t, 1)
	var body bytes.Buffer
	req := httptest.NewRequest(http.MethodPost, "/api/stamps", &body)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d", rec.Code)
	}
}

func hasRed(img image.Image, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if int(c.R)-int(c.G) > 80 && int(c.R)-int(c.B) > 80 {
				return true
			}
		}
	}
	return false
}
