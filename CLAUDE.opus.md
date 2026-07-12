# CLAUDE.md

stashPad — 自宅ファイルサーバ上のメディア(音声・動画・画像/マンガ)をブラウザから検索・視聴するセルフホスト型メディアライブラリ。

## 現在のステータス

**Phase 1 (MVP) と Phase 2 の主要機能、マイルストーン 1〜3(信頼性・体験・基盤)は実装済み。** 今後の着手順と open issue の対応は [docs/roadmap.md](docs/roadmap.md) を参照。

## ドキュメントマップ(着手前に必ず該当箇所を読むこと)

| ファイル | 内容 |
|---------|------|
| [docs/design.md](docs/design.md) | 設計の正本。要件・アーキテクチャ・データモデル・API 設計・**§12 決定事項ログ(D1〜)** |
| [docs/roadmap.md](docs/roadmap.md) | 開発ロードマップ。open issue と着手順の対応 |
| [docs/implementation-notes.md](docs/implementation-notes.md) | 実装の具体的指針。環境変数・DDL・API の JSON 例・**§6 パス検証**・各機能の実装上の注意 |
| [docs/samples/works.csv](docs/samples/works.csv) | DLsite 作品情報 CSV の実サンプル(インポータのテストフィクスチャ) |

全文を読む必要はない。**担当タスクに関係する節だけを特定して読む**(例: API を触るなら design.md §7 と implementation-notes.md §4)。ただし設計判断を伴う変更では §12 決定事項ログを必ず確認し、過去の決定と矛盾しないこと。

## 確定済みの重要事項(詳細は design.md §12)

- バックエンド: **Go 1.25+**(chi + modernc.org/sqlite)。Python ではない
- フロントエンド: React + TypeScript + Vite。本番はビルド成果物を Go バイナリに `go:embed` し単一コンテナで配信
- DB: SQLite。メディアファイルの実体は **read-only マウント**され、絶対に書き込まない
- トランスコードなし(ブラウザネイティブ再生できる形式のみ)
- 認証は当面なし。ただし全 API をミドルウェアチェーン経由にし後付け可能にしておく
- 作品フォルダは `RJxxxxx_作品名` 形式。CSV との突合は **RJ 番号のみ**で行う
- スキャンはトップディレクトリの対応付けとサムネイルだけ。**フォルダ内部のファイルを DB に取り込まない**(ブラウズ時にリアルタイム読み)

## 開発プロセス(この順で進める)

### 1. 着手前

- issue 本文と docs/roadmap.md で背景・優先度・関連 issue を確認する
- 変更対象を grep で洗い出し、影響範囲を把握してから書き始める
- 「やらないこと」を先に決める。issue に書かれていない改善を勝手に混ぜない(気づいたことは完了報告で提案する)
- **着手前に既存テスト一式が通ることを確認する**。変更後に失敗が出たとき、自分の変更が原因かを正しく切り分けるため
- **外部ライブラリの仕様・破壊的変更は記憶で断定しない**。公式ドキュメント・リリースノート・移行ガイドを WebFetch で取得して確認する。issue が「〜に沿って」と手順を指定していたら、その手順を実際に踏む

### 2. テストファースト(t-wada 流 TDD)

- 実装前に**テストリスト**(このタスクで検証すべき振る舞いの箇条書き)を作る
- **Red → Green → Refactor**: 失敗するテストを 1 つ書き、通る最小の実装をし、テストが通る状態を保ったまま整理する
- 既存の挙動を変える・バグを直すときは、**先に現状の挙動を固定する(または再現する)テスト**を書いてから変更する
- テストは仕様の記述。テスト名(日本語)を読めば何を保証しているか分かるように書く
- 例外: 依存更新や設定変更などテストを書く対象がない変更では、**既存テスト一式を回帰網として使う**。着手前に全テストが通ることを確認してから始める

### 3. 検証(全部通るまで「完了」と言わない)

```bash
cd backend && go test ./...          # backend を触ったら
cd frontend && npm run test          # frontend を触ったら(vitest)
cd frontend && npm run typecheck     # 〃(npx tsc --noEmit は使わないこと)
cd frontend && npm run lint          # 〃
cd frontend && npm run build         # 〃(tsc -b + vite build)
```

- 検証コマンドは**実際に実行し、出力を確認する**。推測で「通るはず」と報告しない
- 失敗したら直してから再実行。全て通った状態でのみコミットする
- 実行時の挙動に影響し得る変更(状態管理・ルーティング・ビルド系・依存メジャー更新)では、テスト緑に加えて `npm run dev` を起動し Playwright(Chromium は導入済み)で主要画面を開き、**コンソールエラー・警告が出ないこと**を確認する。テストは全経路を再現しない

### 依存ライブラリの更新(issue #103 系のタスク)

- **1 メジャー = 1 コミット**で分割し、各ステップで検証マトリクスを全通しする
- 公式リリースノート・移行ガイドを取得し、**破壊的変更を一項目ずつ既存コードと突合**する。突合結果(該当あり/なしと根拠)はコミットメッセージに記録する
- 「コード変更ゼロ」で終わる場合こそ根拠を厚く書く。変更が無いことの正しさは変更があることより証明しにくい

### 4. 自己レビュー(コミット前に必ず)

- `git diff` を通読し、以下を確認する:
  - 意図しない変更・デバッグ残骸・不要な整形差分が混ざっていないか
  - 命名は既存コードの語彙と一致しているか
  - コメントは「なぜ」を説明しているか(「何をするか」の逐語コメントは書かない)
  - 周辺コードのイディオム・コメント密度・エラーハンドリング様式に合わせているか

### 5. ドキュメント更新

- roadmap.md に対応チェックボックスがあれば完了に更新する
- 設計判断(トレードオフの受容・方針の選択)をしたら design.md §12 決定事項ログに追記する
- ユーザーへの応答・ドキュメント・PR・コミットメッセージは**日本語**で書く

### 6. コミット

- 書式: `prefix(scope): 日本語の要約 (#issue番号)`。prefix は feat / fix / test / docs / refactor / chore、scope は backend / frontend / api など(省略可)
- **1 論理変更 = 1 コミット**。テスト追加と修正本体を分けられるなら分ける。無関係な変更を混ぜない

### 7. 完了報告

- 依頼者が報告の形式・項目を指定していたら**その形式に従う**。自己省察や一般論に流れない
- 指定が無ければ以下を含める: 変更内容の要約 / 実行した検証コマンドと結果(全通過の明示)/ 判断に迷った点・レビューしてほしい点 / スコープ外で気づいた問題(あれば)
- 事実(観測した出力)と推測(こうなるはず)を区別して書く

## コーディング規約(リーダブルコードの実践)

- **名前重要**: 変更の意図が名前に出るまで考える。既存コードで使われている語彙(work / track / overlay / queue など)を流用し、同じ概念に別の名前を発明しない
- **スコープは最小に**: 変数は使う場所の直前で宣言し、関数は 1 つの仕事だけをする
- **早期 return** でネストを浅く保つ(既存コードのスタイル)
- Go は標準的なイディオム(エラーは即 return、`t.Helper()`、テーブル駆動テスト)に従う

## テストの書き方(既存慣習に合わせる)

- **backend**: 標準 `testing` のみ。テーブル駆動テスト + `t.Run(日本語ケース名)`。DB は `:memory:` SQLite(`openTestDB(t)` ヘルパ)。CSV は docs/samples/works.csv をフィクスチャに使う。回帰テストには `// 回帰テスト: issue #N` コメント
- **frontend**: vitest + @testing-library/react。`it('日本語でユーザー視点の説明')`。store は `setState` で直接リセット、jsdom 非対応 API(HTMLMediaElement 等)は `vi.spyOn` でスタブ、要素特定は role / aria-label を優先

## リポジトリ固有の罠

- `path` パラメータを受けるエンドポイントは必ずパストラバーサル検証を通す(implementation-notes.md §6)。symlink 解決後に作品ルート配下かを確認
- API レスポンスの NULL 許容フィールドはキー省略ではなく**明示 `null`** で返す(フロント型定義 `string | null` との契約。design.md D 系ログ参照)
- Content-Type は `mime.TypeByExtension` に頼らず明示テーブル優先(distroless 環境差異のため)
- ファイル名の並びは自然順ソート(`page2 < page10`)。文字列比較で並べない
- CSV インポートはストリーミング処理(`ReadAll` 禁止)。行単位エラーは積んで継続
- 非表示作品(`hidden=1`)は検索・集計・履歴の全てから除外。upsert で hidden / manually_edited に触れない
- スキャナはライブラリルート直下のみ列挙(再帰しない)

## コマンド

```bash
# backend
cd backend && go run ./cmd/stashpad     # 起動(要 STASHPAD_* 環境変数。README 参照)
cd backend && go test ./...             # テスト

# frontend
cd frontend && npm run dev              # Vite dev server(/api を :8080 へ proxy)
cd frontend && npm run build            # 本番ビルド
cd frontend && npm run typecheck        # 型チェック
cd frontend && npm run test             # vitest
cd frontend && npm run lint             # ESLint

# 本番ビルド(embed): frontend/dist を backend/internal/web/dist へコピーして go build(README 参照)
```
