// Package onboard はサーバー B（onboard）の HTTP ハンドラーを提供する。
package onboard

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/card"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/cardmeta"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/httpx"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/pngmeta"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/ratelimit"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/store"
)

// EmployeeNumberHeader は発行した社員番号を返すレスポンスヘッダー。
const EmployeeNumberHeader = "X-Employee-Number"

// Config はハンドラーの設定。
type Config struct {
	Store          *store.Store
	Signer         *cardmeta.Signer // 社員証のメタデータに署名する
	Limiter        *ratelimit.Limiter
	AllowedOrigins []string
	// MaxConcurrent は写真のデコードと画像合成を同時に行う上限（メモリ使用量の抑制）。
	MaxConcurrent int
	Now           func() time.Time
	Logger        *slog.Logger
}

type handler struct {
	Config
	cors httpx.CORS
	sem  chan struct{}
}

// NewHandler はサーバー B のルーティングを組み立てる。
func NewHandler(cfg Config) http.Handler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 2
	}
	h := &handler{
		Config: cfg,
		cors:   httpx.CORS{AllowedOrigins: cfg.AllowedOrigins, ExposeHeaders: []string{EmployeeNumberHeader}},
		sem:    make(chan struct{}, cfg.MaxConcurrent),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Healthz)
	mux.HandleFunc("POST /api/cards", h.createCard)
	mux.HandleFunc("OPTIONS /api/cards", h.cors.Preflight)
	return h.cors.Wrap(mux)
}

func (h *handler) createCard(w http.ResponseWriter, r *http.Request) {
	// ブラウザ以外からの雑な呼び出しで社員番号を消費されないよう、Origin とレート制限で絞る。
	ip, ok := h.cors.Admit(w, r, h.Limiter)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
	in, err := parseInput(r, h.Now())
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		h.writeInputError(w, ip, err)
		return
	}
	defer in.Close()

	// 受信と検証が終わってから枠を取る（回線の遅い利用者が枠を占有しないように）。
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	case <-r.Context().Done():
		return
	}

	photo, err := in.DecodePhoto()
	if err != nil {
		h.writeInputError(w, ip, err)
		return
	}

	generatedAt := h.Now().In(JST)
	var png []byte
	number, err := h.Store.Issue(r.Context(), in.Department, generatedAt, func(number int64) error {
		num := store.FormatNumber(number)
		img := card.Render(card.Data{
			EmployeeNumber: num,
			Name:           in.Name,
			Birthdate:      in.Birthdate,
			Department:     in.Department,
			Remarks:        in.Remarks,
			Photo:          photo,
		})
		var err error
		// 社員番号・生年月日・署名をメタデータとして埋め込む（fire で照合に使う）。
		png, err = pngmeta.Encode(img, h.Signer.Chunks(cardmeta.Identity{
			EmployeeNumber: num,
			Birthdate:      in.Birthdate.Format(time.DateOnly),
		})...)
		return err
	})
	if err != nil {
		h.Logger.Error("社員証を発行できません", "ip", ip, "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "社員証の作成に失敗しました。時間をおいて再度お試しください。")
		return
	}

	num := store.FormatNumber(number)
	// 個人情報（氏名・生年月日・写真・備考）はログに残さない。
	h.Logger.Info("社員証を発行しました", "number", num, "department", in.Department, "ip", ip)

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `inline; filename="employee-card-`+num+`.png"`)
	w.Header().Set(EmployeeNumberHeader, num)
	w.Write(png)
}

func (h *handler) writeInputError(w http.ResponseWriter, ip string, err error) {
	var ve *ValidationError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &ve):
		httpx.WriteJSON(w, http.StatusBadRequest, ve)
	case errors.As(err, &tooLarge):
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "送信データが大きすぎます。顔写真のサイズを小さくしてください。")
	default:
		h.Logger.Warn("フォームを読み取れません", "ip", ip, "err", err)
		httpx.WriteError(w, http.StatusBadRequest, "送信内容を読み取れませんでした。")
	}
}
