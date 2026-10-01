# illuse-corp-onboarding

イリューズ社の入社手続きフォームです。入力内容をもとに社員証の画像を発行します。

```
[ブラウザ] ─> GitHub Pages: https://illuse-corp.u-diary.art/   (frontend/, React + Vite)
     │  POST /api/cards (multipart/form-data)
     └──> Tailscale Funnel: https://prometheus.tail3cec56.ts.net:8443
              └─> サーバー B: 127.0.0.1:8081   (backend/, Go + SQLite)
```

## 構成

| パス                      | 内容                                                              |
| ------------------------- | ----------------------------------------------------------------- |
| `frontend/`               | 入力フォーム → ロード画面（jQuery アニメーション）→ 完了画面      |
| `backend/cmd/server`      | サーバー B。社員番号を採番し、社員証の PNG を返す                 |
| `backend/cmd/genbg`       | 社員証の背景画像を生成するツール（`go generate ./internal/card`） |
| `backend/internal/design` | 社員証のレイアウト・フォント（Noto Sans JP, SIL OFL 1.1）         |
| `backend/deploy/`         | systemd のユニット                                                |

## API

`POST /api/cards`（`Origin: https://illuse-corp.u-diary.art` のみ受け付け）

| フィールド   | 必須 | 内容                                            |
| ------------ | ---- | ----------------------------------------------- |
| `name`       | ○    | 氏名（40 文字以内）                             |
| `birthdate`  | ○    | 生年月日 `YYYY-MM-DD`（1900-01-01 〜 今日）     |
| `photo`      | ○    | 顔写真（JPEG / PNG、中央を 3:4 で切り抜き）     |
| `department` | ○    | `sales` / `engineering` / `operations` / `none` |
| `remarks`    |      | 備考（200 文字以内）                            |
| `agreement`  | ○    | `true`                                          |

- 成功時: `200 image/png`、社員番号は `X-Employee-Number` ヘッダー（例: `000001`）
- 入力エラー: `400 {"field": "...", "error": "..."}`、送信過多: `429`、サイズ超過: `413`

`department=none` の場合、社員証と DB には「業務統括部クレーム対応室」と記録します。

### データベース

`backend/data/cards.db`（SQLite）。保存するのは社員番号・部署・画像生成時刻のみで、
氏名・生年月日・顔写真・備考は保存しません。

```sql
CREATE TABLE employees (
  id           INTEGER PRIMARY KEY AUTOINCREMENT, -- 社員番号（000001 から連番）
  department   TEXT    NOT NULL,                  -- 社員証に印字した部署名
  generated_at TEXT    NOT NULL                   -- 画像生成時刻（RFC 3339, JST）
);
```

画像の生成に失敗した場合は記録をロールバックするため、社員番号に欠番は生じません。

## 開発

```sh
# サーバー B（開発用に localhost:5173 からのアクセスを許可）
cd backend
ILLUSE_ALLOWED_ORIGINS=http://localhost:5173 go run ./cmd/server

# フロントエンド（.env.development により localhost:8081 へ送信）
cd frontend
npm install
npm run dev
```

テスト: `cd backend && go test ./...`

## 本番環境

### フロントエンド

`main` へ push すると GitHub Actions（`.github/workflows/pages.yml`）が GitHub Pages へデプロイします。
送信先は `frontend/.env.production` の `VITE_API_BASE_URL` です。

### サーバー B

```sh
cd backend
go build -o bin/illuse-card-server ./cmd/server
mkdir -p data
sudo cp deploy/illuse-card.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now illuse-card

# 8443 番で公開（443 番の既存の設定には触れない）
tailscale funnel --bg --https=8443 http://127.0.0.1:8081
```

設定（環境変数）:

| 変数                     | 既定値                                            |
| ------------------------ | ------------------------------------------------- |
| `ILLUSE_ADDR`            | `127.0.0.1:8081`                                  |
| `ILLUSE_DB`              | `data/cards.db`                                   |
| `ILLUSE_ALLOWED_ORIGINS` | `https://illuse-corp.u-diary.art`（カンマ区切り） |
| `ILLUSE_RATE_LIMIT`      | `5`（IP ごとの 1 分あたりの発行上限）             |
