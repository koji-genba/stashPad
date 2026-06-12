# CLAUDE.md

stashPad — 自宅ファイルサーバ上のメディア(音声・動画・画像/マンガ)をブラウザから検索・視聴するセルフホスト型メディアライブラリ。

## 現在のステータス

**Phase 1 (MVP) 実装済み。** backend(スキャナ・CSV インポータ・検索/ファセット・ブラウズ・Range 配信・履歴)、frontend(一覧・詳細・プレイヤー・ビューア)、Docker まで揃っている。

**Phase 2 の一部も実装済み。** フルスクリーンプレイヤー(±5/±30 秒)、再生キュー操作(⋮ メニュー 3 アクション・キュー画面・ドラッグ並び替え・Android 戻る対応、issue #12/#14)、検索強化(キーワード/タグの NOT 検索・サークルファセット・詳細→一覧の検索状態維持、issue #9/#11/#15)まで完了。残りは k8s マニフェスト整備・カスタムタグ UI の洗練・お気に入り/ソート強化・画像ビューア強化等(design.md §11 Phase 2 参照)。

## ドキュメントマップ(実装前に必ず読むこと)

| ファイル | 内容 |
|---------|------|
| [docs/design.md](docs/design.md) | 設計の正本。要件・アーキテクチャ・データモデル・API 設計・開発フェーズ・決定事項ログ |
| [docs/implementation-notes.md](docs/implementation-notes.md) | 実装の具体的指針。環境変数・DDL・API の JSON 例・各機能の実装上の注意・Phase 1 完了条件 |
| [docs/samples/works.csv](docs/samples/works.csv) | DLsite 作品情報 CSV の実サンプル(インポータのテストフィクスチャに使う) |

## 確定済みの重要事項(詳細は design.md §12 決定事項ログ)

- バックエンド: **Go 1.25+**(chi + modernc.org/sqlite v1.52 の要求)。Python ではない
- フロントエンド: React + TypeScript + Vite。本番はビルド成果物を Go バイナリに `go:embed` し単一コンテナで配信
- DB: SQLite。メディアファイルの実体は **read-only マウント**され、絶対に書き込まない
- トランスコードなし(flac/wav/mp3/mp4/webp/png/jpg は全てブラウザネイティブ再生)
- 認証は当面なし。ただし全 API をミドルウェアチェーン経由にし後付け可能にしておく
- 作品フォルダは `RJxxxxx_作品名` 形式。CSV との突合は **RJ 番号のみ**で行う(タイトルは禁止文字置換で一致しない)
- スキャンはトップディレクトリの対応付けとサムネイルだけ。**フォルダ内部のファイルを DB に取り込まない**(ブラウズ時にリアルタイム読み)

## 開発の進め方

- 実装順序は design.md §11 の Phase 1 チェックリストに従う。推奨順は implementation-notes.md 参照
- ユーザーへの応答・ドキュメント・PR は日本語で書く
- スキャナ・CSV インポータ・パス検証(セキュリティ境界)にはユニットテストを書く
- `path` パラメータを受けるエンドポイントは必ずパストラバーサル検証を通す(implementation-notes.md §6)

## コマンド

```bash
# backend
cd backend && go run ./cmd/stashpad     # 起動(要 STASHPAD_* 環境変数。README 参照)
cd backend && go test ./...             # テスト

# frontend
cd frontend && npm run dev              # Vite dev server(/api を :8080 へ proxy)
cd frontend && npm run build            # 本番ビルド
cd frontend && npm run typecheck        # 型チェック(npx tsc --noEmit は使わないこと)

# 本番ビルド(embed): frontend/dist を backend/internal/web/dist へコピーして go build(README 参照)
```
