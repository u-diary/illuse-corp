// fire-server はサーバー C（fire: 退職・解雇のスタンプを押すサーバー）を起動する。
//
// 設定は環境変数で行う。
//
//	ILLUSE_ADDR              待ち受けアドレス（既定: 127.0.0.1:8083）
//	ILLUSE_DB                SQLite のパス（既定: data/cards.db、サーバー B と共通）
//	ILLUSE_SIGNING_KEY_FILE  署名用の秘密鍵のファイル（必須、サーバー B と共通）
//	ILLUSE_ALLOWED_ORIGINS   CORS で許可する Origin（カンマ区切り、既定: https://illuse-corp.u-diary.art）
//	ILLUSE_RATE_LIMIT        IP ごとの 1 分あたりの上限（既定: 10）
package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/app"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/fire"
	"github.com/u-diary/illuse-corp-onboarding/backend/internal/ratelimit"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("サーバーを起動できません", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := app.Env("ILLUSE_ADDR", "127.0.0.1:8083")
	origins := app.Origins()
	rateLimit, err := app.RateLimit(10)
	if err != nil {
		return err
	}
	signer, err := app.LoadSigner()
	if err != nil {
		return err
	}
	st, dbPath, err := app.OpenStore()
	if err != nil {
		return err
	}
	defer st.Close()

	logger.Info("サーバー C（fire）を起動しました", "addr", addr, "db", dbPath, "origins", origins)
	return app.Serve(logger, addr, fire.NewHandler(fire.Config{
		Store:          st,
		Signer:         signer,
		Limiter:        ratelimit.New(rateLimit, time.Minute),
		AllowedOrigins: origins,
		Logger:         logger,
	}))
}
