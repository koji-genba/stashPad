// Package db は SQLite の接続管理とマイグレーションを担当する。
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // SQLite ドライバ登録
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open は SQLite データベースを開き、PRAGMA を設定してマイグレーションを適用する。
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("DB オープン失敗: %w", err)
	}

	// SQLite の同時書き込みはほぼないが接続ごとに PRAGMA を設定する
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("WAL 設定失敗: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("foreign_keys 設定失敗: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("マイグレーション失敗: %w", err)
	}

	return db, nil
}

// migrate は schema_migrations テーブルを管理し、未適用の SQL ファイルを番号順に実行する。
func migrate(db *sql.DB) error {
	// schema_migrations テーブルを作成(初回のみ)
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("schema_migrations 作成失敗: %w", err)
	}

	// 適用済みバージョンを取得
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("適用済みマイグレーション取得失敗: %w", err)
	}
	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// migrations ディレクトリの SQL ファイルを番号順で列挙
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrations ディレクトリ読み込み失敗: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version := e.Name()
		if applied[version] {
			continue
		}

		data, err := migrationsFS.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("SQL ファイル読み込み失敗 %s: %w", version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("トランザクション開始失敗: %w", err)
		}
		if _, err := tx.Exec(string(data)); err != nil {
			tx.Rollback()
			return fmt.Errorf("マイグレーション適用失敗 %s: %w", version, err)
		}
		if _, err := tx.Exec(
			"INSERT INTO schema_migrations(version) VALUES(?)", version,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("マイグレーション記録失敗 %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("マイグレーションコミット失敗 %s: %w", version, err)
		}
	}

	return nil
}
