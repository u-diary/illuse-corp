# illuse-corp

イリューズ社の社内向けアプリケーション 2 つを収めたリポジトリです。

| アプリ      | URL                                   | 内容                                                               |
| ----------- | ------------------------------------- | ------------------------------------------------------------------ |
| **onboard** | https://illuse-corp.u-diary.art/      | 入社手続きフォーム。入力内容から社員証の画像を発行する             |
| **fire**    | https://illuse-corp.u-diary.art/fire/ | 社員証の画像に、退職・懲戒解雇の赤いスタンプを押して処理を記録する |

```
[ブラウザ] ─> GitHub Pages（frontend/, React + Vite）
     │
     ├─ onboard: POST /api/cards ──> Funnel :8443  ──> サーバー B 127.0.0.1:8081（onboard-server）
     └─ fire:    POST /api/stamps ─> Funnel :10000 ──> サーバー C 127.0.0.1:8083（fire-server）
                                                          │
                     サーバー B・C は同じ SQLite（backend/data/cards.db）と
                     署名用の秘密鍵（/etc/illuse/signing-key）を共有する
```

## 構成

| パス                         | 内容                                                              |
| ---------------------------- | ----------------------------------------------------------------- |
| `frontend/`                  | 2 ページ構成（`index.html` = onboard、`fire/index.html` = fire）  |
| `backend/cmd/onboard-server` | サーバー B。社員番号を採番し、社員証の PNG を返す                 |
| `backend/cmd/fire-server`    | サーバー C。社員証の PNG にスタンプを押して返し、処理を記録する   |
| `backend/cmd/genbg`          | 社員証の背景画像を生成するツール（`go generate ./internal/card`） |
| `backend/internal/cardmeta`  | 社員証のメタデータ（社員番号・生年月日）の署名と検証              |
| `backend/internal/pngmeta`   | PNG の tEXt チャンクの読み書き                                    |
| `backend/internal/store`     | SQLite（採番・発行記録・処理記録）                                |
| `backend/internal/stamp`     | 判子風の赤いスタンプの描画                                        |
| `backend/internal/design`    | 社員証のレイアウト・フォント（Noto Sans JP, SIL OFL 1.1）         |
| `backend/deploy/`            | systemd のユニット（`illuse-corp-onboard` / `illuse-corp-fire`）  |

## 社員証のメタデータと署名

onboard が発行する社員証の PNG には、tEXt チャンクとして次の 3 つを埋め込みます。

| キー             | 例                 |
| ---------------- | ------------------ |
| `EmployeeNumber` | `000001`           |
| `Birthdate`      | `2001-03-15`       |
| `Signature`      | `v1.<HMAC-SHA256>` |

署名は社員番号と生年月日の組を秘密鍵で HMAC-SHA256 したものです。fire は署名を検証してから
DB を社員番号で引くため、メタデータを書き換えた画像や、自分で書き込んだ画像は「不正な画像」として拒否されます。
生年月日の一致は署名で保証されるので、DB に生年月日は保存しません。

## API

### onboard: `POST /api/cards`

| フィールド   | 必須 | 内容                                            |
| ------------ | ---- | ----------------------------------------------- |
| `name`       | ○    | 氏名（40 文字以内）                             |
| `birthdate`  | ○    | 生年月日 `YYYY-MM-DD`（1900-01-01 〜 今日）     |
| `photo`      | ○    | 顔写真（JPEG / PNG、中央を 3:4 で切り抜き）     |
| `department` | ○    | `sales` / `engineering` / `operations` / `none` |
| `remarks`    |      | 備考（200 文字以内）                            |
| `agreement`  | ○    | `true`                                          |

- 成功時: `200 image/png`（署名済みメタデータ入り）、社員番号は `X-Employee-Number` ヘッダー
- 入力エラー: `400 {"field": "...", "error": "..."}`、送信過多: `429`、サイズ超過: `413`
- `department=none` の場合、社員証と DB には「業務統括部クレーム対応室」と記録します

### fire: `POST /api/stamps`

| フィールド        | 必須 | 内容                                               |
| ----------------- | ---- | -------------------------------------------------- |
| `action`          | ○    | `resignation` / `absence` / `crime`                |
| `client_datetime` | ○    | 端末の日時 `YYYY/MM/DD HH:mm`（スタンプの 2 行目） |
| `image`           | ○    | onboard が発行した社員証の PNG（加工せずそのまま） |

| `action`      | DB に記録する処理内容           | スタンプ 1 行目 | スタンプ 3 行目        |
| ------------- | ------------------------------- | --------------- | ---------------------- |
| `resignation` | 退職届の受理                    | 退職済          | 退職届を受理しました   |
| `absence`     | 懲戒解雇(事由:長期間の無断欠勤) | 懲戒解雇        | 事由：長期間の無断欠勤 |
| `crime`       | 懲戒解雇(事由:犯罪行為の発覚)   | 懲戒解雇        | 事由：犯罪行為の発覚   |

- 成功時: `200 image/png`（元のメタデータを引き継ぐ）
- PNG でない／メタデータがない／署名が一致しない／DB にいない: `400 {"error": "不正な画像です。"}`
- すでに処理済み: `409 {"error": "この社員は既に処理済みです。"}`（1 人につき 1 回だけ記録）

## データベース

`backend/data/cards.db`（SQLite）。氏名・生年月日・顔写真・備考は保存しません。

```sql
CREATE TABLE employees (
  id           INTEGER PRIMARY KEY AUTOINCREMENT, -- 社員番号（000001 から連番）
  department   TEXT    NOT NULL,                  -- 社員証に印字した部署名
  generated_at TEXT    NOT NULL,                  -- 画像生成時刻（RFC 3339, JST）
  action       TEXT,                              -- fire の処理内容（未処理なら NULL）
  action_at    TEXT                               -- fire の処理時刻（RFC 3339, JST、サーバーの時刻）
);
```

- 社員証の生成に失敗した場合は記録をロールバックするため、社員番号に欠番は生じません
- 列の追加は `PRAGMA user_version` で管理し、起動時に自動で適用します

## 開発

```sh
# 署名用の秘密鍵（開発用）
openssl rand -hex 32 > /tmp/dev-signing-key

# サーバー B・C（開発用に localhost:5173 からのアクセスを許可）
cd backend
export ILLUSE_SIGNING_KEY_FILE=/tmp/dev-signing-key ILLUSE_ALLOWED_ORIGINS=http://localhost:5173
go run ./cmd/onboard-server &   # 127.0.0.1:8081
go run ./cmd/fire-server &      # 127.0.0.1:8083

# フロントエンド（.env.development により localhost:8081 / 8083 へ送信）
cd frontend
npm install
npm run dev   # http://localhost:5173/ と http://localhost:5173/fire/
```

テスト: `cd backend && go test ./...`

## 本番環境

### フロントエンド

`main` へ push すると GitHub Actions（`.github/workflows/pages.yml`）が GitHub Pages へデプロイします。
送信先は `frontend/.env.production` の `VITE_ONBOARD_API_BASE_URL` / `VITE_FIRE_API_BASE_URL` です。

### サーバー B・C

```sh
# 署名用の秘密鍵（初回のみ。作り直すと発行済みの社員証が fire で使えなくなる）
sudo install -d -m 755 /etc/illuse
openssl rand -hex 32 | sudo tee /etc/illuse/signing-key > /dev/null
sudo chown root:prometheus17th /etc/illuse/signing-key
sudo chmod 640 /etc/illuse/signing-key

cd backend
go build -o bin/onboard-server ./cmd/onboard-server
go build -o bin/fire-server ./cmd/fire-server
mkdir -p data
sudo cp deploy/illuse-corp-onboard.service deploy/illuse-corp-fire.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now illuse-corp-onboard illuse-corp-fire

# 公開（443 番の既存の設定には触れない）
sudo tailscale funnel --bg --https=8443 http://127.0.0.1:8081    # サーバー B
sudo tailscale funnel --bg --https=10000 http://127.0.0.1:8083   # サーバー C
```

設定（環境変数）:

| 変数                      | 既定値                                            |
| ------------------------- | ------------------------------------------------- |
| `ILLUSE_ADDR`             | B: `127.0.0.1:8081` / C: `127.0.0.1:8083`         |
| `ILLUSE_DB`               | `data/cards.db`                                   |
| `ILLUSE_SIGNING_KEY_FILE` | （必須）署名用の秘密鍵のファイル                  |
| `ILLUSE_ALLOWED_ORIGINS`  | `https://illuse-corp.u-diary.art`（カンマ区切り） |
| `ILLUSE_RATE_LIMIT`       | B: `5` / C: `10`（IP ごとの 1 分あたりの上限）    |
