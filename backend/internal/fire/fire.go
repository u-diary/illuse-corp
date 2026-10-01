// Package fire はサーバー C（fire）の HTTP ハンドラーを提供する。
// onboard が発行した社員証の画像を受け取り、署名済みのメタデータで社員を特定して、
// 退職・解雇のスタンプを押した画像を返すとともに、処理内容を DB に記録する。
package fire

import (
	"bytes"
	"errors"
	"image"
	_ "image/png" // image.Decode で PNG を扱うため
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/cardmeta"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/httpx"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/pngmeta"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/ratelimit"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/stamp"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/store"
)

// Action は選択できる処理内容。
type Action struct {
	Code  string // フォームから送られる値
	Label string // ラジオボタンの文言。DB にはこれを記録する
	Title string // スタンプの 1 行目
	Note  string // スタンプの 3 行目
}

// Actions は選択肢の一覧。
var Actions = []Action{
	{Code: "resignation", Label: "退職届の受理", Title: "退職済", Note: "退職届を受理しました"},
	{Code: "absence", Label: "懲戒解雇(事由:長期間の無断欠勤)", Title: "懲戒解雇", Note: "事由：長期間の無断欠勤"},
	{Code: "crime", Label: "懲戒解雇(事由:犯罪行為の発覚)", Title: "懲戒解雇", Note: "事由：犯罪行為の発覚"},
}

func findAction(code string) (Action, bool) {
	for _, a := range Actions {
		if a.Code == code {
			return a, true
		}
	}
	return Action{}, false
}

// 入力の上限。
const (
	MaxUploadBytes = 12 << 20
	MaxImagePixels = 24_000_000
)

// ClientDateTimeLayout はクライアントから送られる日時（スタンプの 2 行目）の形式。
const ClientDateTimeLayout = "2006/01/02 15:04"

// EmployeeNumberHeader は処理した社員番号を返すレスポンスヘッダー。
const EmployeeNumberHeader = "X-Employee-Number"

// 利用者に見せるエラーメッセージ。
const (
	MsgInvalidImage     = "不正な画像です。"
	MsgAlreadyProcessed = "この社員は既に処理済みです。"
)

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

// Config はハンドラーの設定。
type Config struct {
	Store          *store.Store
	Signer         *cardmeta.Signer // 社員証のメタデータの署名を検証する
	Limiter        *ratelimit.Limiter
	AllowedOrigins []string
	// MaxConcurrent は画像のデコードとスタンプ合成を同時に行う上限（メモリ使用量の抑制）。
	MaxConcurrent int
	Now           func() time.Time
	Logger        *slog.Logger
}

type handler struct {
	Config
	cors httpx.CORS
	sem  chan struct{}
}

// NewHandler はサーバー C のルーティングを組み立てる。
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
	mux.HandleFunc("POST /api/stamps", h.createStamp)
	mux.HandleFunc("OPTIONS /api/stamps", h.cors.Preflight)
	return h.cors.Wrap(mux)
}

func (h *handler) createStamp(w http.ResponseWriter, r *http.Request) {
	ip, ok := h.cors.Admit(w, r, h.Limiter)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "画像のサイズが大きすぎます。")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "送信内容を読み取れませんでした。")
		return
	}
	defer r.MultipartForm.RemoveAll()

	action, ok := findAction(r.FormValue("action"))
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "処理内容を選択してください。")
		return
	}
	clientTime, err := time.Parse(ClientDateTimeLayout, r.FormValue("client_datetime"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "日時の形式が正しくありません。")
		return
	}
	data, err := readFile(r, "image")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "画像を添付してください。")
		return
	}

	// メタデータ（社員番号・生年月日・署名）を検証し、DB で社員を特定する。
	id, number, err := h.identify(r, data)
	switch {
	case errors.Is(err, store.ErrAlreadyProcessed):
		httpx.WriteError(w, http.StatusConflict, MsgAlreadyProcessed)
		return
	case err != nil:
		h.Logger.Info("不正な画像を受け付けませんでした", "ip", ip, "reason", err)
		httpx.WriteError(w, http.StatusBadRequest, MsgInvalidImage)
		return
	}

	// 受信と検証が終わってから枠を取る（回線の遅い利用者が枠を占有しないように）。
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	case <-r.Context().Done():
		return
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		h.Logger.Info("画像をデコードできません", "ip", ip, "err", err)
		httpx.WriteError(w, http.StatusBadRequest, MsgInvalidImage)
		return
	}
	stamped := stamp.Apply(src, stamp.Content{
		Title:    action.Title,
		DateTime: clientTime.Format(ClientDateTimeLayout),
		Note:     action.Note,
	})
	// 元のメタデータを引き継ぐ（処理済みの画像を再び送られたときに「処理済み」と判定できる）。
	out, err := pngmeta.Encode(stamped, h.Signer.Chunks(id)...)
	if err != nil {
		h.Logger.Error("画像をエンコードできません", "ip", ip, "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "処理に失敗しました。時間をおいて再度お試しください。")
		return
	}

	// 画像ができてから記録する。記録できたときだけ画像を返す。
	switch err := h.Store.RecordAction(r.Context(), number, action.Label, h.Now().In(jst)); {
	case errors.Is(err, store.ErrAlreadyProcessed):
		httpx.WriteError(w, http.StatusConflict, MsgAlreadyProcessed)
		return
	case errors.Is(err, store.ErrNotFound):
		httpx.WriteError(w, http.StatusBadRequest, MsgInvalidImage)
		return
	case err != nil:
		h.Logger.Error("処理を記録できません", "ip", ip, "number", id.EmployeeNumber, "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "処理に失敗しました。時間をおいて再度お試しください。")
		return
	}

	h.Logger.Info("処理を記録しました", "number", id.EmployeeNumber, "action", action.Label, "ip", ip)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set(EmployeeNumberHeader, id.EmployeeNumber)
	w.Write(out)
}

// identify は画像のメタデータを検証して社員を特定する。
// 処理済みなら store.ErrAlreadyProcessed、それ以外の問題はすべて「不正な画像」として扱うエラーを返す。
func (h *handler) identify(r *http.Request, data []byte) (cardmeta.Identity, int64, error) {
	chunks, err := pngmeta.Read(data)
	if err != nil {
		return cardmeta.Identity{}, 0, err
	}
	id, err := h.Signer.Verify(chunks)
	if err != nil {
		return cardmeta.Identity{}, 0, err
	}
	number, err := strconv.ParseInt(id.EmployeeNumber, 10, 64)
	if err != nil {
		return cardmeta.Identity{}, 0, err
	}
	rec, err := h.Store.Get(r.Context(), number)
	if err != nil {
		return cardmeta.Identity{}, 0, err
	}
	if rec.Action != "" {
		return cardmeta.Identity{}, 0, store.ErrAlreadyProcessed
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return cardmeta.Identity{}, 0, err
	}
	if cfg.Width*cfg.Height > MaxImagePixels {
		return cardmeta.Identity{}, 0, errors.New("画像の解像度が大きすぎます")
	}
	return id, number, nil
}

func readFile(r *http.Request, field string) ([]byte, error) {
	f, _, err := r.FormFile(field)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
