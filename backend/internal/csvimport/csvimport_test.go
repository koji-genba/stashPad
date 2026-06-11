package csvimport

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB はインメモリ SQLite を開いてスキーマを適用する。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("DB オープン失敗: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	schema := `
	CREATE TABLE works (
		id             INTEGER PRIMARY KEY,
		rj_number      TEXT UNIQUE,
		title          TEXT NOT NULL,
		circle         TEXT,
		series_name    TEXT,
		purchase_date  TEXT,
		work_type      TEXT,
		age_rating     TEXT,
		file_format    TEXT,
		file_size_text TEXT,
		event          TEXT,
		root_path      TEXT,
		thumbnail_path TEXT,
		created_at     TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE tags (
		id       INTEGER PRIMARY KEY,
		name     TEXT NOT NULL,
		category TEXT NOT NULL,
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
		file_path TEXT NOT NULL,
		played_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("スキーマ適用失敗: %v", err)
	}
	return db
}

// samplesCSVPath は docs/samples/works.csv への絶対パスを返す。
func samplesCSVPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller 失敗")
	}
	// このファイルは backend/internal/csvimport/csvimport_test.go
	// stashPad/docs/samples は 3 階層上の docs/samples
	//   csvimport/ -> internal/ -> backend/ -> stashPad/
	dir := filepath.Dir(file)
	path := filepath.Join(dir, "..", "..", "..", "docs", "samples", "works.csv")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestImportSamplesCSV は docs/samples/works.csv のインポートをテスト。
func TestImportSamplesCSV(t *testing.T) {
	db := openTestDB(t)
	csvPath := samplesCSVPath(t)

	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("CSV ファイルオープン失敗: %v", err)
	}
	defer f.Close()

	res, err := Import(db, f)
	if err != nil {
		t.Fatalf("Import 失敗: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Errorf("エラーあり: %v", res.Errors)
	}

	// CSV は 7 行なので 7 件作成
	if res.Created != 7 {
		t.Errorf("Created = %d, want 7", res.Created)
	}
	if res.Updated != 0 {
		t.Errorf("Updated = %d, want 0", res.Updated)
	}

	// works テーブルに 7 件あることを確認
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM works").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 7 {
		t.Errorf("works 件数 = %d, want 7", count)
	}
}

// TestImportRJ404669Tags は RJ404669 が design.md §4.3 の 13 タグに展開されることをテスト。
func TestImportRJ404669Tags(t *testing.T) {
	db := openTestDB(t)
	csvPath := samplesCSVPath(t)

	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("CSV ファイルオープン失敗: %v", err)
	}
	defer f.Close()

	if _, err := Import(db, f); err != nil {
		t.Fatalf("Import 失敗: %v", err)
	}

	// RJ404669 の work_id を取得
	var workID int64
	if err := db.QueryRow("SELECT id FROM works WHERE rj_number='RJ404669'").Scan(&workID); err != nil {
		t.Fatalf("RJ404669 が見つからない: %v", err)
	}

	// タグ件数
	// design.md §4.3 の表を数えると:
	//   genre×2 + detail_genre×8 + scenario×1 + illustration×1 + voice_actor×2 = 14
	// 本文の「計13タグ」は誤植(表の行数が正しい)
	var tagCount int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM work_tags WHERE work_id=?", workID,
	).Scan(&tagCount); err != nil {
		t.Fatal(err)
	}
	if tagCount != 14 {
		t.Errorf("RJ404669 のタグ件数 = %d, want 14", tagCount)
	}

	// 期待するタグ一覧(design.md §4.3)
	expected := []struct {
		category string
		name     string
	}{
		{"genre", "R-15"},
		{"genre", "ボイス・ASMR"},
		{"detail_genre", "ASMR"},
		{"detail_genre", "癒し"},
		{"detail_genre", "淡白/あっさり"},
		{"detail_genre", "バイノーラル/ダミヘ"},
		{"detail_genre", "耳かき"},
		{"detail_genre", "ラブラブ/あまあま"},
		{"detail_genre", "ささやき"},
		{"detail_genre", "耳舐め"},
		{"scenario", "カマキリ"},
		{"illustration", "葉月かなめ"},
		{"voice_actor", "耳恋なか"},
		// voice_actor の 2 つ目は実際に展開されるので 13 タグは以下を含む
	}

	// voice_actor が 2 件(耳恋なか / 箱河ノア)であることも確認
	var vaCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM work_tags wt
		 JOIN tags t ON wt.tag_id=t.id
		 WHERE wt.work_id=? AND t.category='voice_actor'`, workID,
	).Scan(&vaCount); err != nil {
		t.Fatal(err)
	}
	if vaCount != 2 {
		t.Errorf("voice_actor タグ件数 = %d, want 2", vaCount)
	}

	// 各タグが存在することを確認
	for _, exp := range expected {
		var exists bool
		err := db.QueryRow(
			`SELECT EXISTS(
				SELECT 1 FROM work_tags wt
				JOIN tags t ON wt.tag_id=t.id
				WHERE wt.work_id=? AND t.category=? AND t.name=?
			)`, workID, exp.category, exp.name,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("タグ存在確認失敗 %s/%s: %v", exp.category, exp.name, err)
		}
		if !exists {
			t.Errorf("タグが存在しない: category=%s, name=%s", exp.category, exp.name)
		}
	}
}

// TestImportRJ404669WorkRow は RJ404669 の works テーブルデータが正しいことをテスト。
func TestImportRJ404669WorkRow(t *testing.T) {
	db := openTestDB(t)
	csvPath := samplesCSVPath(t)

	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("CSV ファイルオープン失敗: %v", err)
	}
	defer f.Close()

	if _, err := Import(db, f); err != nil {
		t.Fatalf("Import 失敗: %v", err)
	}

	type workData struct {
		rjNumber     string
		title        string
		seriesName   sql.NullString
		circle       sql.NullString
		purchaseDate sql.NullString
		workType     sql.NullString
		fileFormat   sql.NullString
		fileSizeText sql.NullString
		ageRating    sql.NullString
	}
	var w workData
	err = db.QueryRow(
		`SELECT rj_number, title, series_name, circle, purchase_date,
		        work_type, file_format, file_size_text, age_rating
		 FROM works WHERE rj_number='RJ404669'`,
	).Scan(&w.rjNumber, &w.title, &w.seriesName, &w.circle, &w.purchaseDate,
		&w.workType, &w.fileFormat, &w.fileSizeText, &w.ageRating)
	if err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}

	if w.title != "耳舐め&耳ふ～サンドイッチ ダウナー妹と低音姉【R-15癒し/CV:箱河ノアさま・耳恋なかさま】" {
		t.Errorf("title = %q", w.title)
	}
	if w.seriesName.String != "ぐっすり眠れるASMR" {
		t.Errorf("series_name = %q", w.seriesName.String)
	}
	if w.circle.String != "チームランドセル" {
		t.Errorf("circle = %q", w.circle.String)
	}
	if w.purchaseDate.String != "2026/01/04 10:44" {
		t.Errorf("purchase_date = %q", w.purchaseDate.String)
	}
	if w.workType.String != "ボイス・ASMR" {
		t.Errorf("work_type = %q", w.workType.String)
	}
	if w.ageRating.String != "R-15" {
		t.Errorf("age_rating = %q", w.ageRating.String)
	}

	// 全年齢作品は genres カラムに「全年齢」を含まないため、
	// age_rating カラム自体から取り込まれていることを確認する(回帰テスト)
	var allAges sql.NullString
	if err := db.QueryRow(
		"SELECT age_rating FROM works WHERE rj_number='RJ01547274'",
	).Scan(&allAges); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if allAges.String != "全年齢" {
		t.Errorf("RJ01547274 age_rating = %q, want 全年齢", allAges.String)
	}
}

// TestImportUpsert は再インポートで重複しないことをテスト。
func TestImportUpsert(t *testing.T) {
	db := openTestDB(t)

	csvData := `rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music
RJ999999,テスト作品,シリーズ,サークル,2026/01/01,ボイス・ASMR,ASMR,ボイス・ASMR,MP3,1GB,,全年齢,,,,,
`

	// 1回目
	res1, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("1回目 Import 失敗: %v", err)
	}
	if res1.Created != 1 {
		t.Errorf("1回目 Created = %d, want 1", res1.Created)
	}

	// 2回目
	res2, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("2回目 Import 失敗: %v", err)
	}
	if res2.Created != 0 || res2.Updated != 1 {
		t.Errorf("2回目 Created=%d Updated=%d, want Created=0 Updated=1", res2.Created, res2.Updated)
	}

	// works 件数が変わっていない
	var count int
	db.QueryRow("SELECT COUNT(*) FROM works").Scan(&count)
	if count != 1 {
		t.Errorf("works 件数 = %d, want 1", count)
	}
}

// TestImportCustomTagPreserved は CSV 再インポートで custom タグが保持されることをテスト。
func TestImportCustomTagPreserved(t *testing.T) {
	db := openTestDB(t)

	csvData := `rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music
RJ888888,カスタムタグテスト,,,,,ASMR,ボイス・ASMR,MP3,1GB,,全年齢,,,,,
`

	// 1回目インポート
	if _, err := Import(db, strings.NewReader(csvData)); err != nil {
		t.Fatal(err)
	}

	// custom タグを手動追加
	var workID int64
	db.QueryRow("SELECT id FROM works WHERE rj_number='RJ888888'").Scan(&workID)

	db.Exec("INSERT OR IGNORE INTO tags (name, category) VALUES ('睡眠用', 'custom')")
	var tagID int64
	db.QueryRow("SELECT id FROM tags WHERE name='睡眠用' AND category='custom'").Scan(&tagID)
	db.Exec("INSERT OR IGNORE INTO work_tags (work_id, tag_id) VALUES (?, ?)", workID, tagID)

	// 2回目インポート
	if _, err := Import(db, strings.NewReader(csvData)); err != nil {
		t.Fatal(err)
	}

	// custom タグが保持されているか
	var exists bool
	db.QueryRow(
		`SELECT EXISTS(
			SELECT 1 FROM work_tags wt
			JOIN tags t ON wt.tag_id=t.id
			WHERE wt.work_id=? AND t.category='custom' AND t.name='睡眠用'
		)`, workID,
	).Scan(&exists)
	if !exists {
		t.Error("custom タグが削除された(保持されるべき)")
	}
}

// TestImportLinkedCount は既存 root_path 付き works に CSV がリンクされた数のテスト。
func TestImportLinkedCount(t *testing.T) {
	db := openTestDB(t)

	// root_path が設定されている works を事前に登録
	db.Exec(
		`INSERT INTO works (rj_number, title, root_path)
		 VALUES ('RJ777777', 'スキャン済み', '/media/RJ777777_作品')`,
	)

	csvData := `rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music
RJ777777,CSVのタイトル,,,,,ASMR,ボイス・ASMR,MP3,1GB,,全年齢,,,,,
`

	res, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("Import 失敗: %v", err)
	}

	if res.Linked != 1 {
		t.Errorf("Linked = %d, want 1", res.Linked)
	}
}

// TestImportBOM は BOM 付き UTF-8 CSV が正しく処理されることをテスト。
func TestImportBOM(t *testing.T) {
	db := openTestDB(t)

	// BOM 付き CSV
	bom := "\xef\xbb\xbf"
	csvData := bom + `rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music
RJ111111,BOMテスト,,,,,ASMR,ボイス・ASMR,MP3,1GB,,全年齢,,,,,
`

	res, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("Import 失敗: %v", err)
	}
	if res.Created != 1 {
		t.Errorf("Created = %d, want 1", res.Created)
	}

	// rj_number が正しく登録されているか
	var rj string
	db.QueryRow("SELECT rj_number FROM works WHERE rj_number='RJ111111'").Scan(&rj)
	if rj != "RJ111111" {
		t.Errorf("rj_number = %q, want RJ111111", rj)
	}
}

// TestParseTags はタグ解析のユニットテスト。
func TestParseTags(t *testing.T) {
	tags := parseTags(
		"R-15, ボイス・ASMR",
		"ASMR 癒し 淡白/あっさり",
		"耳恋なか/箱河ノア",
		"カマキリ",
		"葉月かなめ",
		"",
	)

	// genres(カンマ区切り): 2 件
	// detail_genres(空白区切り): 3 件
	// voice_actor(スラッシュ区切り): 2 件
	// scenario: 1 件
	// illustration: 1 件
	// music: 0 件
	// 合計 9 件
	if len(tags) != 9 {
		t.Errorf("タグ件数 = %d, want 9", len(tags))
	}

	// detail_genre の "淡白/あっさり" はタグ名に "/" を含む(区切りは空白)ことを確認
	found := false
	for _, tag := range tags {
		if tag.Category == "detail_genre" && tag.Name == "淡白/あっさり" {
			found = true
			break
		}
	}
	if !found {
		t.Error("detail_genre '淡白/あっさり' が見つからない(空白区切りで分割されるべき)")
	}
}

// TestSplitTrim は splitTrim のユニットテスト。
func TestSplitTrim(t *testing.T) {
	cases := []struct {
		s    string
		sep  string
		want []string
	}{
		{"a, b , c", ",", []string{"a", "b", "c"}},
		{"a b  c", " ", []string{"a", "b", "c"}},
		{"a/b/c", "/", []string{"a", "b", "c"}},
		{"", ",", nil},
		{"  ", ",", nil},
		{"a//b", "/", []string{"a", "b"}},
	}

	for _, tc := range cases {
		got := splitTrim(tc.s, tc.sep)
		if len(got) != len(tc.want) {
			t.Errorf("splitTrim(%q, %q) = %v, want %v", tc.s, tc.sep, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitTrim(%q, %q)[%d] = %q, want %q", tc.s, tc.sep, i, got[i], tc.want[i])
			}
		}
	}
}

// TestBOMReaderShortRead は bomReader を 1 バイトずつ読んでも全データが正しく読めることを確認する。
// len(p) < len(b.buf) の場合に copy の戻り値ではなく len(b.buf) を返していたバグの回帰テスト。
func TestBOMReaderShortRead(t *testing.T) {
	t.Helper()
	// BOM(3バイト) + 'a' + 'b' の 5 バイト入力
	input := []byte{0xEF, 0xBB, 0xBF, 'a', 'b'}
	br := &bomReader{r: bytes.NewReader(input)}

	// 1バイトずつ読んで結果を連結する
	var result []byte
	buf := make([]byte, 1)
	for {
		n, err := br.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read エラー: %v", err)
		}
	}

	// BOM を除いた 'a', 'b' が読めること
	want := []byte{'a', 'b'}
	if !bytes.Equal(result, want) {
		t.Errorf("読み込み結果 = %v, want %v", result, want)
	}
}

// TestImportErrorsAlwaysArray は errors が nil でなく常に配列で返ることをテスト。
// nil だと JSON で "errors": null になりフロントエンドが落ちる(回帰テスト)。
func TestImportErrorsAlwaysArray(t *testing.T) {
	db := openTestDB(t)
	csvData := `rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music
RJ555555,正常行,,,,,ASMR,ボイス・ASMR,MP3,1GB,,全年齢,,,,,
`
	res, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatal(err)
	}
	if res.Errors == nil {
		t.Error("Errors が nil(JSON で null になる)。空スライスであるべき")
	}
}
