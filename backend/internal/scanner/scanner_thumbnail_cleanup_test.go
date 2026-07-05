package scanner

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestScanMissingPathClearsThumbnailAndCache は、フォルダが消えた work の
// thumbnail_path が NULL 化され、かつサムネイルのキャッシュファイル
// ({id}.jpg / {id}.src)も削除されることをテスト(PR #89 レビュー指摘)。
//
// 従来は root_path=NULL 化のみで thumbnail_path とキャッシュファイルが残っていたため、
// /api/works が thumbnail_path の存在だけを見て has_folder=false でも
// thumbnail_url を返し、削除済み作品の古いサムネイルが露出し続けていた。
func TestScanMissingPathClearsThumbnailAndCache(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	gen := newFakeThumbGen()

	// 1回目: 全フォルダをスキャンして thumbnail_path を設定する
	if _, err := Scan(db, []string{lib}, gen); err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}

	var workID int64
	var thumbPath sql.NullString
	if err := db.QueryRow(
		"SELECT id, thumbnail_path FROM works WHERE rj_number='RJ404669'",
	).Scan(&workID, &thumbPath); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if !thumbPath.Valid || thumbPath.String == "" {
		t.Fatalf("前提: thumbnail_path が設定されているはず, got %v", thumbPath)
	}

	// フォルダを削除
	rjDir := filepath.Join(lib, "RJ404669_耳舐め作品")
	if err := os.RemoveAll(rjDir); err != nil {
		t.Fatalf("フォルダ削除失敗: %v", err)
	}

	// 2回目: フォルダ消失を検知するスキャン
	scanRes, err := Scan(db, []string{lib}, gen)
	if err != nil {
		t.Fatalf("2回目スキャン失敗: %v", err)
	}
	if scanRes.MissingMarked != 1 {
		t.Errorf("MissingMarked = %d, want 1", scanRes.MissingMarked)
	}

	var rootPath, thumbPathAfter sql.NullString
	if err := db.QueryRow(
		"SELECT root_path, thumbnail_path FROM works WHERE id=?", workID,
	).Scan(&rootPath, &thumbPathAfter); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if rootPath.Valid {
		t.Errorf("root_path = %q, want NULL", rootPath.String)
	}
	if thumbPathAfter.Valid {
		t.Errorf("thumbnail_path = %q, want NULL(フォルダ消失時にクリアされるべき)", thumbPathAfter.String)
	}

	// キャッシュファイルの削除が要求されたことを確認(RemoveCache 呼び出し)
	gen.mu.Lock()
	removed := append([]int64{}, gen.removedCacheIDs...)
	gen.mu.Unlock()
	found := false
	for _, id := range removed {
		if id == workID {
			found = true
		}
	}
	if !found {
		t.Errorf("RemoveCache(%d) が呼ばれていない: removed=%v", workID, removed)
	}
}

// TestScanMissingPathNilThumbGenClearsDBOnly は thumbGen が nil の場合でも
// thumbnail_path の DB クリアだけは行われることをテスト。
func TestScanMissingPathNilThumbGenClearsDBOnly(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	// thumbnail_path を手動で設定しておく(nil thumbGen なので Scan からは設定されない)
	if _, err := Scan(db, []string{lib}, nil); err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}
	var workID int64
	if err := db.QueryRow("SELECT id FROM works WHERE rj_number='RJ404669'").Scan(&workID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE works SET thumbnail_path=? WHERE id=?", "/thumbs/1.jpg", workID); err != nil {
		t.Fatal(err)
	}

	rjDir := filepath.Join(lib, "RJ404669_耳舐め作品")
	if err := os.RemoveAll(rjDir); err != nil {
		t.Fatalf("フォルダ削除失敗: %v", err)
	}

	if _, err := Scan(db, []string{lib}, nil); err != nil {
		t.Fatalf("2回目スキャン失敗: %v", err)
	}

	var thumbPathAfter sql.NullString
	if err := db.QueryRow("SELECT thumbnail_path FROM works WHERE id=?", workID).Scan(&thumbPathAfter); err != nil {
		t.Fatal(err)
	}
	if thumbPathAfter.Valid {
		t.Errorf("thumbnail_path = %q, want NULL", thumbPathAfter.String)
	}
}
