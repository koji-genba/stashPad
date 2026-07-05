package scanner

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDBWithBadTitleTrigger は openTestDB と同じスキーマに加えて、
// title が badTitle と一致する行の INSERT を拒否する BEFORE INSERT トリガーを持つ
// DB を返す。「不正データ」による 1 件の upsert 失敗を再現するためのテスト専用ヘルパー。
//
// RAISE(ABORT, ...) を使うのが重要: SQLite の制約違反時の既定動作(ON CONFLICT ABORT)
// と同じく、失敗するのは違反した文だけでトランザクション自体は生きたままになる
// (RAISE(ROLLBACK, ...) だとトランザクション全体を巻き込んでしまうため使わない)。
func openTestDBWithBadTitleTrigger(t *testing.T, badTitle string) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	stmt := fmt.Sprintf(`
		CREATE TRIGGER reject_bad_title BEFORE INSERT ON works
		WHEN NEW.title = '%s'
		BEGIN
			SELECT RAISE(ABORT, 'テスト用: 不正データによる INSERT 拒否');
		END;
	`, badTitle)
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("トリガー作成失敗: %v", err)
	}
	return db
}

// TestScanChunkOneFailureDoesNotLoseOthers は、同一チャンク(同一トランザクション)
// 内で 1 件の upsert が不正データにより失敗しても、トランザクション全体が失われず
// 他の作品は正常にコミットされることをテストする(issue #26)。
//
// SQLite は 1 文のエラーでトランザクションを自動 abort しないため、
// 「ログして continue、最後に Commit」という実装方針が正しく機能していることの
// リグレッションガードになる。もし誤って失敗時に tx.Rollback() してしまう実装に
// 変わってしまうと、このテストは(以降の Exec が ErrTxDone で失敗するため)
// 正常作品も失われて Red になる。
func TestScanChunkOneFailureDoesNotLoseOthers(t *testing.T) {
	const badTitle = "不正データ"
	db := openTestDBWithBadTitleTrigger(t, badTitle)

	base := t.TempDir()
	lib := filepath.Join(base, "library")
	dirs := []string{
		"RJ111111_正常作品A",
		"RJ222222_" + badTitle, // このフォルダだけ INSERT が拒否される
		"RJ333333_正常作品B",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(lib, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Scan(db, []string{lib}, nil)
	if err != nil {
		t.Fatalf("Scan は個別 upsert 失敗があってもエラーを返してはならない: %v", err)
	}

	// 3 フォルダ中 2 件だけ新規登録される(不正データの 1 件は失敗)
	if res.WorksFound != 3 {
		t.Errorf("WorksFound = %d, want 3", res.WorksFound)
	}
	if res.NewlyRegistered != 2 {
		t.Errorf("NewlyRegistered = %d, want 2 (不正データの 1 件は失敗するはず)", res.NewlyRegistered)
	}

	// 正常な 2 件は同じトランザクション内でコミットされ、DB に残っているべき
	var countGood int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM works WHERE rj_number IN ('RJ111111','RJ333333')",
	).Scan(&countGood); err != nil {
		t.Fatal(err)
	}
	if countGood != 2 {
		t.Errorf("正常な 2 件のうち DB に残っている件数 = %d, want 2 (同一トランザクション内の他作品が失われている)", countGood)
	}

	// 不正データの行は INSERT が拒否されているため存在しないはず
	var countBad int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM works WHERE rj_number='RJ222222'",
	).Scan(&countBad); err != nil {
		t.Fatal(err)
	}
	if countBad != 0 {
		t.Errorf("不正データの行数 = %d, want 0 (トリガーで INSERT が拒否されるはず)", countBad)
	}
}
