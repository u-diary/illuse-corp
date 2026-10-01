package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/card"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/ratelimit"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/store"
)

const origin = "https://illuse-corp.u-diary.art"

var fixedNow = time.Date(2026, 10, 1, 15, 4, 5, 0, JST)

type env struct {
	handler http.Handler
	store   *store.Store
}

func newEnv(t *testing.T, limit int) *env {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := NewHandler(Config{
		Store:          s,
		Limiter:        ratelimit.New(limit, time.Minute),
		AllowedOrigins: []string{origin},
		Now:            func() time.Time { return fixedNow },
	})
	return &env{handler: h, store: s}
}

func jpegPhoto(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func validFields() map[string]string {
	return map[string]string{
		"name":       "山田 太郎",
		"birthdate":  "2000-04-01",
		"department": "engineering",
		"remarks":    "よろしくお願いします。",
		"agreement":  "true",
	}
}

func newRequest(t *testing.T, fields map[string]string, photo []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	if photo != nil {
		fw, err := mw.CreateFormFile("photo", "photo.jpg")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(photo)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/cards", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	req.RemoteAddr = "127.0.0.1:50000"
	return req
}

func (e *env) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func TestCreateCard(t *testing.T) {
	e := newEnv(t, 100)
	photo := jpegPhoto(t, 300, 400)

	for i, tc := range []struct {
		department string
		wantNumber string
		wantDept   string
	}{
		{"engineering", "000001", "技術開発部"},
		{"none", "000002", "業務統括部クレーム対応室"},
	} {
		fields := validFields()
		fields["department"] = tc.department
		rec := e.do(newRequest(t, fields, photo))

		if rec.Code != http.StatusOK {
			t.Fatalf("[%d] status = %d, body = %s", i, rec.Code, rec.Body)
		}
		if got := rec.Header().Get("Content-Type"); got != "image/png" {
			t.Errorf("[%d] Content-Type = %q", i, got)
		}
		if got := rec.Header().Get(EmployeeNumberHeader); got != tc.wantNumber {
			t.Errorf("[%d] 社員番号 = %q, want %q", i, got, tc.wantNumber)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("[%d] ACAO = %q", i, got)
		}
		if got := rec.Header().Get("Access-Control-Expose-Headers"); got != EmployeeNumberHeader {
			t.Errorf("[%d] Expose-Headers = %q", i, got)
		}
		body := rec.Body.Bytes()
		meta, err := card.ReadTextChunks(body)
		if err != nil {
			t.Fatalf("[%d] メタデータを読めない: %v", i, err)
		}
		wantMeta := []card.TextChunk{
			{Key: MetaEmployeeNumber, Value: tc.wantNumber},
			{Key: MetaBirthdate, Value: "2000-04-01"},
		}
		if len(meta) != 2 || meta[0] != wantMeta[0] || meta[1] != wantMeta[1] {
			t.Errorf("[%d] メタデータ = %+v, want %+v", i, meta, wantMeta)
		}
		img, err := png.Decode(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("[%d] PNG として読めない: %v", i, err)
		}
		if img.Bounds().Dx() != 1012 || img.Bounds().Dy() != 638 {
			t.Errorf("[%d] 画像サイズ = %v", i, img.Bounds())
		}
	}

	records, err := e.store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []store.Record{
		{Number: 1, Department: "技術開発部", GeneratedAt: "2026-10-01T15:04:05+09:00"},
		{Number: 2, Department: "業務統括部クレーム対応室", GeneratedAt: "2026-10-01T15:04:05+09:00"},
	}
	if len(records) != 2 || records[0] != want[0] || records[1] != want[1] {
		t.Errorf("records = %+v, want %+v", records, want)
	}
}

func TestCreateCardValidation(t *testing.T) {
	photo := jpegPhoto(t, 300, 400)
	tests := []struct {
		name      string
		modify    func(map[string]string)
		photo     []byte
		wantField string
	}{
		{"氏名なし", func(f map[string]string) { f["name"] = "　 " }, photo, "name"},
		{"氏名が長すぎる", func(f map[string]string) { f["name"] = strings.Repeat("あ", 41) }, photo, "name"},
		{"氏名に制御文字", func(f map[string]string) { f["name"] = "山田\n太郎" }, photo, "name"},
		{"生年月日なし", func(f map[string]string) { delete(f, "birthdate") }, photo, "birthdate"},
		{"生年月日の形式違い", func(f map[string]string) { f["birthdate"] = "2000/04/01" }, photo, "birthdate"},
		{"生年月日が未来", func(f map[string]string) { f["birthdate"] = "2026-10-02" }, photo, "birthdate"},
		{"生年月日が古すぎる", func(f map[string]string) { f["birthdate"] = "1899-12-31" }, photo, "birthdate"},
		{"部署なし", func(f map[string]string) { delete(f, "department") }, photo, "department"},
		{"部署が選択肢にない", func(f map[string]string) { f["department"] = "人事部" }, photo, "department"},
		{"備考が長すぎる", func(f map[string]string) { f["remarks"] = strings.Repeat("あ", 201) }, photo, "remarks"},
		{"同意なし", func(f map[string]string) { delete(f, "agreement") }, photo, "agreement"},
		{"写真なし", func(map[string]string) {}, nil, "photo"},
		{"写真が画像でない", func(map[string]string) {}, []byte("not an image"), "photo"},
		{"写真が小さすぎる", func(map[string]string) {}, jpegPhoto(t, 32, 32), "photo"},
	}

	e := newEnv(t, 100)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := validFields()
			tt.modify(fields)
			rec := e.do(newRequest(t, fields, tt.photo))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
			}
			var ve ValidationError
			if err := json.NewDecoder(rec.Body).Decode(&ve); err != nil {
				t.Fatal(err)
			}
			if ve.Field != tt.wantField || ve.Message == "" {
				t.Errorf("error = %+v, want field %q", ve, tt.wantField)
			}
		})
	}

	// 入力エラーでは社員番号を消費しない。
	rec := e.do(newRequest(t, validFields(), photo))
	if got := rec.Header().Get(EmployeeNumberHeader); got != "000001" {
		t.Errorf("入力エラーの後の社員番号 = %q, want 000001", got)
	}
}

func TestOptionalRemarks(t *testing.T) {
	e := newEnv(t, 100)
	fields := validFields()
	delete(fields, "remarks")
	if rec := e.do(newRequest(t, fields, jpegPhoto(t, 300, 400))); rec.Code != http.StatusOK {
		t.Fatalf("備考なしでも発行できる: status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestForbiddenOrigin(t *testing.T) {
	e := newEnv(t, 100)
	for _, o := range []string{"", "https://evil.example", "https://u-diary.github.io"} {
		req := newRequest(t, validFields(), jpegPhoto(t, 300, 400))
		req.Header.Set("Origin", o)
		rec := e.do(req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("Origin %q: status = %d", o, rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("Origin %q: ACAO が付いている", o)
		}
	}
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t, 2)
	photo := jpegPhoto(t, 300, 400)
	send := func(ip string) int {
		req := newRequest(t, validFields(), photo)
		req.Header.Set("X-Forwarded-For", ip)
		return e.do(req).Code
	}
	if send("203.0.113.1") != http.StatusOK || send("203.0.113.1") != http.StatusOK {
		t.Fatal("上限までは成功する")
	}
	if got := send("203.0.113.1"); got != http.StatusTooManyRequests {
		t.Errorf("上限超過: status = %d", got)
	}
	if got := send("203.0.113.2"); got != http.StatusOK {
		t.Errorf("別の IP は影響を受けない: status = %d", got)
	}
}

func TestTooLarge(t *testing.T) {
	e := newEnv(t, 100)
	fields := validFields()
	fields["padding"] = strings.Repeat("x", MaxUploadBytes)
	rec := e.do(newRequest(t, fields, jpegPhoto(t, 300, 400)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestPreflight(t *testing.T) {
	e := newEnv(t, 100)
	req := httptest.NewRequest(http.MethodOptions, "/api/cards", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := e.do(req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Errorf("許可した Origin: status = %d, headers = %v", rec.Code, rec.Header())
	}

	req.Header.Set("Origin", "https://evil.example")
	if rec := e.do(req); rec.Code != http.StatusForbidden {
		t.Errorf("許可していない Origin: status = %d", rec.Code)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		remote, xff, want string
	}{
		{"127.0.0.1:1234", "198.51.100.7", "198.51.100.7"},
		{"127.0.0.1:1234", "1.2.3.4, 198.51.100.7", "198.51.100.7"}, // 先頭は偽装されうる
		{"[::1]:1234", "198.51.100.7", "198.51.100.7"},
		{"127.0.0.1:1234", "", "127.0.0.1"},
		{"192.0.2.1:1234", "198.51.100.7", "192.0.2.1"}, // プロキシ経由でなければ XFF は無視
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tt.remote
		if tt.xff != "" {
			req.Header.Set("X-Forwarded-For", tt.xff)
		}
		if got := clientIP(req); got != tt.want {
			t.Errorf("clientIP(%q, %q) = %q, want %q", tt.remote, tt.xff, got, tt.want)
		}
	}
}
