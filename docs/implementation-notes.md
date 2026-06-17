# stashPad 実装ノート

[design.md](design.md) を実装に落とすための具体的指針。設計と矛盾が生じた場合は design.md を正とし、本書を更新すること。

- 対象: Phase 1 (MVP)
- 最終更新: 2026-06-10

---

## 1. 開発環境

- Go 1.25 以上(modernc.org/sqlite v1.52 の要求。ルーターは chi を採用)
- Node.js 20 以上 + npm
- 主要 Go 依存(予定):
  - `github.com/go-chi/chi/v5` — ルーター
  - `modernc.org/sqlite` — pure Go SQLite ドライバ(cgo 不要)
  - `golang.org/x/image/webp` — webp デコード(サムネイル用)
- 開発時はバックエンド(:8080)と Vite dev server を並走させ、Vite の proxy で `/api` を :8080 へ転送する
- 本番ビルドは frontend の `dist/` を `go:embed` で取り込み、単一バイナリにする

## 2. 設定(環境変数)

| 変数 | 必須 | 例 | 説明 |
|------|------|----|------|
| `STASHPAD_LIBRARY_ROOTS` | ✔ | `/media/voice,/media/comic` | ライブラリルート。カンマ区切りで複数指定可 |
| `STASHPAD_DATA_DIR` | ✔ | `/data` | SQLite DB とサムネイルキャッシュの置き場所 |
| `STASHPAD_ADDR` | | `:8080`(デフォルト) | リッスンアドレス |

- DB ファイル: `{STASHPAD_DATA_DIR}/stashpad.db`
- サムネイル: `{STASHPAD_DATA_DIR}/thumbs/{work_id}.jpg`

## 3. DB スキーマ(DDL)

マイグレーションは「embed した SQL ファイルを番号順に適用し、適用済み番号を `schema_migrations` テーブルに記録する」自前の単純方式でよい(外部ツール不要)。

```sql
CREATE TABLE works (
    id             INTEGER PRIMARY KEY,
    rj_number      TEXT UNIQUE,            -- NULL 可(RJ番号なしフォルダ)
    title          TEXT NOT NULL,
    circle         TEXT,
    series_name    TEXT,
    purchase_date  TEXT,
    work_type      TEXT,
    age_rating     TEXT,
    file_format    TEXT,
    file_size_text TEXT,
    event          TEXT,
    root_path      TEXT,                   -- 作品フォルダの絶対パス。NULL = CSVのみでフォルダ未発見
    thumbnail_path TEXT,
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE tags (
    id       INTEGER PRIMARY KEY,
    name     TEXT NOT NULL,
    category TEXT NOT NULL,                -- genre / detail_genre / voice_actor / scenario / illustration / music / custom
    UNIQUE (name, category)
);

CREATE TABLE work_tags (
    work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    PRIMARY KEY (work_id, tag_id)
);

CREATE TABLE play_history (
    id        INTEGER PRIMARY KEY,
    work_id   INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,               -- 作品ルートからの相対パス
    played_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_work_tags_tag ON work_tags(tag_id);
CREATE INDEX idx_play_history_work ON play_history(work_id, played_at);
```

メモ:

- `works.root_path` は**絶対パス**(例 `/media/voice/RJ404669_耳舐め&...`)で保持する。マウントポイント変更等でパスが無効になっても、再スキャンで RJ 番号により再リンクされるので問題ない。スキャン時に存在しないパスを検出したら NULL に戻す
- SQLite は `PRAGMA foreign_keys = ON` を接続ごとに有効化すること
- 同時書き込みはほぼ無いが `PRAGMA journal_mode = WAL` を推奨

## 4. API の入出力例

エラーは一律 `{"error": "メッセージ"}` + 適切なステータスコード。

### GET /api/works?q=&tags=1,5&exclude_tags=7&sort=purchase_date&order=desc&page=1&limit=40

```json
{
  "items": [
    {
      "id": 12,
      "rj_number": "RJ404669",
      "title": "耳舐め&耳ふ～サンドイッチ ダウナー妹と低音姉【...】",
      "circle": "チームランドセル",
      "age_rating": "R-15",
      "has_folder": true,
      "thumbnail_url": "/api/works/12/thumbnail"
    }
  ],
  "total": 1532,
  "page": 1,
  "limit": 40
}
```

- `q` は空白(半角・全角・タブ)区切りで複数語の AND。各語はタイトル・サークル・RJ 番号への部分一致。`-語` で除外(`-` 単体は無視)
- `tags` は AND 条件(指定タグを全部持つ作品のみ)。`exclude_tags` は指定タグを 1 つでも持つ作品を除外(`NOT EXISTS`)
- `sort`: `purchase_date`(デフォルト) / `title` / `created_at` / `circle`
- `has_folder` = `root_path IS NOT NULL`。false の作品は一覧で「未取込」表示
- `hidden`: 未指定/`0` は可視作品のみ、`1` は非表示作品のみ(設定画面の非表示一覧用)。デフォルトで非表示作品は一覧に出ない

### GET /api/works/12

```json
{
  "id": 12,
  "rj_number": "RJ404669",
  "title": "...",
  "circle": "チームランドセル",
  "series_name": "ぐっすり眠れるASMR",
  "purchase_date": "2026/01/04 10:44",
  "work_type": "ボイス・ASMR",
  "age_rating": "R-15",
  "file_format": "WAV/ MP3同梱",
  "file_size_text": "4.91GB",
  "has_folder": true,
  "hidden": false,
  "tags": [
    {"id": 3, "name": "ボイス・ASMR", "category": "genre"},
    {"id": 17, "name": "耳舐め", "category": "detail_genre"},
    {"id": 41, "name": "耳恋なか", "category": "voice_actor"}
  ]
}
```

- `hidden` は常に bool で返る。`PATCH /api/works/12` に `{"hidden": true|false}` を送ると切り替わる(タイトル・サークル編集と同じエンドポイント)

### GET /api/works/12/entries?path=mp3

```json
{
  "path": "mp3",
  "parent": "",
  "entries": [
    {"name": "01_オープニング.mp3", "is_dir": false, "size": 12345678, "media_kind": "audio"},
    {"name": "おまけ", "is_dir": true, "size": 0, "media_kind": ""}
  ]
}
```

- `path=` 空文字列が作品ルート
- ディレクトリ→ファイルの順、それぞれ**自然順ソート**(§7)

### GET /api/tags?category=voice_actor&q=耳

```json
{
  "items": [
    {"id": 41, "name": "耳恋なか", "category": "voice_actor", "work_count": 3}
  ]
}
```

### GET /api/circles?q=ランドセル

```json
{
  "items": [
    {"name": "チームランドセル", "work_count": 5}
  ]
}
```

- `circle` が NULL・空文字の作品は集計から除外。`work_count` 降順 → サークル名昇順

### POST /api/works/12/tags

リクエスト `{"name": "睡眠用"}` → category は常に `custom` で作成(既存 custom タグがあれば再利用)。

### POST /api/works/12/plays

リクエスト `{"path": "mp3/01_オープニング.mp3"}` → 201。

### GET /api/history?page=1

```json
{
  "items": [
    {
      "work": {"id": 12, "title": "...", "thumbnail_url": "/api/works/12/thumbnail"},
      "last_played_at": "2026-06-10T22:15:03Z",
      "last_file_path": "mp3/01_オープニング.mp3",
      "play_count": 8
    }
  ],
  "page": 1
}
```

作品単位でグルーピングし、最終再生日時の降順。

### POST /api/import/csv

multipart で CSV を受け取り、結果サマリを返す:

```json
{"created": 12, "updated": 1480, "linked": 5, "errors": []}
```

### POST /api/scan

```json
{"works_found": 1532, "newly_registered": 3, "linked_to_csv": 2, "missing_marked": 1}
```

スキャンは数千フォルダの readdir 程度なので同期実行でよい(数秒で終わる)。タイムアウトが問題になったら非同期化を検討。

## 5. media_kind の拡張子マッピング

| media_kind | 拡張子 |
|------------|--------|
| audio | .flac .wav .mp3 |
| video | .mp4 |
| image | .jpg .jpeg .png .webp |
| text | .txt |
| other | 上記以外すべて |

大文字小文字は無視。判定はこの表に閉じる(MIME 推定などはしない)。配信時の Content-Type は拡張子から `mime.TypeByExtension` で決める。

## 6. パストラバーサル検証(セキュリティ境界・必ずテストを書く)

`path` パラメータを受ける全エンドポイント(`/entries`, `/file`)で:

1. `path` が空 or 相対パスであることを確認(先頭 `/` や Windows ドライブは拒否)
2. `filepath.Join(work.RootPath, path)` で結合し `filepath.EvalSymlinks` で実体解決
3. 解決後パスが `EvalSymlinks(work.RootPath)` 配下(prefix + パス区切り境界)にあることを確認
4. 違反は 403。対象が存在しなければ 404

テストケース例: `../../etc/passwd`、`..%2f` デコード後、作品外への symlink、正常系の日本語パス。

## 7. 自然順ソート

`page2.jpg < page10.jpg` となる比較。ファイル名を「数字列 / 非数字列」のトークンに分割し、数字列は数値として、非数字列は文字列として比較する。小さい関数なので自前実装し、テーブル駆動テストを書く(`01.mp3 < 2.mp3 < 10.mp3`、`トラック1 < トラック02 < トラック10` など)。

## 8. ファイル配信(Range)

```go
f, _ := os.Open(resolvedPath)
defer f.Close()
st, _ := f.Stat()
w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(resolvedPath)))
http.ServeContent(w, r, st.Name(), st.ModTime(), f)
```

`http.ServeContent` が Range / If-Range / 206 / HEAD を全部処理する。自前で Range をパースしないこと。

## 9. スキャナ

- 各ライブラリルートの**直下ディレクトリのみ**列挙(再帰しない)。ファイルは無視
- RJ 番号抽出: `^(RJ\d{6,8})` をフォルダ名に適用
  - マッチ → その RJ 番号で works を upsert(既存 CSV 行があれば `root_path` をリンク、無ければ新規作成しタイトルは `_` 以降の文字列)
  - 非マッチ → フォルダ名全体をタイトルとして新規作成(`rj_number` NULL)。既存判定は `root_path` 一致で行う
- 既存 works の `root_path` が指す先が消えていたら NULL に戻す(works 行自体は消さない)
- サムネイル: 作品ルートから深さ 2 までを探索し、`表紙|cover|jacket|サムネ|main`(大文字小文字無視)を名前に含む画像を優先、なければ自然順で最初の画像。長辺 512px に縮小し jpeg (quality 85) で `{data}/thumbs/{work_id}.jpg` へ保存。生成済みならスキップ

## 10. CSV インポータ

- ヘッダ行必須。列は docs/samples/works.csv の 17 列を想定するが、**列順はヘッダ名で解決**する(将来の列追加に耐える)
- 文字コードは UTF-8(BOM 付き許容。先頭の BOM を剥がす)
- `rj_number` で upsert。works のカラムは CSV 値で上書き(手動編集した title が CSV 再インポートで戻る件は許容: design.md の運用前提)
- タグ展開の区切り(design.md §4.4): genres=`,` / detail_genres=空白 / voice_actor・scenario・illustration・music=`/`。各要素は trim し空要素は捨てる
- タグ再リンク: その作品の **CSV 由来カテゴリのタグ紐付けを一旦全削除して張り直す**。`custom` カテゴリは触らない
- 1 ファイル分は単一トランザクションで処理
- docs/samples/works.csv をテストフィクスチャとして使い、RJ404669 が design.md §4.3 のとおり 14 タグに展開されることをテストする(genre×2 + detail_genre×8 + scenario×1 + illustration×1 + voice_actor×2)

## 11. フロントエンド

- ルーティング: `/`(一覧)、`/works/:id`(詳細+ブラウザ)、`/history`、`/settings`
- 状態管理: オーディオプレイヤーはルート直下のグローバル状態(Context か zustand)。**ページ遷移しても `<audio>` 要素を unmount しない**構造にする(これが SPA 採用の理由)
- プレイヤー Phase 1 はミニモードのみ: 再生/停止、±10 秒、前後トラック、シークバー、再生速度。キューは「再生開始したファイルと同じディレクトリの audio を自然順」で構築(entries API の結果をそのまま使う)
- Media Session API: `play/pause/seekbackward/seekforward/previoustrack/nexttrack` のハンドラと metadata(タイトル・作品名・サムネ)を設定
- 画像ビューア: entries の image を自然順でページ列化。スワイプ(touch)と ←→ キー。次ページ 1 枚プリロード
- UI はスマホ基準(2 カラムグリッド、ファセットはドロワー)。PC は CSS で列数を増やす程度から始める

## 12. Docker

multi-stage ビルド:

1. `node:20` で frontend をビルド
2. `golang:1.25` で dist を embed して `CGO_ENABLED=0 go build`
3. `gcr.io/distroless/static`(または alpine)へバイナリのみコピー

compose 例は design.md §10 参照。

## 13. Phase 1 推奨実装順と完了条件

推奨順(各ステップで動作確認できる単位):

1. backend 骨格: 設定読み込み → DB オープン → マイグレーション → chi 起動 → `/api/healthz`
2. スキャナ + `POST /api/scan`(testdata のダミーフォルダ構成でテスト)
3. CSV インポータ + `POST /api/import/csv`(samples/works.csv でテスト)
4. works / tags の参照系 API(検索・ファセット)
5. entries + file 配信(パス検証・自然順・Range)
6. サムネイル生成・配信
7. play_history(POST/GET)
8. frontend: 一覧 → 詳細+ファイルブラウザ → プレイヤー → 画像ビューア → 履歴 → 設定
9. go:embed + Dockerfile + compose

完了条件(Definition of Done):

- [x] 実フォルダ構成を模した testdata でスキャン・ブラウズ・配信の go test が通る
- [x] samples/works.csv のインポートで RJ404669 が 14 タグに展開される(テストあり)
- [x] パストラバーサルのテスト(§6 のケース)が通る
- [ ] スマホブラウザで: 検索 → 作品選択 → フォルダ移動 → flac 再生(シーク・±10秒・連続再生・ロック画面操作)が動く
- [ ] 画像のページ送り、mp4 再生が動く
- [ ] 履歴画面に再生した作品が出る
- [ ] `docker compose up` だけで起動する
