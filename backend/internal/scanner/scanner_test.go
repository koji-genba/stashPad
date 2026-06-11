package scanner

import (
	"database/sql"
	"os"
	"path/filepath"
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

// setupLibrary はテスト用ライブラリディレクトリ構成を作る。
//
//	library/
//	├── RJ404669_耳舐め作品/
//	│   └── cover.jpg
//	├── RJ304928_別の作品/
//	│   └── 表紙.png
//	├── 非RJフォルダ/
//	│   └── main.jpg
//	└── not_a_dir.txt
func setupLibrary(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	lib := filepath.Join(base, "library")

	dirs := []string{
		filepath.Join(lib, "RJ404669_耳舐め作品"),
		filepath.Join(lib, "RJ304928_別の作品"),
		filepath.Join(lib, "非RJフォルダ"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// ダミー画像ファイル
	files := map[string]string{
		filepath.Join(lib, "RJ404669_耳舐め作品", "cover.jpg"): "img",
		filepath.Join(lib, "RJ304928_別の作品", "表紙.png"):     "img",
		filepath.Join(lib, "非RJフォルダ", "main.jpg"):         "img",
		filepath.Join(lib, "not_a_dir.txt"):               "text",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return lib
}

// TestScanNewWorks は RJ フォルダと非 RJ フォルダが正しく登録されることをテスト。
func TestScanNewWorks(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	res, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}

	// ファイルは無視され、ディレクトリ 3 件が見つかる
	if res.WorksFound != 3 {
		t.Errorf("WorksFound = %d, want 3", res.WorksFound)
	}
	// 全件新規
	if res.NewlyRegistered != 3 {
		t.Errorf("NewlyRegistered = %d, want 3", res.NewlyRegistered)
	}
	if res.LinkedToCSV != 0 {
		t.Errorf("LinkedToCSV = %d, want 0", res.LinkedToCSV)
	}
	if res.MissingMarked != 0 {
		t.Errorf("MissingMarked = %d, want 0", res.MissingMarked)
	}

	// RJ 番号が正しく登録されているか確認
	var rjNumber sql.NullString
	err = db.QueryRow("SELECT rj_number FROM works WHERE root_path LIKE '%RJ404669%'").Scan(&rjNumber)
	if err != nil {
		t.Fatalf("RJ404669 の検索失敗: %v", err)
	}
	if !rjNumber.Valid || rjNumber.String != "RJ404669" {
		t.Errorf("rj_number = %v, want RJ404669", rjNumber)
	}

	// 非 RJ フォルダは rj_number が NULL で登録されているか
	var nonRJTitle string
	var nonRJRJ sql.NullString
	err = db.QueryRow("SELECT title, rj_number FROM works WHERE root_path LIKE '%非RJフォルダ%'").Scan(&nonRJTitle, &nonRJRJ)
	if err != nil {
		t.Fatalf("非RJフォルダの検索失敗: %v", err)
	}
	if nonRJRJ.Valid {
		t.Errorf("非RJフォルダの rj_number = %v, want NULL", nonRJRJ)
	}
	if nonRJTitle != "非RJフォルダ" {
		t.Errorf("非RJフォルダの title = %q, want 非RJフォルダ", nonRJTitle)
	}
}

// TestScanLinkToCSV は CSV 先行登録済み work に root_path がリンクされることをテスト。
func TestScanLinkToCSV(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	// CSV 先行登録: RJ404669 が root_path NULL で存在する
	_, err := db.Exec(
		`INSERT INTO works (rj_number, title) VALUES ('RJ404669', 'CSVから入ったタイトル')`,
	)
	if err != nil {
		t.Fatalf("事前 INSERT 失敗: %v", err)
	}

	res, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}

	// RJ404669 は既存なので新規登録ではなくリンク
	if res.LinkedToCSV != 1 {
		t.Errorf("LinkedToCSV = %d, want 1", res.LinkedToCSV)
	}
	// 新規は非RJ + RJ304928 の 2 件
	if res.NewlyRegistered != 2 {
		t.Errorf("NewlyRegistered = %d, want 2", res.NewlyRegistered)
	}

	// root_path がリンクされているか
	var rootPath sql.NullString
	err = db.QueryRow("SELECT root_path FROM works WHERE rj_number='RJ404669'").Scan(&rootPath)
	if err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if !rootPath.Valid {
		t.Error("root_path は NULL であってはならない")
	}
	expected := filepath.Join(lib, "RJ404669_耳舐め作品")
	if rootPath.String != expected {
		t.Errorf("root_path = %q, want %q", rootPath.String, expected)
	}
}

// TestScanMissingMarked はフォルダが消えた work の root_path が NULL に戻されることをテスト。
func TestScanMissingMarked(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	// 1回目のスキャンで登録
	_, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}

	// RJ404669 のフォルダを削除
	rjDir := filepath.Join(lib, "RJ404669_耳舐め作品")
	if err := os.RemoveAll(rjDir); err != nil {
		t.Fatalf("フォルダ削除失敗: %v", err)
	}

	// 2回目のスキャン
	res, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("2回目スキャン失敗: %v", err)
	}

	if res.MissingMarked != 1 {
		t.Errorf("MissingMarked = %d, want 1", res.MissingMarked)
	}

	// root_path が NULL に戻っているか
	var rootPath sql.NullString
	err = db.QueryRow("SELECT root_path FROM works WHERE rj_number='RJ404669'").Scan(&rootPath)
	if err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if rootPath.Valid {
		t.Errorf("root_path = %q, want NULL", rootPath.String)
	}
}

// TestScanIdempotent は再スキャンで重複登録されないことをテスト。
func TestScanIdempotent(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	res1, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}
	res2, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("2回目スキャン失敗: %v", err)
	}

	// 2回目は新規登録なし
	if res2.NewlyRegistered != 0 {
		t.Errorf("2回目 NewlyRegistered = %d, want 0", res2.NewlyRegistered)
	}

	// DB の works 件数が変わっていない
	var count1, count2 int
	db.QueryRow("SELECT COUNT(*) FROM works").Scan(&count2)
	_ = res1
	_ = count1
	if count2 != 3 {
		t.Errorf("works 件数 = %d, want 3", count2)
	}
}

// TestScanTitleExtraction はフォルダ名からタイトルが正しく抽出されることをテスト。
func TestScanTitleExtraction(t *testing.T) {
	db := openTestDB(t)
	base := t.TempDir()
	lib := filepath.Join(base, "library")

	// タイトル抽出パターンのテスト用フォルダ
	testDirs := []struct {
		dirName       string
		expectedTitle string
	}{
		{"RJ404669_耳舐め作品タイトル", "耳舐め作品タイトル"},
		{"RJ01234567_長いRJ番号の作品", "長いRJ番号の作品"},
		{"RJ123456", "RJ123456"}, // アンダースコアなし → フォルダ名全体
	}

	for _, tc := range testDirs {
		dirPath := filepath.Join(lib, tc.dirName)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	_, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}

	for _, tc := range testDirs {
		tc := tc
		t.Run(tc.dirName, func(t *testing.T) {
			var title string
			err := db.QueryRow(
				"SELECT title FROM works WHERE root_path=?",
				filepath.Join(lib, tc.dirName),
			).Scan(&title)
			if err != nil {
				t.Errorf("タイトル取得失敗 %q: %v", tc.dirName, err)
				return
			}
			if title != tc.expectedTitle {
				t.Errorf("フォルダ %q のタイトル = %q, want %q", tc.dirName, title, tc.expectedTitle)
			}
		})
	}
}
