// Package app はサーバー B・C に共通する起動処理（設定の読み込み・DB と鍵の準備・HTTP サーバーの実行）を提供する。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/u-diary/illuse-corp/backend/internal/cardmeta"
	"github.com/u-diary/illuse-corp/backend/internal/store"
)

// 共通の設定（環境変数）。
const (
	EnvDB             = "ILLUSE_DB"               // SQLite のパス（既定: data/cards.db）
	EnvSigningKeyFile = "ILLUSE_SIGNING_KEY_FILE" // 署名用の秘密鍵のファイル（必須）
	EnvAllowedOrigins = "ILLUSE_ALLOWED_ORIGINS"  // CORS で許可する Origin（カンマ区切り）
	EnvRateLimit      = "ILLUSE_RATE_LIMIT"       // IP ごとの 1 分あたりの上限

	DefaultOrigin = "https://illuse-corp.u-diary.art"
)

// Env は環境変数 key の値を返す。未設定なら fallback を返す。
func Env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Origins は CORS で許可する Origin の一覧を返す。
func Origins() []string {
	origins := strings.Split(Env(EnvAllowedOrigins, DefaultOrigin), ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}
	return origins
}

// RateLimit は IP ごとの 1 分あたりの上限を返す。
func RateLimit(fallback int) (int, error) {
	n, err := strconv.Atoi(Env(EnvRateLimit, strconv.Itoa(fallback)))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s は正の整数で指定してください", EnvRateLimit)
	}
	return n, nil
}

// OpenStore は DB を開く（ディレクトリがなければ作る）。
func OpenStore() (*store.Store, string, error) {
	path := Env(EnvDB, "data/cards.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, "", err
	}
	st, err := store.Open(path)
	return st, path, err
}

// LoadSigner は署名用の秘密鍵を読み込む。
func LoadSigner() (*cardmeta.Signer, error) {
	path := os.Getenv(EnvSigningKeyFile)
	if path == "" {
		return nil, errors.New(EnvSigningKeyFile + " に署名用の秘密鍵のファイルを指定してください（`openssl rand -hex 32` で作成できます）")
	}
	return cardmeta.LoadSigner(path)
}

// Serve は SIGINT / SIGTERM を受けるまで HTTP サーバーを動かし、受けたら処理中のリクエストを待って止める。
func Serve(logger *slog.Logger, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		// スマートフォンの遅い回線からのアップロードも考慮して長めに取る。
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	logger.Info("停止しています…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
