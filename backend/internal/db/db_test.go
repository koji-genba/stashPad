package db

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestOpenCreatesFile は Open でファイルが作成されることをテスト。
func TestOpenCreatesFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open 失敗: %v", err)
	}
	defer db.Close()

	// ping で接続確認
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping 失敗: %v", err)
	}
}

// TestOpenCreatesAllTables は Open 後に migrations/*.sql のテーブルが全部作られていることをテスト。
func TestOpenCreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open 失敗: %v", err)
	}
	defer db.Close()

	// migrations ディレクトリから期待テーブルを収集する
	expectedTables := collectExpectedTables(t)

	// sqlite_master からテーブル一覧を取得
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("sqlite_master クエリ失敗: %v", err)
	}
	defer rows.Close()

	actualTables := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		actualTables[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, table := range expectedTables {
		if !actualTables[table] {
			t.Errorf("テーブル %q が存在しない", table)
		}
	}
}

// collectExpectedTables は migrations/*.sql を解析して CREATE TABLE 文のテーブル名を返す。
func collectExpectedTables(t *testing.T) []string {
	t.Helper()

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations ディレクトリ読み込み失敗: %v", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var tables []string
	seen := make(map[string]bool)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			t.Fatalf("SQL ファイル読み込み失敗 %s: %v", e.Name(), err)
		}

		// CREATE TABLE <name> または CREATE TABLE IF NOT EXISTS <name> を抽出
		content := string(data)
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			upper := strings.ToUpper(line)
			if !strings.HasPrefix(upper, "CREATE TABLE") {
				continue
			}
			// トークン分割: ["CREATE", "TABLE", ["IF", "NOT", "EXISTS",] name, ...]
			fields := strings.Fields(line)
			nameIdx := 2
			if len(fields) > 4 && strings.ToUpper(fields[2]) == "IF" {
				nameIdx = 5
			}
			if nameIdx >= len(fields) {
				continue
			}
			name := strings.TrimSuffix(fields[nameIdx], "(")
			if !seen[name] {
				seen[name] = true
				tables = append(tables, name)
			}
		}
	}
	return tables
}

// TestOpenRecordsMigrationVersions は schema_migrations に全バージョンが記録されることをテスト。
func TestOpenRecordsMigrationVersions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open 失敗: %v", err)
	}
	defer db.Close()

	// migrations の全ファイル名を収集
	entries, readErr := fs.ReadDir(migrationsFS, "migrations")
	if readErr != nil {
		t.Fatalf("migrations 読み込み失敗: %v", readErr)
	}
	var expectedVersions []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			expectedVersions = append(expectedVersions, e.Name())
		}
	}

	// schema_migrations のバージョン一覧を取得
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("schema_migrations クエリ失敗: %v", err)
	}
	defer rows.Close()

	appliedVersions := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		appliedVersions[v] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, v := range expectedVersions {
		if !appliedVersions[v] {
			t.Errorf("schema_migrations にバージョン %q が記録されていない", v)
		}
	}
}

// TestOpenIdempotent は同じパスで再 Open しても冪等であることをテスト。
func TestOpenIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("1回目 Open 失敗: %v", err)
	}
	db1.Close()

	// 2回目 Open: エラーにならず、schema_migrations が二重適用されないこと
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("2回目 Open 失敗: %v", err)
	}
	defer db2.Close()

	entries, readErr := fs.ReadDir(migrationsFS, "migrations")
	if readErr != nil {
		t.Fatalf("migrations 読み込み失敗: %v", readErr)
	}
	var sqlCount int
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqlCount++
		}
	}

	var count int
	if err := db2.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("schema_migrations 件数取得失敗: %v", err)
	}
	if count != sqlCount {
		t.Errorf("schema_migrations 件数 = %d, want %d(二重適用の疑い)", count, sqlCount)
	}
}

// TestOpenForeignKeysEnabled は foreign_keys PRAGMA が有効になっていることをテスト。
// work_tags に存在しない work_id で INSERT → FK 違反エラーを確認。
func TestOpenForeignKeysEnabled(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open 失敗: %v", err)
	}
	defer db.Close()

	// 存在しない work_id で work_tags に INSERT → FK 違反
	_, err = db.Exec(
		"INSERT INTO work_tags (work_id, tag_id) VALUES (99999, 99999)",
	)
	if err == nil {
		t.Error("存在しない work_id/tag_id の INSERT が成功した(foreign_keys が無効の可能性)")
	}
}

// TestOpenJournalModeWAL は journal_mode が WAL になっていることをテスト。
func TestOpenJournalModeWAL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open 失敗: %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode 取得失敗: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}
