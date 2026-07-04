package scanner

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// benchSchema は works テーブルのみを持つ最小スキーマ。
// Scan / upsert / markMissingPaths が参照するのは works テーブルのみなので、
// ベンチマークではこれで十分。
const benchSchema = `
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
`

// openBenchFileDB はベンチマーク用にファイルベース SQLite(WAL)を開く。
// :memory: では fsync が発生せず、今回のトランザクションバッチ化による
// I/O 削減効果を計測できないため、t.TempDir 配下の実ファイルを使う。
// DSN は internal/db.Open と同じ PRAGMA(WAL・busy_timeout)を使い、
// 本番相当の書き込みコストを再現する。
func openBenchFileDB(b *testing.B, dbPath string) *sql.DB {
	b.Helper()
	dsn := "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		b.Fatalf("DB オープン失敗: %v", err)
	}
	if _, err := db.Exec(benchSchema); err != nil {
		b.Fatalf("スキーマ適用失敗: %v", err)
	}
	return db
}

// setupBenchLibrary は n 件の RJ フォルダを持つライブラリディレクトリを作る。
func setupBenchLibrary(b *testing.B, n int) string {
	b.Helper()
	base := b.TempDir()
	lib := filepath.Join(base, "library")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < n; i++ {
		dirName := fmt.Sprintf("RJ%06d_ベンチ作品%d", 100000+i, i)
		if err := os.MkdirAll(filepath.Join(lib, dirName), 0o755); err != nil {
			b.Fatal(err)
		}
	}
	return lib
}

// BenchmarkScanUpsert は 500 件の新規作品を含むライブラリを Scan する時間を計測する。
// 反復ごとに新規のファイルベース DB を用意し、「初回スキャン(全件 INSERT)」の
// コストを毎回同条件で測る(2 回目以降の Scan は idempotent で UPDATE 判定のみに
// なり計測意図がずれるため)。DB オープン/スキーマ適用は計測対象外にする。
func BenchmarkScanUpsert(b *testing.B) {
	const workCount = 500
	lib := setupBenchLibrary(b, workCount)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dbPath := filepath.Join(b.TempDir(), fmt.Sprintf("bench-%d.db", i))
		db := openBenchFileDB(b, dbPath)
		b.StartTimer()

		if _, err := Scan(db, []string{lib}, nil); err != nil {
			b.Fatalf("Scan 失敗: %v", err)
		}

		b.StopTimer()
		db.Close()
	}
}
