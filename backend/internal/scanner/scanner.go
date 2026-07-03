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
	"runtime"
	"strings"
	"sync"
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
// サムネイル生成のみ worker pool(NumCPU)で並列化する。
func Scan(db *sql.DB, roots []string, thumbGen ThumbnailGenerator) (Result, error) {
	var res Result

	// ステップ1: 各ルートの直下ディレクトリを列挙し works を直列 upsert する。
	// サムネイル生成が必要な (workID, absPath) ペアを収集する。
	foundPaths := make(map[string]bool) // スキャンで見つかった root_path の集合
	var failedRoots []string            // ReadDir が失敗したルート(NAS 一時障害・未マウント等)

	type thumbJob struct {
		workID  int64
		absPath string
	}
	var thumbJobs []thumbJob

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			log.Printf("スキャン: ルート %q 読み込み失敗: %v", root, err)
			failedRoots = append(failedRoots, root)
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

			n, linked, workID, err := upsertWorkNoThumb(db, absPath, dirName)
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

			// サムネイル生成ジョブを積む
			if thumbGen != nil && workID > 0 {
				thumbJobs = append(thumbJobs, thumbJob{workID: workID, absPath: absPath})
			}
		}
	}

	// 全ルートが読み込み失敗した場合は NAS 未マウント等の一時障害とみなし、
	// DB に一切触れずに中断する(markMissingPaths による全件 NULL 化を防ぐ)。
	if len(failedRoots) == len(roots) {
		return Result{}, fmt.Errorf(
			"全ライブラリルート (%d 件) が読めないためスキャンを中断しました: マウント状態を確認してください",
			len(roots),
		)
	}

	// ステップ2: 既存 works の root_path が消えていたら NULL に戻す。
	// ただし読み込み失敗したルート配下は「消えた」と判定できないので対象外とする。
	missing, err := markMissingPaths(db, foundPaths, failedRoots)
	if err != nil {
		return res, fmt.Errorf("消失パスの処理に失敗: %w", err)
	}
	res.MissingMarked = missing

	// ステップ3: サムネイル生成を worker pool で並列実行
	if thumbGen != nil && len(thumbJobs) > 0 {
		type thumbResult struct {
			workID    int64
			thumbPath string
		}

		numWorkers := runtime.NumCPU()
		if numWorkers < 1 {
			numWorkers = 1
		}
		jobs := make(chan thumbJob, len(thumbJobs))
		results := make(chan thumbResult, len(thumbJobs))

		var wg sync.WaitGroup
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					thumbPath, tErr := thumbGen.Generate(j.workID, j.absPath)
					if tErr != nil {
						log.Printf("サムネイル生成失敗 work_id=%d: %v", j.workID, tErr)
						continue
					}
					if thumbPath != "" {
						results <- thumbResult{workID: j.workID, thumbPath: thumbPath}
					}
				}
			}()
		}

		for _, j := range thumbJobs {
			jobs <- j
		}
		close(jobs)

		// ワーカー完了後にチャネルを閉じる
		go func() {
			wg.Wait()
			close(results)
		}()

		// DB 更新は直列化(SQLite は同時書き込み非対応)
		for r := range results {
			if _, uErr := db.Exec(
				"UPDATE works SET thumbnail_path=?, updated_at=datetime('now') WHERE id=?",
				r.thumbPath, r.workID,
			); uErr != nil {
				log.Printf("thumbnail_path 更新失敗 work_id=%d: %v", r.workID, uErr)
			}
		}
	}

	return res, nil
}

// upsertWorkNoThumb は 1 つのフォルダを works テーブルに upsert する(サムネイル生成なし)。
// 返り値は (新規作成されたか, CSV 既存行に root_path がリンクされたか, 作成/更新された work の ID, エラー)。
func upsertWorkNoThumb(db *sql.DB, absPath, dirName string) (newlyRegistered bool, linkedToCSV bool, workID int64, err error) {
	m := rjPattern.FindStringSubmatch(dirName)

	if m != nil {
		// RJ 番号あり
		rjNumber := m[1]
		newlyRegistered, linkedToCSV, workID, err = upsertByRJ(db, rjNumber, absPath, dirName)
	} else {
		// RJ 番号なし → フォルダ名全体をタイトルとして登録
		newlyRegistered, workID, err = upsertByPath(db, absPath, dirName)
	}
	return
}

// upsertByRJ は RJ 番号でワークを upsert する。
// 既存の CSV 行(rj_number 一致、root_path が NULL)があればリンクする。
// 既存の root_path が同一であればスキップ(何も変えない)。
func upsertByRJ(db *sql.DB, rjNumber, absPath, dirName string) (newlyRegistered bool, linkedToCSV bool, workID int64, err error) {
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
			return false, false, 0, fmt.Errorf("works INSERT 失敗: %w", insErr)
		}
		newID, lErr := res.LastInsertId()
		if lErr != nil {
			return false, false, 0, fmt.Errorf("LastInsertId 取得失敗: %w", lErr)
		}
		return true, false, newID, nil
	}
	if scanErr != nil {
		return false, false, 0, fmt.Errorf("works SELECT 失敗: %w", scanErr)
	}

	// 既存行あり
	if currentRootPath.Valid && currentRootPath.String == absPath {
		// 同一パスなので何もしない
		return false, false, id, nil
	}

	// root_path を更新(NULL → パス の場合は「CSV 行にリンク」と判定)
	wasNull := !currentRootPath.Valid
	if _, uErr := db.Exec(
		"UPDATE works SET root_path=?, updated_at=datetime('now') WHERE id=?",
		absPath, id,
	); uErr != nil {
		return false, false, 0, fmt.Errorf("works UPDATE 失敗: %w", uErr)
	}
	return false, wasNull, id, nil
}

// upsertByPath は RJ 番号なしフォルダをフォルダ名全体をタイトルとして登録する。
// 既存判定は root_path の一致で行う。
// root_path 一致で見つからない場合、NAS 一時障害等で NULL 化された孤児行
// (root_path IS NULL かつ rj_number IS NULL かつ同一タイトル)への再リンクを試みてから
// INSERT する(issue #48: 重複行によるタグ・履歴の消失を防ぐ)。
func upsertByPath(db *sql.DB, absPath, dirName string) (newlyRegistered bool, workID int64, err error) {
	var id int64
	row := db.QueryRow("SELECT id FROM works WHERE root_path=?", absPath)
	scanErr := row.Scan(&id)

	if scanErr == sql.ErrNoRows {
		orphanID, found, findErr := findOrphanWorkByTitle(db, dirName)
		if findErr != nil {
			return false, 0, findErr
		}
		if found {
			if _, uErr := db.Exec(
				"UPDATE works SET root_path=?, updated_at=datetime('now') WHERE id=?",
				absPath, orphanID,
			); uErr != nil {
				return false, 0, fmt.Errorf("孤児行の再リンク UPDATE 失敗: %w", uErr)
			}
			return false, orphanID, nil
		}

		res, insErr := db.Exec(
			`INSERT INTO works (title, root_path, updated_at)
			 VALUES (?, ?, datetime('now'))`,
			dirName, absPath,
		)
		if insErr != nil {
			return false, 0, fmt.Errorf("works INSERT 失敗: %w", insErr)
		}
		newID, lErr := res.LastInsertId()
		if lErr != nil {
			return false, 0, fmt.Errorf("LastInsertId 取得失敗: %w", lErr)
		}
		return true, newID, nil
	}
	if scanErr != nil {
		return false, 0, fmt.Errorf("works SELECT 失敗: %w", scanErr)
	}
	// 既存 — 何もしない
	return false, id, nil
}

// findOrphanWorkByTitle は root_path・rj_number がともに NULL で、
// title が一致する行を 1 件探す。見つからなければ found=false を返す。
func findOrphanWorkByTitle(db *sql.DB, title string) (id int64, found bool, err error) {
	row := db.QueryRow(
		"SELECT id FROM works WHERE root_path IS NULL AND rj_number IS NULL AND title=? ORDER BY id LIMIT 1",
		title,
	)
	scanErr := row.Scan(&id)
	if scanErr == sql.ErrNoRows {
		return 0, false, nil
	}
	if scanErr != nil {
		return 0, false, fmt.Errorf("孤児行 SELECT 失敗: %w", scanErr)
	}
	return id, true, nil
}

// markMissingPaths は DB にある root_path のうち、foundPaths に含まれないものを NULL に戻す。
// ただし failedRoots 配下(読み込みに失敗したルート)にあるパスは、
// 「実際に消えた」のか「読めなかっただけ」なのか区別できないため対象から除外する。
// NULL 化した件数を返す。
func markMissingPaths(db *sql.DB, foundPaths map[string]bool, failedRoots []string) (int, error) {
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
		if !foundPaths[r.path] && !underFailedRoot(r.path, failedRoots) {
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

// underFailedRoot は path がいずれかの failedRoots 配下(またはルート自身)にあるかを判定する。
// セパレータ境界を考慮し、"/lib" と "/lib2" のような部分一致誤爆を防ぐ。
func underFailedRoot(path string, failedRoots []string) bool {
	for _, root := range failedRoots {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
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
