package scanner

import (
	"errors"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestScanPartialRootFailurePreservesMissingRootWorks は、
// 複数ルートのうち一部が読み込み失敗した場合に、
// 失敗したルート配下の既存 work の root_path が NULL 化されないことをテスト(issue #48)。
func TestScanPartialRootFailurePreservesMissingRootWorks(t *testing.T) {
	db := openTestDB(t)
	libA := setupLibrary(t)

	base := t.TempDir()
	libB := filepath.Join(base, "libB_missing_mount")

	// libB は存在しないが、事前に libB 配下の work が DB に登録されている状態を再現する
	// (前回のスキャンで正常にマウントされ登録された想定)。
	preexistingPath := filepath.Join(libB, "RJ999999_マウント前作品")
	res, err := db.Exec(
		`INSERT INTO works (rj_number, title, root_path, updated_at) VALUES (?, ?, ?, datetime('now'))`,
		"RJ999999", "マウント前作品", preexistingPath,
	)
	if err != nil {
		t.Fatalf("事前 INSERT 失敗: %v", err)
	}
	preexistingID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	scanRes, err := Scan(db, []string{libA, libB}, nil)
	if err != nil {
		t.Fatalf("Scan は libA が読めるのでエラーを返してはならない: %v", err)
	}

	// libB は読めなかったので MissingMarked には数えられない
	if scanRes.MissingMarked != 0 {
		t.Errorf("MissingMarked = %d, want 0 (libB 配下は読み込み失敗のため対象外)", scanRes.MissingMarked)
	}

	var rootPath sql.NullString
	if err := db.QueryRow("SELECT root_path FROM works WHERE id=?", preexistingID).Scan(&rootPath); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if !rootPath.Valid || rootPath.String != preexistingPath {
		t.Errorf("root_path = %v, want %q (読み込み失敗ルート配下は NULL 化されてはならない)", rootPath, preexistingPath)
	}
}

// TestScanPartialRootFailureStillMarksMissingInReadableRoot は、
// 一部ルートが読み込み失敗していても、正常に読めたルート配下で
// 実際にフォルダが消えた work は従来どおり NULL 化されることをテスト(リグレッションガード)。
func TestScanPartialRootFailureStillMarksMissingInReadableRoot(t *testing.T) {
	db := openTestDB(t)
	libA := setupLibrary(t)

	base := t.TempDir()
	libB := filepath.Join(base, "libB_missing_mount")

	// 1回目: libA のみで正常スキャンして RJ404669 を登録する
	if _, err := Scan(db, []string{libA}, nil); err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}

	// libA から RJ404669 のフォルダを削除
	rjDir := filepath.Join(libA, "RJ404669_耳舐め作品")
	if err := os.RemoveAll(rjDir); err != nil {
		t.Fatalf("フォルダ削除失敗: %v", err)
	}

	// 2回目: libA(読める) + libB(存在しない) でスキャン
	scanRes, err := Scan(db, []string{libA, libB}, nil)
	if err != nil {
		t.Fatalf("Scan は libA が読めるのでエラーを返してはならない: %v", err)
	}

	if scanRes.MissingMarked != 1 {
		t.Errorf("MissingMarked = %d, want 1 (libA 配下で実際に消えた work は NULL 化されるべき)", scanRes.MissingMarked)
	}

	var rootPath sql.NullString
	if err := db.QueryRow("SELECT root_path FROM works WHERE rj_number='RJ404669'").Scan(&rootPath); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if rootPath.Valid {
		t.Errorf("root_path = %q, want NULL", rootPath.String)
	}
}

// TestScanAllRootsFailReturnsErrorWithoutTouchingDB は、
// 全ルートが読み込み失敗した場合に Scan がエラーを返し、
// DB の root_path が一切変更されないことをテスト(issue #48)。
func TestScanAllRootsFailReturnsErrorWithoutTouchingDB(t *testing.T) {
	db := openTestDB(t)

	base := t.TempDir()
	libA := filepath.Join(base, "libA_missing")
	libB := filepath.Join(base, "libB_missing")

	preexistingPath := filepath.Join(libA, "RJ999999_作品")
	res, err := db.Exec(
		`INSERT INTO works (rj_number, title, root_path, updated_at) VALUES (?, ?, ?, datetime('now'))`,
		"RJ999999", "作品", preexistingPath,
	)
	if err != nil {
		t.Fatalf("事前 INSERT 失敗: %v", err)
	}
	preexistingID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	_, err = Scan(db, []string{libA, libB}, nil)
	if err == nil {
		t.Fatal("全ルートが読めない場合 Scan はエラーを返すべき")
	}
	if !errors.Is(err, ErrAllRootsUnreadable) {
		t.Errorf("err = %v, want ErrAllRootsUnreadable を含む(API 側が識別して対処メッセージを返すため)", err)
	}

	var rootPath sql.NullString
	if err := db.QueryRow("SELECT root_path FROM works WHERE id=?", preexistingID).Scan(&rootPath); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if !rootPath.Valid || rootPath.String != preexistingPath {
		t.Errorf("root_path = %v, want %q (全ルート失敗時は DB を一切変更してはならない)", rootPath, preexistingPath)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM works").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("works 件数 = %d, want 1 (INSERT が行われてはならない)", count)
	}
}

// TestScanFailedRootPrefixBoundary は failedRoots のプレフィックス判定が
// セパレータ境界を考慮しており、"/…/lib" と "/…/lib2" のような部分一致で
// 誤爆しないことをテスト(issue #48)。
func TestScanFailedRootPrefixBoundary(t *testing.T) {
	db := openTestDB(t)

	base := t.TempDir()
	lib := filepath.Join(base, "lib")
	lib2 := filepath.Join(base, "lib2")

	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	// lib2 は存在しないルート(読み込み失敗)

	rjDir := filepath.Join(lib, "RJ404669_耳舐め作品")
	if err := os.MkdirAll(rjDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1回目: lib のみでスキャンして登録
	if _, err := Scan(db, []string{lib}, nil); err != nil {
		t.Fatalf("1回目スキャン失敗: %v", err)
	}

	// lib からフォルダを削除(実際に消えたケース)
	if err := os.RemoveAll(rjDir); err != nil {
		t.Fatalf("フォルダ削除失敗: %v", err)
	}

	// 2回目: lib(読める、フォルダ消失) + lib2(読み込み失敗) でスキャン
	// lib2 が failedRoots に入っていても、"lib" 配下の消失は
	// プレフィックス誤爆によって除外されてはならない。
	scanRes, err := Scan(db, []string{lib, lib2}, nil)
	if err != nil {
		t.Fatalf("Scan は lib が読めるのでエラーを返してはならない: %v", err)
	}

	if scanRes.MissingMarked != 1 {
		t.Errorf("MissingMarked = %d, want 1 (lib2 は lib のプレフィックスではないので誤爆してはならない)", scanRes.MissingMarked)
	}

	var rootPath sql.NullString
	if err := db.QueryRow("SELECT root_path FROM works WHERE rj_number='RJ404669'").Scan(&rootPath); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if rootPath.Valid {
		t.Errorf("root_path = %q, want NULL", rootPath.String)
	}
}

// TestUpsertByPathRelinksOrphanedNonRJWork は、RJ 番号なしフォルダの再スキャンで、
// root_path=NULL・rj_number=NULL の孤児行が同名タイトルであれば
// 新規 INSERT ではなく再リンクされることをテスト(issue #48)。
func TestUpsertByPathRelinksOrphanedNonRJWork(t *testing.T) {
	db := openTestDB(t)
	lib := setupLibrary(t)

	// 孤児行を仕込む: 非RJフォルダと同じタイトルで root_path=NULL, rj_number=NULL
	res, err := db.Exec(
		`INSERT INTO works (title, root_path, updated_at) VALUES (?, NULL, datetime('now'))`,
		"非RJフォルダ",
	)
	if err != nil {
		t.Fatalf("事前 INSERT 失敗: %v", err)
	}
	orphanID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	var countBefore int
	if err := db.QueryRow("SELECT COUNT(*) FROM works").Scan(&countBefore); err != nil {
		t.Fatal(err)
	}

	scanRes, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}

	var countAfter int
	if err := db.QueryRow("SELECT COUNT(*) FROM works").Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	// setupLibrary には RJ404669, RJ304928, 非RJフォルダ の 3 フォルダがある。
	// 非RJフォルダは孤児行への再リンクなので新規行は RJ 2件分だけ増える想定。
	wantCountAfter := countBefore + 2
	if countAfter != wantCountAfter {
		t.Errorf("works 件数 = %d → %d, want %d (非RJフォルダは再リンクされ新規行を作らないはず)", countBefore, countAfter, wantCountAfter)
	}

	// 非RJフォルダの work は孤児行と同じ ID を持つべき(再リンク)
	var relinkedID int64
	var rootPath sql.NullString
	expectedPath := filepath.Join(lib, "非RJフォルダ")
	if err := db.QueryRow(
		"SELECT id, root_path FROM works WHERE title=? AND root_path=?", "非RJフォルダ", expectedPath,
	).Scan(&relinkedID, &rootPath); err != nil {
		t.Fatalf("再リンクされた行の SELECT 失敗: %v", err)
	}
	if relinkedID != orphanID {
		t.Errorf("再リンクされた work の id = %d, want %d (孤児行の id を保持すべき)", relinkedID, orphanID)
	}
	if !rootPath.Valid || rootPath.String != expectedPath {
		t.Errorf("root_path = %v, want %q", rootPath, expectedPath)
	}

	// newly_registered としてカウントされてはならない
	// (setupLibrary には RJ404669, RJ304928, 非RJフォルダ の 3 件があり、
	//  非RJフォルダは再リンクなので新規は RJ 2件のみ)
	if scanRes.NewlyRegistered != 2 {
		t.Errorf("NewlyRegistered = %d, want 2 (非RJフォルダは再リンクなので新規カウントされない)", scanRes.NewlyRegistered)
	}
}
