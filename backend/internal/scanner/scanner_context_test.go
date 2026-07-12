package scanner

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

// TestScanContext_CanceledBeforeMarkMissing は、キャンセル済み ctx で
// ScanContext を呼んだ場合に markMissingPaths が実行されないことをテストする
// (issue #83)。途中までしか見ていない foundPaths で消失判定すると、
// 未走査の作品の root_path を誤って NULL 化してしまうため、キャンセル時は
// markMissingPaths を絶対に実行してはならない。
func TestScanContext_CanceledBeforeMarkMissing(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	// 事前に、フォルダが存在しない work を用意しておく。
	// 通常スキャン(非キャンセル)であれば markMissingPaths でこの root_path は
	// NULL 化されるはずの状態。
	missingPath := filepath.Join(lib, "RJ999999_消えた作品")
	insRes, err := db.Exec(
		`INSERT INTO works (rj_number, title, root_path, updated_at) VALUES (?, ?, ?, datetime('now'))`,
		"RJ999999", "消えた作品", missingPath,
	)
	if err != nil {
		t.Fatalf("事前 INSERT 失敗: %v", err)
	}
	workID, err := insRes.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 呼び出し前からキャンセル済みにしておく

	_, err = ScanContext(ctx, db, []string{lib}, nil)
	if err == nil {
		t.Fatal("キャンセル済み ctx では ScanContext はエラーを返すべき")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled を含む", err)
	}

	var rootPath sql.NullString
	if err := db.QueryRow("SELECT root_path FROM works WHERE id=?", workID).Scan(&rootPath); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if !rootPath.Valid || rootPath.String != missingPath {
		t.Errorf("root_path = %v, want %q (キャンセル時は markMissingPaths を実行してはならない)", rootPath, missingPath)
	}
}
