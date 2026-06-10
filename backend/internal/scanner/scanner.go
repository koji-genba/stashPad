// Package scanner はライブラリルートのスキャンを担当する。
// 各ルートの直下ディレクトリだけを列挙し、RJ 番号を抽出して works テーブルに upsert する。
// サムネイル生成は thumb パッケージに委譲する。
package scanner

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
)

// rjPattern は "RJ" に続く 6〜8 桁の数字にマッチする。
var rjPattern = regexp.MustCompile(`^(RJ\d{6,8})`)

// Result はスキャン結果のサマリ。POST /api/scan のレスポンスに使う。
type Result struct {
	WorksFound      int `json:"works_found"`
	NewlyRegistered int `json:"newly_registered"`
	LinkedToCSV     int `json:"linked_to_csv"`
	MissingMarked   int `json:"missing_marked"`
}

// ThumbnailGenerator はサムネイル生成の依存を抽象化するインターフェース。
// thumb.Generator が実装する。
type ThumbnailGenerator interface {
	Generate(workID int64, rootPath string) (string, error)
}

// Scan はすべてのライブラリルートをスキャンしてワークを upsert し、
// 消えたパスの root_path を NULL に戻す。
// thumbGen が nil の場合はサムネイル生成をスキップする。
func Scan(db *sql.DB, roots []string, thumbGen ThumbnailGenerator) (Result, error) {
	var res Result

	// ステップ1: 各ルートの直下ディレクトリを列挙してスキャン
	foundPaths := make(map[string]bool) // スキャンで見つかった root_path の集合

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			log.Printf("スキャン: ルート %q 読み込み失敗: %v", root, err)
			continue
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dirName := e.Name()
			absPath := filepath.Join(root, dirName)
			foundPaths[absPath] = true
			res.WorksFound++

			n, linked, err := upsertWork(db, absPath, dirName, thumbGen)
			if err != nil {
				log.Printf("スキャン: upsert 失敗 %q: %v", absPath, err)
				continue
			}
			if n {
				res.NewlyRegistered++
			}
			if linked {
				res.LinkedToCSV++
			}
		}
	}

	// ステップ2: 既存 works の root_path が消えていたら NULL に戻す
	missing, err := markMissingPaths(db, foundPaths)
	if err != nil {
		return res, fmt.Errorf("消失パスの処理に失敗: %w", err)
	}
	res.MissingMarked = missing

	return res, nil
}

// upsertWork は 1 つのフォルダを works テーブルに upsert する。
// 返り値は (新規作成されたか, CSV 既存行に root_path がリンクされたか, エラー)。
func upsertWork(db *sql.DB, absPath, dirName string, thumbGen ThumbnailGenerator) (newlyRegistered bool, linkedToCSV bool, err error) {
	m := rjPattern.FindStringSubmatch(dirName)

	if m != nil {
		// RJ 番号あり
		rjNumber := m[1]
		newlyRegistered, linkedToCSV, err = upsertByRJ(db, rjNumber, absPath, dirName)
	} else {
		// RJ 番号なし → フォルダ名全体をタイトルとして登録
		newlyRegistered, err = upsertByPath(db, absPath, dirName)
	}
	if err != nil {
		return
	}

	// サムネイル生成
	if thumbGen != nil {
		workID, qErr := getWorkIDByPath(db, absPath)
		if qErr == nil && workID > 0 {
			thumbPath, tErr := thumbGen.Generate(workID, absPath)
			if tErr != nil {
				log.Printf("サムネイル生成失敗 work_id=%d: %v", workID, tErr)
			} else if thumbPath != "" {
				if _, uErr := db.Exec(
					"UPDATE works SET thumbnail_path=?, updated_at=datetime('now') WHERE id=?",
					thumbPath, workID,
				); uErr != nil {
					log.Printf("thumbnail_path 更新失敗 work_id=%d: %v", workID, uErr)
				}
			}
		}
	}

	return
}

// upsertByRJ は RJ 番号でワークを upsert する。
// 既存の CSV 行(rj_number 一致、root_path が NULL)があればリンクする。
// 既存の root_path が同一であればスキップ(何も変えない)。
func upsertByRJ(db *sql.DB, rjNumber, absPath, dirName string) (newlyRegistered bool, linkedToCSV bool, err error) {
	// タイトル候補: "RJxxxxxx_" 以降の文字列。アンダースコアがなければフォルダ名全体
	title := dirName
	if idx := firstUnderscoreAfterRJ(dirName); idx >= 0 {
		title = dirName[idx+1:]
	}
	if title == "" {
		title = dirName
	}

	// 既存チェック
	var id int64
	var currentRootPath sql.NullString
	var currentTitle string
	row := db.QueryRow(
		"SELECT id, root_path, title FROM works WHERE rj_number=?",
		rjNumber,
	)
	scanErr := row.Scan(&id, &currentRootPath, &currentTitle)

	if scanErr == sql.ErrNoRows {
		// 新規作成
		res, insErr := db.Exec(
			`INSERT INTO works (rj_number, title, root_path, updated_at)
			 VALUES (?, ?, ?, datetime('now'))`,
			rjNumber, title, absPath,
		)
		if insErr != nil {
			return false, false, fmt.Errorf("works INSERT 失敗: %w", insErr)
		}
		_ = res
		return true, false, nil
	}
	if scanErr != nil {
		return false, false, fmt.Errorf("works SELECT 失敗: %w", scanErr)
	}

	// 既存行あり
	if currentRootPath.Valid && currentRootPath.String == absPath {
		// 同一パスなので何もしない
		return false, false, nil
	}

	// root_path を更新(NULL → パス の場合は「CSV 行にリンク」と判定)
	wasNull := !currentRootPath.Valid
	if _, uErr := db.Exec(
		"UPDATE works SET root_path=?, updated_at=datetime('now') WHERE id=?",
		absPath, id,
	); uErr != nil {
		return false, false, fmt.Errorf("works UPDATE 失敗: %w", uErr)
	}
	return false, wasNull, nil
}

// upsertByPath は RJ 番号なしフォルダをフォルダ名全体をタイトルとして登録する。
// 既存判定は root_path の一致で行う。
func upsertByPath(db *sql.DB, absPath, dirName string) (newlyRegistered bool, err error) {
	var id int64
	row := db.QueryRow("SELECT id FROM works WHERE root_path=?", absPath)
	scanErr := row.Scan(&id)

	if scanErr == sql.ErrNoRows {
		_, insErr := db.Exec(
			`INSERT INTO works (title, root_path, updated_at)
			 VALUES (?, ?, datetime('now'))`,
			dirName, absPath,
		)
		if insErr != nil {
			return false, fmt.Errorf("works INSERT 失敗: %w", insErr)
		}
		return true, nil
	}
	if scanErr != nil {
		return false, fmt.Errorf("works SELECT 失敗: %w", scanErr)
	}
	// 既存 — 何もしない
	return false, nil
}

// markMissingPaths は DB にある root_path のうち、foundPaths に含まれないものを NULL に戻す。
// NULL 化した件数を返す。
func markMissingPaths(db *sql.DB, foundPaths map[string]bool) (int, error) {
	rows, err := db.Query("SELECT id, root_path FROM works WHERE root_path IS NOT NULL")
	if err != nil {
		return 0, fmt.Errorf("root_path 一覧取得失敗: %w", err)
	}
	defer rows.Close()

	type row struct {
		id   int64
		path string
	}
	var toNull []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.path); err != nil {
			return 0, err
		}
		if !foundPaths[r.path] {
			toNull = append(toNull, r)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, r := range toNull {
		if _, err := db.Exec(
			"UPDATE works SET root_path=NULL, updated_at=datetime('now') WHERE id=?",
			r.id,
		); err != nil {
			log.Printf("root_path NULL 化失敗 id=%d: %v", r.id, err)
		}
	}
	return len(toNull), nil
}

// getWorkIDByPath は root_path から work の id を取得する。
func getWorkIDByPath(db *sql.DB, absPath string) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM works WHERE root_path=?", absPath).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// firstUnderscoreAfterRJ は "RJxxxxxx_" の最初の "_" のインデックスを返す。
// 見つからなければ -1。
func firstUnderscoreAfterRJ(name string) int {
	for i, c := range name {
		if c == '_' {
			return i
		}
	}
	return -1
}
