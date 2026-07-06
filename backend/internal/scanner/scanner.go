// Package scanner はライブラリルートのスキャンを担当する。
// 各ルートの直下ディレクトリだけを列挙し、RJ 番号を抽出して works テーブルに upsert する。
// サムネイル生成は thumb パッケージに委譲する。
package scanner

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/koji-genba/stashpad/backend/internal/media"
)

// ErrAllRootsUnreadable は全ライブラリルートが読めずスキャンを中断したことを示す。
// NAS 未マウント等の一時障害が典型で、API 層はこれを識別してユーザーに対処を促す
// メッセージを返す(内部データを含まないためそのまま表示してよい)。
var ErrAllRootsUnreadable = errors.New("全ライブラリルートが読めないためスキャンを中断しました")

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
	Refresh(workID int64, rootPath string) (regenerated bool, outPath string, candidateFound bool, err error)

	// RemoveCache は workID に対応するサムネイルキャッシュファイル
	// ({workID}.jpg / {workID}.src)を削除する。フォルダ消失で work の
	// root_path が NULL 化された際に、古いサムネイルを露出させないために呼ばれる。
	RemoveCache(workID int64) error
}

// execQuerier は upsert 系ヘルパーが必要とする最小限のインターフェース。
// *sql.DB と *sql.Tx はどちらもこれを満たすため、Scan からは *sql.Tx を渡して
// バッチをトランザクションにまとめられる一方、既存テストのように *sql.DB を
// 直接渡すことも変わらず可能。
type execQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// upsertChunkSize は upsert ループを何件ごとにコミットするかの単位。
// ルート全体を 1 トランザクションにすると巨大ライブラリでロールバック/失敗時の
// コストが大きくなるため、件数で区切って BEGIN...COMMIT を繰り返す。
const upsertChunkSize = 500

// Scan はすべてのライブラリルートをスキャンしてワークを upsert し、
// 消えたパスの root_path を NULL に戻す。
// thumbGen が nil の場合はサムネイル生成をスキップする。
// サムネイル生成のみ worker pool(NumCPU)で並列化する。
func Scan(db *sql.DB, roots []string, thumbGen ThumbnailGenerator) (Result, error) {
	var res Result

	// roots を filepath.Clean で正規化する。config.Load 側でも Clean するが、
	// Scan は公開 API であり呼び出し元の正規化に依存すべきではない。末尾スラッシュ
	// 付きルート(例: "/mnt/nas/libB/")が生文字列のまま failedRoots に入ると、
	// DB の root_path(filepath.Join で Clean 済み)との比較(underFailedRoot の
	// HasPrefix(path, root+"/"))が二重スラッシュ不一致で false になり、部分障害時に
	// markMissingPaths が配下を誤って NULL 化してしまう(PR #79 レビュー指摘)。
	cleanedRoots := make([]string, len(roots))
	for i, r := range roots {
		cleanedRoots[i] = filepath.Clean(r)
	}
	roots = cleanedRoots

	// ステップ1: 各ルートの直下ディレクトリを列挙し works を直列 upsert する。
	// サムネイル生成が必要な (workID, absPath) ペアを収集する。
	foundPaths := make(map[string]bool) // スキャンで見つかった root_path の集合
	var failedRoots []string            // ReadDir が失敗したルート(NAS 一時障害・未マウント等)

	type thumbJob struct {
		workID  int64
		absPath string
	}
	var thumbJobs []thumbJob

	// upsert ループ全体を(500 件チャンク単位の)トランザクションでまとめ、
	// 作品ごとの db.Exec による fsync を減らす。1 件の upsert 失敗で
	// トランザクション全体を失いたくないため、SQLite が自動 abort しない
	// ことを利用し「ログして継続」した上で最後に Commit する。
	tx, err := db.Begin()
	if err != nil {
		return Result{}, fmt.Errorf("スキャン用トランザクション開始失敗: %w", err)
	}
	// 正常系では明示的に Commit する。Commit 済みの tx に対する Rollback は
	// 何もせずエラーを返すだけなので、パニックや早期 return からの保護として
	// 無条件に defer しておいて問題ない。
	defer func() { _ = tx.Rollback() }()

	chunkCount := 0
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
			if media.IsHiddenName(dirName) {
				continue
			}
			absPath := filepath.Join(root, dirName)
			foundPaths[absPath] = true
			res.WorksFound++

			n, linked, workID, uErr := upsertWorkNoThumb(tx, absPath, dirName)
			if uErr != nil {
				log.Printf("スキャン: upsert 失敗 %q: %v", absPath, uErr)
			} else {
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

			chunkCount++
			if chunkCount >= upsertChunkSize {
				if cErr := tx.Commit(); cErr != nil {
					return res, fmt.Errorf("upsert トランザクションのコミット失敗: %w", cErr)
				}
				newTx, bErr := db.Begin()
				if bErr != nil {
					return res, fmt.Errorf("upsert トランザクション再開失敗: %w", bErr)
				}
				tx = newTx
				chunkCount = 0
			}
		}
	}

	if cErr := tx.Commit(); cErr != nil {
		return res, fmt.Errorf("upsert トランザクションのコミット失敗: %w", cErr)
	}

	// 全ルートが読み込み失敗した場合は NAS 未マウント等の一時障害とみなし、
	// DB に一切触れずに中断する(markMissingPaths による全件 NULL 化を防ぐ)。
	if len(failedRoots) == len(roots) {
		return Result{}, fmt.Errorf(
			"%w (%d 件): マウント状態を確認してください", ErrAllRootsUnreadable, len(roots),
		)
	}

	// ステップ2: 既存 works の root_path が消えていたら NULL に戻す。
	// ただし読み込み失敗したルート配下は「消えた」と判定できないので対象外とする。
	// thumbnail_path も同時に NULL 化する(残すと /api/works が has_folder=false でも
	// 古いサムネイルの thumbnail_url を返してしまう。PR #89 レビュー指摘)。
	missingIDs, err := markMissingPaths(db, foundPaths, failedRoots)
	if err != nil {
		return res, fmt.Errorf("消失パスの処理に失敗: %w", err)
	}
	res.MissingMarked = len(missingIDs)

	// サムネイルのキャッシュファイル({id}.jpg / {id}.src)も削除する。
	// DB クリアと別経路のファイル削除なので、失敗してもスキャン全体は継続しログのみ残す。
	if thumbGen != nil {
		for _, id := range missingIDs {
			if rErr := thumbGen.RemoveCache(id); rErr != nil {
				log.Printf("サムネイルキャッシュ削除失敗 work_id=%d: %v", id, rErr)
			}
		}
	}

	// ステップ3: サムネイル生成を worker pool で並列実行
	if thumbGen != nil && len(thumbJobs) > 0 {
		type thumbResult struct {
			workID         int64
			thumbPath      string
			candidateFound bool
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
					_, thumbPath, candidateFound, tErr := thumbGen.Refresh(j.workID, j.absPath)
					if tErr != nil {
						log.Printf("サムネイル生成失敗 work_id=%d: %v", j.workID, tErr)
						continue
					}
					if !candidateFound || thumbPath != "" {
						results <- thumbResult{
							workID:         j.workID,
							thumbPath:      thumbPath,
							candidateFound: candidateFound,
						}
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
			if !r.candidateFound {
				if _, uErr := db.Exec(
					"UPDATE works SET thumbnail_path=NULL, updated_at=datetime('now') WHERE id=? AND thumbnail_path IS NOT NULL",
					r.workID,
				); uErr != nil {
					log.Printf("thumbnail_path クリア失敗 work_id=%d: %v", r.workID, uErr)
				}
				continue
			}
			if r.thumbPath != "" {
				if _, uErr := db.Exec(
					"UPDATE works SET thumbnail_path=?, updated_at=datetime('now') WHERE id=?",
					r.thumbPath, r.workID,
				); uErr != nil {
					log.Printf("thumbnail_path 更新失敗 work_id=%d: %v", r.workID, uErr)
				}
			}
		}
	}

	return res, nil
}

// upsertWorkNoThumb は 1 つのフォルダを works テーブルに upsert する(サムネイル生成なし)。
// 返り値は (新規作成されたか, CSV 既存行に root_path がリンクされたか, 作成/更新された work の ID, エラー)。
func upsertWorkNoThumb(db execQuerier, absPath, dirName string) (newlyRegistered bool, linkedToCSV bool, workID int64, err error) {
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
func upsertByRJ(db execQuerier, rjNumber, absPath, dirName string) (newlyRegistered bool, linkedToCSV bool, workID int64, err error) {
	// タイトル候補: RJ 番号の直後の文字列から先頭の区切り文字(_ - 半角スペース)を
	// 取り除いたもの。フォルダ名全体で最初の "_" を探すと、RJ 番号直後が "_" 以外の
	// 区切り(例: "-作品名_ver2")の場合に後方の "_" で誤って区切ってしまうため、
	// RJ 番号の直後だけを見る(issue #65)。
	rest := dirName[len(rjNumber):]
	title := strings.TrimLeft(rest, "_- ")
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
	if !wasNull {
		// 別の非 NULL パスからの上書き = 同一 RJ 番号のフォルダが複数ルート
		// (または同一ルート内の複数フォルダ)に存在する可能性が高い。
		// 後勝ちで上書きする挙動は従来どおりだが、静かに切り替わると
		// 気付けないためログに残す(issue #70)。
		log.Printf("スキャン: %s の root_path を上書きします(同一 RJ 番号のフォルダが複数存在する可能性): %q → %q",
			rjNumber, currentRootPath.String, absPath)
	}
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
func upsertByPath(db execQuerier, absPath, dirName string) (newlyRegistered bool, workID int64, err error) {
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
			// タイトル一致だけでは「以前 NULL 化された同一作品」と「別の同名フォルダ」を
			// 区別できないため、誤帰属を後から追跡できるよう監査ログを残す(issue #81)
			log.Printf("孤児行 id=%d (title=%q) を root_path=%q に再リンクした", orphanID, dirName, absPath)
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
func findOrphanWorkByTitle(db execQuerier, title string) (id int64, found bool, err error) {
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

// markMissingPaths は DB にある root_path のうち、foundPaths に含まれないものを
// root_path・thumbnail_path とも NULL に戻す(thumbnail_path を残すと /api/works が
// has_folder=false でも古いサムネイルの thumbnail_url を返してしまうため。
// PR #89 レビュー指摘)。
// ただし failedRoots 配下(読み込みに失敗したルート)にあるパスは、
// 「実際に消えた」のか「読めなかっただけ」なのか区別できないため対象から除外する。
// NULL 化した work の ID 一覧を返す(呼び出し元がサムネイルキャッシュファイルの削除に使う)。
func markMissingPaths(db *sql.DB, foundPaths map[string]bool, failedRoots []string) ([]int64, error) {
	rows, err := db.Query("SELECT id, root_path FROM works WHERE root_path IS NOT NULL")
	if err != nil {
		return nil, fmt.Errorf("root_path 一覧取得失敗: %w", err)
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
			return nil, err
		}
		if !foundPaths[r.path] && !underFailedRoot(r.path, failedRoots) {
			toNull = append(toNull, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 以降は db.Exec によるトランザクション開始が必要になるため、SELECT のカーソルは
	// ここで明示的に閉じておく(defer によるクローズも安全のため残す)。
	rows.Close()

	if len(toNull) == 0 {
		return nil, nil
	}

	// NULL 化 UPDATE ループを 1 トランザクションにまとめ、件数分の fsync を避ける。
	// upsert ループと同様、1 件の失敗でトランザクション全体を失わないよう
	// ログして継続し、最後に Commit する。
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("root_path NULL 化用トランザクション開始失敗: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nulledIDs []int64
	for _, r := range toNull {
		if _, err := tx.Exec(
			"UPDATE works SET root_path=NULL, thumbnail_path=NULL, updated_at=datetime('now') WHERE id=?",
			r.id,
		); err != nil {
			log.Printf("root_path NULL 化失敗 id=%d: %v", r.id, err)
			continue
		}
		nulledIDs = append(nulledIDs, r.id)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("root_path NULL 化用トランザクションのコミット失敗: %w", err)
	}
	return nulledIDs, nil
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
