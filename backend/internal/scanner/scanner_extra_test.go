package scanner

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// fakeThumbGen は ThumbnailGenerator インターフェースのフェイク実装。
// 呼び出しを記録し、設定に応じてパスまたはエラーを返す。
type fakeThumbGen struct {
	mu      sync.Mutex
	calls   []thumbCall
	errors  map[string]error // absPath → 返すエラー
	empties map[string]bool  // absPath → 空文字を返すか
}

type thumbCall struct {
	workID  int64
	absPath string
}

func newFakeThumbGen() *fakeThumbGen {
	return &fakeThumbGen{
		errors:  make(map[string]error),
		empties: make(map[string]bool),
	}
}

func (f *fakeThumbGen) Generate(workID int64, rootPath string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, thumbCall{workID: workID, absPath: rootPath})
	if err, ok := f.errors[rootPath]; ok {
		return "", err
	}
	if f.empties[rootPath] {
		return "", nil
	}
	return filepath.Join("/thumbs", fmt.Sprintf("%d.jpg", workID)), nil
}

// TestScanWithThumbnailGenerator はサムネイル生成経路のテスト。
// フェイク ThumbnailGenerator を渡して thumbnail_path が DB に保存されることを確認。
func TestScanWithThumbnailGenerator(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	gen := newFakeThumbGen()
	res, err := Scan(db, []string{lib}, gen)
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}
	if res.WorksFound != 3 {
		t.Errorf("WorksFound = %d, want 3", res.WorksFound)
	}
	if res.NewlyRegistered != 3 {
		t.Errorf("NewlyRegistered = %d, want 3", res.NewlyRegistered)
	}

	// 全 work の thumbnail_path が更新されていることを確認
	rows, err := db.Query("SELECT id, thumbnail_path FROM works ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id int64
		var thumbPath sql.NullString
		if err := rows.Scan(&id, &thumbPath); err != nil {
			t.Fatal(err)
		}
		if !thumbPath.Valid || thumbPath.String == "" {
			t.Errorf("work id=%d の thumbnail_path が空または NULL", id)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("works 件数 = %d, want 3", count)
	}

	// Generate が 3 回呼ばれたことを確認
	gen.mu.Lock()
	callCount := len(gen.calls)
	gen.mu.Unlock()
	if callCount != 3 {
		t.Errorf("Generate 呼び出し回数 = %d, want 3", callCount)
	}
}

// TestScanThumbnailGeneratorError は Generate がエラーを返す work が混ざっても
// 他の work の thumbnail_path は更新されることをテスト。
func TestScanThumbnailGeneratorError(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	gen := newFakeThumbGen()
	// RJ404669 のフォルダはエラーを返す
	rjPath := filepath.Join(lib, "RJ404669_耳舐め作品")
	gen.errors[rjPath] = errors.New("サムネイル生成エラー(テスト)")

	res, err := Scan(db, []string{lib}, gen)
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}
	if res.WorksFound != 3 {
		t.Errorf("WorksFound = %d, want 3", res.WorksFound)
	}

	// エラーが発生したフォルダの thumbnail_path は NULL のまま
	var rjThumb sql.NullString
	if err := db.QueryRow(
		"SELECT thumbnail_path FROM works WHERE root_path=?", rjPath,
	).Scan(&rjThumb); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if rjThumb.Valid && rjThumb.String != "" {
		t.Errorf("エラー発生 work の thumbnail_path = %q, want 空/NULL", rjThumb.String)
	}

	// 他の 2 件は thumbnail_path が更新されている
	rows, err := db.Query(
		"SELECT thumbnail_path FROM works WHERE root_path != ?", rjPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tp sql.NullString
		if err := rows.Scan(&tp); err != nil {
			t.Fatal(err)
		}
		if !tp.Valid || tp.String == "" {
			t.Error("エラーでない work の thumbnail_path が空/NULL")
		}
	}
}

// TestScanThumbnailGeneratorEmpty は Generate が空文字を返した場合、
// thumbnail_path が更新されないことをテスト。
func TestScanThumbnailGeneratorEmpty(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	gen := newFakeThumbGen()
	// RJ304928 のフォルダは空文字を返す
	rjPath := filepath.Join(lib, "RJ304928_別の作品")
	gen.empties[rjPath] = true

	_, err := Scan(db, []string{lib}, gen)
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}

	// 空文字を返した work の thumbnail_path は NULL のまま
	var thumbPath sql.NullString
	if err := db.QueryRow(
		"SELECT thumbnail_path FROM works WHERE root_path=?", rjPath,
	).Scan(&thumbPath); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if thumbPath.Valid && thumbPath.String != "" {
		t.Errorf("空文字返却 work の thumbnail_path = %q, want 空/NULL", thumbPath.String)
	}
}

// TestScanNilThumbnailGenerator は thumbGen=nil の場合にサムネイル生成がスキップされることをテスト。
func TestScanNilThumbnailGenerator(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	_, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}

	// thumbnail_path は全件 NULL
	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM works WHERE thumbnail_path IS NOT NULL",
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("thumbGen=nil なのに thumbnail_path が %d 件設定された", count)
	}
}

// TestScanInvalidRootContinues は存在しないルートが混ざっていても
// 有効なルートは処理されることをテスト。
func TestScanInvalidRootContinues(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	invalidRoot := "/nonexistent/path/that/does/not/exist"
	res, err := Scan(db, []string{invalidRoot, lib}, nil)
	if err != nil {
		t.Fatalf("Scan 失敗(エラーは内部ログに留まり続行すべき): %v", err)
	}

	// 有効なルートのフォルダは処理される
	if res.WorksFound != 3 {
		t.Errorf("WorksFound = %d, want 3", res.WorksFound)
	}
	if res.NewlyRegistered != 3 {
		t.Errorf("NewlyRegistered = %d, want 3", res.NewlyRegistered)
	}
}

// TestScanIdempotentWithCount は同一ライブラリを 2 回 Scan しても
// 2 回目は NewlyRegistered=0 であることをテスト。
func TestScanIdempotentWithCount(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	_, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}

	res2, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("2回目スキャン失敗: %v", err)
	}

	if res2.NewlyRegistered != 0 {
		t.Errorf("2回目 NewlyRegistered = %d, want 0", res2.NewlyRegistered)
	}
	if res2.LinkedToCSV != 0 {
		t.Errorf("2回目 LinkedToCSV = %d, want 0", res2.LinkedToCSV)
	}
	if res2.MissingMarked != 0 {
		t.Errorf("2回目 MissingMarked = %d, want 0", res2.MissingMarked)
	}
}

// TestScanMissingAndRestore はフォルダ削除後の再 Scan で root_path が NULL になり、
// フォルダ復活後の再 Scan で root_path が再リンクされることをテスト。
func TestScanMissingAndRestore(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	// 1回目: 全部登録
	_, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}

	// RJ404669 のフォルダを削除
	rjDir := filepath.Join(lib, "RJ404669_耳舐め作品")
	if err := os.RemoveAll(rjDir); err != nil {
		t.Fatalf("フォルダ削除失敗: %v", err)
	}

	// 2回目: フォルダ削除後スキャン
	res2, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("2回目スキャン失敗: %v", err)
	}
	if res2.MissingMarked != 1 {
		t.Errorf("MissingMarked = %d, want 1", res2.MissingMarked)
	}

	// root_path が NULL になっているか
	var rootPath sql.NullString
	if err := db.QueryRow(
		"SELECT root_path FROM works WHERE rj_number='RJ404669'",
	).Scan(&rootPath); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if rootPath.Valid {
		t.Errorf("削除後の root_path = %q, want NULL", rootPath.String)
	}

	// フォルダを復活させる
	if err := os.MkdirAll(rjDir, 0o755); err != nil {
		t.Fatalf("フォルダ復活失敗: %v", err)
	}

	// 3回目: フォルダ復活後スキャン → LinkedToCSV=1(NULL → パスの再リンク)
	res3, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("3回目スキャン失敗: %v", err)
	}
	if res3.LinkedToCSV != 1 {
		t.Errorf("復活後 LinkedToCSV = %d, want 1", res3.LinkedToCSV)
	}
	if res3.NewlyRegistered != 0 {
		t.Errorf("復活後 NewlyRegistered = %d, want 0(既存 rj_number 行にリンク)", res3.NewlyRegistered)
	}

	// root_path が復活しているか
	if err := db.QueryRow(
		"SELECT root_path FROM works WHERE rj_number='RJ404669'",
	).Scan(&rootPath); err != nil {
		t.Fatalf("3回目後 SELECT 失敗: %v", err)
	}
	if !rootPath.Valid || rootPath.String != rjDir {
		t.Errorf("復活後 root_path = %v, want %q", rootPath, rjDir)
	}
}

// TestScanRJEdgeCases は RJ 番号の各種エッジケースをテスト。
// 各ケースは独立したライブラリに分けて RJ 番号の衝突を避ける。
func TestScanRJEdgeCases(t *testing.T) {
	cases := []struct {
		name         string
		dirName      string
		wantRJNumber string // "" は rj_number = NULL
		wantTitle    string
	}{
		{
			// RJ+5桁 → パターン不一致(RJ\d{6,8}) → フォルダ名全体がタイトル
			name:         "RJ5桁はパターン不一致でフォルダ名全体がタイトル",
			dirName:      "RJ12345_五桁作品",
			wantRJNumber: "",
			wantTitle:    "RJ12345_五桁作品",
		},
		{
			// RJ+6桁 → マッチ、タイトル抽出
			name:         "RJ6桁はマッチしタイトル抽出される",
			dirName:      "RJ223456_六桁作品",
			wantRJNumber: "RJ223456",
			wantTitle:    "六桁作品",
		},
		{
			// RJ+8桁 → マッチ、タイトル抽出
			name:         "RJ8桁はマッチしタイトル抽出される",
			dirName:      "RJ32345678_八桁作品",
			wantRJNumber: "RJ32345678",
			wantTitle:    "八桁作品",
		},
		{
			// "RJ123456"(アンダースコアなし) → rj_number=RJ123456, title=フォルダ名全体
			name:         "アンダースコアなしはフォルダ名全体がタイトル",
			dirName:      "RJ423456",
			wantRJNumber: "RJ423456",
			wantTitle:    "RJ423456",
		},
		{
			// "RJ1234567_"(アンダースコア末尾のみ → title="" → フォールバックでフォルダ名全体)
			name:         "末尾アンダースコアのみはフォルダ名全体がタイトル",
			dirName:      "RJ5234567_",
			wantRJNumber: "RJ5234567",
			wantTitle:    "RJ5234567_",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			base := t.TempDir()
			lib := filepath.Join(base, "library")

			dirPath := filepath.Join(lib, tc.dirName)
			if err := os.MkdirAll(dirPath, 0o755); err != nil {
				t.Fatal(err)
			}

			_, err := Scan(db, []string{lib}, nil)
			if err != nil {
				t.Fatalf("Scan 失敗: %v", err)
			}

			var title string
			var rjNumber sql.NullString
			absPath := filepath.Join(lib, tc.dirName)
			err = db.QueryRow(
				"SELECT title, rj_number FROM works WHERE root_path=?", absPath,
			).Scan(&title, &rjNumber)
			if err != nil {
				t.Fatalf("SELECT 失敗: %v", err)
			}

			if title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}

			if tc.wantRJNumber == "" {
				if rjNumber.Valid {
					t.Errorf("rj_number = %q, want NULL", rjNumber.String)
				}
			} else {
				if !rjNumber.Valid || rjNumber.String != tc.wantRJNumber {
					t.Errorf("rj_number = %v, want %q", rjNumber, tc.wantRJNumber)
				}
			}
		})
	}
}

// TestScanThumbnailUpdatedPath はサムネイル生成後に再スキャンしても
// 既存 thumbnail_path が上書きされることをテスト(冪等)。
func TestScanThumbnailUpdatedPath(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	gen := newFakeThumbGen()

	// 1回目スキャン: thumbnail_path が設定される
	_, err := Scan(db, []string{lib}, gen)
	if err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}

	// 1件の thumbnail_path を取得
	var id1 int64
	var tp1 sql.NullString
	if err := db.QueryRow("SELECT id, thumbnail_path FROM works LIMIT 1").Scan(&id1, &tp1); err != nil {
		t.Fatal(err)
	}
	if !tp1.Valid || tp1.String == "" {
		t.Fatal("1回目スキャン後に thumbnail_path が空")
	}

	// 2回目スキャン: 同じフェイク Generator を使う
	_, err = Scan(db, []string{lib}, gen)
	if err != nil {
		t.Fatalf("2回目スキャン失敗: %v", err)
	}

	// thumbnail_path は同じ値のまま
	var tp2 sql.NullString
	if err := db.QueryRow("SELECT thumbnail_path FROM works WHERE id=?", id1).Scan(&tp2); err != nil {
		t.Fatal(err)
	}
	if tp2.String != tp1.String {
		t.Errorf("2回目スキャン後 thumbnail_path = %q, want %q", tp2.String, tp1.String)
	}
}
