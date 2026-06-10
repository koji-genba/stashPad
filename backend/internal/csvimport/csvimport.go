// Package csvimport は DLsite 作品情報 CSV のインポートを担当する。
// implementation-notes.md §10 の仕様に従う。
package csvimport

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// Result は CSV インポートの結果サマリ。POST /api/import/csv のレスポンスに使う。
type Result struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Linked  int      `json:"linked"`
	Errors  []string `json:"errors"`
}

// CSV カテゴリ名の定数(タグ再リンク時に削除するカテゴリ)。
var csvCategories = []string{
	"genre",
	"detail_genre",
	"voice_actor",
	"scenario",
	"illustration",
	"music",
}

// Import は reader から CSV を読み込み、単一トランザクションで upsert する。
// BOM は自動的に除去する。
func Import(db *sql.DB, reader io.Reader) (Result, error) {
	var res Result

	// BOM 除去
	br := newBOMReader(reader)

	csvReader := csv.NewReader(br)
	csvReader.LazyQuotes = true

	// ヘッダ行を読む
	header, err := csvReader.Read()
	if err != nil {
		return res, fmt.Errorf("ヘッダ読み込み失敗: %w", err)
	}
	colIdx := buildColumnIndex(header)

	// 必須カラムチェック
	if _, ok := colIdx["rj_number"]; !ok {
		return res, fmt.Errorf("CSV に rj_number カラムがありません")
	}

	// 全行読み込み
	records, err := csvReader.ReadAll()
	if err != nil {
		return res, fmt.Errorf("CSV 読み込み失敗: %w", err)
	}

	// 単一トランザクション
	tx, err := db.Begin()
	if err != nil {
		return res, fmt.Errorf("トランザクション開始失敗: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	for i, record := range records {
		if err2 := importRow(tx, &res, colIdx, record, i+2); err2 != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("行%d: %v", i+2, err2))
		}
	}

	if err = tx.Commit(); err != nil {
		return res, fmt.Errorf("コミット失敗: %w", err)
	}
	return res, nil
}

// importRow は 1 行分のデータを処理する。
func importRow(tx *sql.Tx, res *Result, colIdx map[string]int, record []string, lineNum int) error {
	get := func(col string) string {
		idx, ok := colIdx[col]
		if !ok || idx >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[idx])
	}

	rjNumber := get("rj_number")
	if rjNumber == "" {
		return fmt.Errorf("rj_number が空")
	}

	title := get("title")
	if title == "" {
		title = rjNumber
	}

	// works upsert
	workID, created, linked, err := upsertWork(tx, WorkRow{
		RJNumber:     rjNumber,
		Title:        title,
		SeriesName:   get("series_name"),
		Circle:       get("circle"),
		PurchaseDate: get("purchase_date"),
		WorkType:     get("work_type"),
		FileFormat:   get("file_format"),
		FileSizeText: get("file_size"),
		AgeRating:    get("age_rating"),
		Event:        get("event"),
	})
	if err != nil {
		return fmt.Errorf("works upsert 失敗: %w", err)
	}

	if created {
		res.Created++
	} else {
		res.Updated++
	}
	if linked {
		res.Linked++
	}

	// CSV 由来カテゴリのタグを全削除
	if err := deleteCSVTags(tx, workID); err != nil {
		return fmt.Errorf("CSV タグ削除失敗: %w", err)
	}

	// タグを展開して再リンク
	tags := parseTags(get("genres"), get("detail_genres"), get("voice_actor"),
		get("scenario"), get("illustration"), get("music"))
	for _, tag := range tags {
		if err := linkTag(tx, workID, tag.Name, tag.Category); err != nil {
			return fmt.Errorf("タグリンク失敗 %q/%q: %w", tag.Category, tag.Name, err)
		}
	}

	return nil
}

// WorkRow は works テーブルへの挿入データをまとめた構造体。
type WorkRow struct {
	RJNumber     string
	Title        string
	SeriesName   string
	Circle       string
	PurchaseDate string
	WorkType     string
	FileFormat   string
	FileSizeText string
	AgeRating    string
	Event        string
}

// upsertWork は rj_number をキーに works を upsert する。
// 返り値: (workID, 新規作成か, root_path がリンクされたか)
func upsertWork(tx *sql.Tx, row WorkRow) (workID int64, created bool, linked bool, err error) {
	var id int64
	var currentRootPath sql.NullString

	scanErr := tx.QueryRow(
		"SELECT id, root_path FROM works WHERE rj_number=?",
		row.RJNumber,
	).Scan(&id, &currentRootPath)

	if scanErr == sql.ErrNoRows {
		// 新規作成
		res, insErr := tx.Exec(
			`INSERT INTO works
				(rj_number, title, series_name, circle, purchase_date,
				 work_type, file_format, file_size_text, age_rating, event,
				 updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,datetime('now'))`,
			nullIfEmpty(row.RJNumber), row.Title,
			nullIfEmpty(row.SeriesName), nullIfEmpty(row.Circle),
			nullIfEmpty(row.PurchaseDate), nullIfEmpty(row.WorkType),
			nullIfEmpty(row.FileFormat), nullIfEmpty(row.FileSizeText),
			nullIfEmpty(row.AgeRating), nullIfEmpty(row.Event),
		)
		if insErr != nil {
			return 0, false, false, insErr
		}
		id, err = res.LastInsertId()
		return id, true, false, err
	}
	if scanErr != nil {
		return 0, false, false, scanErr
	}

	// 既存行の更新。root_path が NULL で今回も NULL なら linked=false、
	// スキャン済みで root_path が付いていれば「CSV がリンクされた」= linked=true。
	linkedFlag := currentRootPath.Valid

	_, uErr := tx.Exec(
		`UPDATE works SET
			title=?, series_name=?, circle=?, purchase_date=?,
			work_type=?, file_format=?, file_size_text=?, age_rating=?, event=?,
			updated_at=datetime('now')
		 WHERE id=?`,
		row.Title,
		nullIfEmpty(row.SeriesName), nullIfEmpty(row.Circle),
		nullIfEmpty(row.PurchaseDate), nullIfEmpty(row.WorkType),
		nullIfEmpty(row.FileFormat), nullIfEmpty(row.FileSizeText),
		nullIfEmpty(row.AgeRating), nullIfEmpty(row.Event),
		id,
	)
	if uErr != nil {
		return 0, false, false, uErr
	}
	return id, false, linkedFlag, nil
}

// deleteCSVTags は work_id の CSV 由来カテゴリのタグ紐付けを削除する。
// custom カテゴリは触らない。
func deleteCSVTags(tx *sql.Tx, workID int64) error {
	placeholders := make([]string, len(csvCategories))
	args := make([]any, len(csvCategories)+1)
	args[0] = workID
	for i, cat := range csvCategories {
		placeholders[i] = "?"
		args[i+1] = cat
	}
	query := fmt.Sprintf(
		`DELETE FROM work_tags WHERE work_id=?
		 AND tag_id IN (SELECT id FROM tags WHERE category IN (%s))`,
		strings.Join(placeholders, ","),
	)
	_, err := tx.Exec(query, args...)
	return err
}

// linkTag は tags テーブルに (name, category) を upsert してから work_tags にリンクする。
func linkTag(tx *sql.Tx, workID int64, name, category string) error {
	// tags upsert
	_, err := tx.Exec(
		`INSERT INTO tags (name, category) VALUES (?,?)
		 ON CONFLICT(name, category) DO NOTHING`,
		name, category,
	)
	if err != nil {
		return fmt.Errorf("tags INSERT 失敗: %w", err)
	}

	var tagID int64
	if err := tx.QueryRow(
		"SELECT id FROM tags WHERE name=? AND category=?", name, category,
	).Scan(&tagID); err != nil {
		return fmt.Errorf("tag_id 取得失敗: %w", err)
	}

	// work_tags upsert(重複は無視)
	_, err = tx.Exec(
		`INSERT INTO work_tags (work_id, tag_id) VALUES (?,?)
		 ON CONFLICT(work_id, tag_id) DO NOTHING`,
		workID, tagID,
	)
	return err
}

// TagEntry はカテゴリと名前のペア。
type TagEntry struct {
	Category string
	Name     string
}

// parseTags は各フィールドを区切り文字で分割してタグエントリのスライスを返す。
func parseTags(genres, detailGenres, voiceActor, scenario, illustration, music string) []TagEntry {
	var tags []TagEntry

	// genres: カンマ区切り → category=genre
	// ただし age_rating(全年齢/R-15/R-18)は genres カラムに混じっているので
	// そのままタグとして扱う(design.md §4.3 の例がそうなっている)
	for _, s := range splitTrim(genres, ",") {
		tags = append(tags, TagEntry{Category: "genre", Name: s})
	}

	// detail_genres: 空白区切り → category=detail_genre
	for _, s := range splitTrim(detailGenres, " ") {
		tags = append(tags, TagEntry{Category: "detail_genre", Name: s})
	}

	// voice_actor, scenario, illustration, music: スラッシュ区切り
	for _, s := range splitTrim(voiceActor, "/") {
		tags = append(tags, TagEntry{Category: "voice_actor", Name: s})
	}
	for _, s := range splitTrim(scenario, "/") {
		tags = append(tags, TagEntry{Category: "scenario", Name: s})
	}
	for _, s := range splitTrim(illustration, "/") {
		tags = append(tags, TagEntry{Category: "illustration", Name: s})
	}
	for _, s := range splitTrim(music, "/") {
		tags = append(tags, TagEntry{Category: "music", Name: s})
	}

	return tags
}

// splitTrim は s を sep で分割し、各要素をトリムして空要素を除いたスライスを返す。
func splitTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// buildColumnIndex はヘッダ行からカラム名→インデックスのマップを作る。
func buildColumnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		h = strings.TrimSpace(h)
		// BOM が残っていた場合に備えて先頭の BOM 文字を除去
		h = strings.TrimPrefix(h, "\xef\xbb\xbf")
		idx[h] = i
	}
	return idx
}

// nullIfEmpty は空文字列を sql.NullString{Valid: false} に変換する。
func nullIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// bomReader は UTF-8 BOM を先頭から除いた io.Reader を提供する。
type bomReader struct {
	r       io.Reader
	checked bool
	buf     []byte
}

// newBOMReader は BOM を除去する Reader を返す。
func newBOMReader(r io.Reader) io.Reader {
	return &bomReader{r: r}
}

func (b *bomReader) Read(p []byte) (int, error) {
	if !b.checked {
		b.checked = true
		// 先頭 3 バイトを読んで BOM (0xEF 0xBB 0xBF) を確認
		head := make([]byte, 3)
		n, err := io.ReadFull(b.r, head)
		if n > 0 {
			if n == 3 && head[0] == 0xEF && head[1] == 0xBB && head[2] == 0xBF {
				// BOM を除去
			} else {
				b.buf = head[:n]
			}
		}
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			// ファイルが 3 バイト未満
			copy(p, b.buf)
			ret := len(b.buf)
			b.buf = nil
			return ret, io.EOF
		}
		if err != nil {
			return 0, err
		}
	}

	if len(b.buf) > 0 {
		n := copy(p, b.buf)
		b.buf = b.buf[n:]
		return n, nil
	}
	return b.r.Read(p)
}
