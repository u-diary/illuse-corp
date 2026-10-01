// Package api はサーバー B の HTTP ハンドラーを提供する。
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/card"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/ratelimit"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/store"
)

// EmployeeNumberHeader は発行した社員番号を返すレスポンスヘッダー。
const EmployeeNumberHeader = "X-Employee-Number"

// Config はハンドラーの設定。
type Config struct {
	Store          *store.Store
	Limiter        *ratelimit.Limiter
	AllowedOrigins []string
	// MaxConcurrent は写真のデコードと画像合成を同時に行う上限（メモリ使用量の抑制）。
	MaxConcurrent int
	Now           func() time.Time
	Logger        *slog.Logger
}

type handler struct {
	Config
	sem chan struct{}
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
	h := &handler{Config: cfg, sem: make(chan struct{}, cfg.MaxConcurrent)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /api/cards", h.createCard)
	mux.HandleFunc("OPTIONS /api/cards", h.preflight)
	return h.cors(mux)
}

func (h *handler) originAllowed(origin string) bool {
	return origin != "" && slices.Contains(h.AllowedOrigins, origin)
}

// cors は許可した Origin にだけ CORS ヘッダーを付ける。
func (h *handler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); h.originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", EmployeeNumberHeader)
		}
		next.ServeHTTP(w, r)
	})
}

func (h *handler) preflight(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r.Header.Get("Origin")) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) createCard(w http.ResponseWriter, r *http.Request) {
	// ブラウザ以外からの雑な呼び出しで社員番号を消費されないよう、Origin を必須にする。
	// （Origin は偽装できるので認証ではない。あくまで抑止）
	if !h.originAllowed(r.Header.Get("Origin")) {
		writeError(w, http.StatusForbidden, "このサイトからの送信は受け付けていません。")
		return
	}
	ip := clientIP(r)
	if !h.Limiter.Allow(ip) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "送信が集中しています。しばらく待ってから再度お試しください。")
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
		img := card.Render(card.Data{
			EmployeeNumber: store.FormatNumber(number),
			Name:           in.Name,
			Birthdate:      in.Birthdate,
			Department:     in.Department,
			Remarks:        in.Remarks,
			Photo:          photo,
		})
		var err error
		png, err = card.EncodePNG(img)
		return err
	})
	if err != nil {
		h.Logger.Error("社員証を発行できません", "ip", ip, "err", err)
		writeError(w, http.StatusInternalServerError, "社員証の作成に失敗しました。時間をおいて再度お試しください。")
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
		writeJSON(w, http.StatusBadRequest, ve)
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "送信データが大きすぎます。顔写真のサイズを小さくしてください。")
	default:
		h.Logger.Warn("フォームを読み取れません", "ip", ip, "err", err)
		writeError(w, http.StatusBadRequest, "送信内容を読み取れませんでした。")
	}
}

// clientIP は利用者の IP アドレスを返す。
// サーバー B は 127.0.0.1 で待ち受け、Tailscale Funnel が付ける X-Forwarded-For の
// 末尾（プロキシ自身が追記した値）だけを信用する。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return host
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
