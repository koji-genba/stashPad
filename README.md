# stashPad

自宅ファイルサーバ上のメディアデータ(音声・動画・画像/マンガ)を、PC・スマホのブラウザから検索・閲覧・再生できるセルフホスト型メディアライブラリ。

## コンセプト

- **コピー不要**: ファイルサーバに置いたまま、ブラウザからストリーミング視聴
- **タグで探せる**: DLsite の作品情報 CSV をインポートしてジャンル・声優等で検索。独自タグも追加可能
- **フォルダ構造そのまま**: 作品フォルダを整理し直す必要なし。ライブラリ内をエクスプローラ的にブラウズしてメディアを開く
- **メディアには書き込まない**: 作品フォルダは読み取り専用で扱う

## ステータス

Phase 1 (MVP) 実装済み。スキャン・CSV インポート・検索/ファセット・ファイルブラウズ・Range 配信・オーディオプレイヤー(連続再生 / ±10 秒 / 速度 / Media Session)・画像ビューア・履歴・Docker まで動作する。Phase 2 の一部も実装済み: フルスクリーンプレイヤー・再生キュー操作(⋮ メニュー 3 アクション / キュー画面 / ドラッグ並び替え / Android 戻る対応)。

## ドキュメント

| ファイル | 内容 |
|---------|------|
| [docs/design.md](docs/design.md) | 設計の正本(要件・アーキテクチャ・データモデル・API・フェーズ計画・決定事項ログ) |
| [docs/implementation-notes.md](docs/implementation-notes.md) | 実装の具体的指針(環境変数・DDL・API 例・実装上の注意・完了条件) |
| [CLAUDE.md](CLAUDE.md) | 開発セッション向けのプロジェクト概要と進め方 |

## 技術スタック

- Backend: Go / chi / SQLite (modernc.org/sqlite, pure Go)
- Frontend: React / TypeScript / Vite(本番はビルド成果物を Go バイナリに `go:embed`)
- Deploy: 単一コンテナ(Docker Compose / k8s)

## 環境変数

| 変数 | 必須 | 説明 |
|------|------|------|
| `STASHPAD_LIBRARY_ROOTS` | ✔ | ライブラリルート。カンマ区切りで複数指定可(例 `/media/voice,/media/comic`) |
| `STASHPAD_DATA_DIR` | ✔ | SQLite DB とサムネイルキャッシュの置き場所(例 `/data`) |
| `STASHPAD_ADDR` | | リッスンアドレス(デフォルト `:8080`) |
| `STASHPAD_SCAN_ON_START` | | `true`/`1` で起動時にライブラリスキャンを自動実行(バックグラウンド) |

## 開発

backend と Vite dev server を並走させる。`/api` は Vite が :8080 へ proxy する。

```bash
# backend(:8080)
cd backend
STASHPAD_LIBRARY_ROOTS=/path/to/media STASHPAD_DATA_DIR=./data go run ./cmd/stashpad

# frontend(:5173)
cd frontend
npm install
npm run dev
```

テスト:

```bash
cd backend && go test ./...
cd frontend && npm run typecheck
```

## 本番ビルド(単一バイナリ)

frontend の成果物を `backend/internal/web/dist/` にコピーしてから go build すると、静的ファイルがバイナリに embed される。

```bash
cd frontend && npm run build
cp -r frontend/dist/. backend/internal/web/dist/
cd backend && CGO_ENABLED=0 go build -o stashpad ./cmd/stashpad
```

※ `backend/internal/web/dist/` は `.gitkeep` のみコミットされており、未コピーでもビルド・テストは通る(その場合 UI は 503 を返す)。

※ PWA 対応として `frontend/public/manifest.webmanifest` と `frontend/public/icons/` を配信している(`npm run build` で dist にコピーされる)。`.webmanifest` は Go の `mime.TypeByExtension` に未登録のため、`backend/internal/web/web.go` で `application/manifest+json` を明示的に付与している(distroless には `/etc/mime.types` も無いため)。

## Docker

```bash
cd deploy
# docker-compose.yml の volumes(メディアのマウント元)を環境に合わせて編集してから
docker compose up -d
```

Dockerfile は multi-stage(node → golang → distroless)で、`docker compose up` だけでフロントエンドのビルドと embed まで完結する。メディアは必ず **read-only**(`:ro`)でマウントすること。

実行イメージは `gcr.io/distroless/static:nonroot`(uid/gid 65532)で**非 root 実行**になっている。`./data`(SQLite + サムネイルキャッシュ)はこの uid が書き込める権限にしておくこと(例: `chown 65532:65532 data`。chown できない環境では `chmod 777 data` でも動くが権限は緩くなる点に注意)。また、コンテナには `HEALTHCHECK`(`/stashpad -healthcheck` が `GET /api/healthz` を叩く)が組み込まれており、`docker compose ps` 等で稼働状態を確認できる。

### k8s へのデプロイ

本番運用で k8s を使う場合は [deploy/k8s/](deploy/k8s/)(Deployment / Service / PVC / Ingress 例)を参照。`kubectl apply -k deploy/k8s/` でまとめて適用できる。プレースホルダ(イメージ名・NFS server/path・Ingress ホスト名等)は [deploy/k8s/README.md](deploy/k8s/README.md) を参照して書き換えること。

## 運用上の注意

stashPad は家庭内 LAN での利用を前提としています(認証なし)。外出先から使う場合は VPN(Tailscale 等)経由でアクセスし、直接インターネットへ公開しないでください。
