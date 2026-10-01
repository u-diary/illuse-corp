// server はサーバー B（社員証画像の生成サーバー）を起動する。
//
// 設定は環境変数で行う。
//
//	ILLUSE_ADDR             待ち受けアドレス（既定: 127.0.0.1:8081）
//	ILLUSE_DB               SQLite のパス（既定: data/cards.db）
//	ILLUSE_ALLOWED_ORIGINS  CORS で許可する Origin（カンマ区切り、既定: https://illuse-corp.u-diary.art）
//	ILLUSE_RATE_LIMIT       IP ごとの 1 分あたりの発行上限（既定: 5）
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/api"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/ratelimit"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("サーバーを起動できません", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := env("ILLUSE_ADDR", "127.0.0.1:8081")
	dbPath := env("ILLUSE_DB", "data/cards.db")
	origins := strings.Split(env("ILLUSE_ALLOWED_ORIGINS", "https://illuse-corp.u-diary.art"), ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}
	rateLimit, err := strconv.Atoi(env("ILLUSE_RATE_LIMIT", "5"))
	if err != nil || rateLimit <= 0 {
		return errors.New("ILLUSE_RATE_LIMIT は正の整数で指定してください")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := &http.Server{
		Addr: addr,
		Handler: api.NewHandler(api.Config{
			Store:          st,
			Limiter:        ratelimit.New(rateLimit, time.Minute),
			AllowedOrigins: origins,
			Logger:         logger,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		// スマートフォンの遅い回線からのアップロードも考慮して長めに取る。
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("サーバー B を起動しました", "addr", addr, "db", dbPath, "origins", origins)
		errCh <- srv.ListenAndServe()
	}()

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

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
