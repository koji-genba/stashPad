package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/koji-genba/stashpad/backend/internal/media"
	"github.com/koji-genba/stashpad/backend/internal/thumb"
)

// ---- 作品一覧 ----------------------------------------------------------------

// worksListResponse は GET /api/works のレスポンス。
type worksListResponse struct {
	Items []map[string]any `json:"items"`
	Total int              `json:"total"`
	Page  int              `json:"page"`
	Limit int              `json:"limit"`
}

// handleListWorks は GET /api/works を処理する。
func (s *Server) handleListWorks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	keyword := q.Get("q")
	tagsParam := q.Get("tags")
	circleFilter := q.Get("circle")
	seriesFilter := q.Get("series")
	sortBy := q.Get("sort")
	order := q.Get("order")
	pageStr := q.Get("page")
	limitStr := q.Get("limit")
	// hidden パラメータ: "1" → 非表示作品のみ、それ以外(未指定/"0") → 可視作品のみ
	hiddenParam := q.Get("hidden")

	page := 1
	limit := 40
	if pageStr != "" {
		if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
			page = v
		}
	}
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}

	// ソートカラムのホワイトリスト(circle を追加)
	allowedSort := map[string]string{
		"purchase_date": "purchase_date",
		"title":         "title",
		"created_at":    "created_at",
		"circle":        "circle",
	}
	sortCol, ok := allowedSort[sortBy]
	if !ok {
		sortCol = "purchase_date"
	}
	if order != "asc" {
		order = "desc"
	}

	// タグ AND フィルタ
	var tagIDs []int64
	if tagsParam != "" {
		tagIDs = parseTagIDs(tagsParam)
	}

	// タグ NOT フィルタ(exclude_tags パラメータ)
	var excludeTagIDs []int64
	if excludeTagsParam := q.Get("exclude_tags"); excludeTagsParam != "" {
		excludeTagIDs = parseTagIDs(excludeTagsParam)
	}

	// クエリ組み立て
	args := []any{}
	whereClause := ""

	// キーワードをターム分割してAND/NOT検索に変換
	includeTerms, excludeTerms := parseSearchTerms(keyword)
	for _, term := range includeTerms {
		like := "%" + term + "%"
		whereClause += " AND (w.title LIKE ? OR w.circle LIKE ? OR w.rj_number LIKE ?)"
		args = append(args, like, like, like)
	}
	for _, term := range excludeTerms {
		like := "%" + term + "%"
		// circle は NULL の可能性があるため COALESCE で空文字に変換して NOT 判定する
		// (NULL LIKE ? は NULL になり NOT NULL = NULL → 行が除外されてしまうため)
		whereClause += " AND NOT (w.title LIKE ? OR COALESCE(w.circle, '') LIKE ? OR w.rj_number LIKE ?)"
		args = append(args, like, like, like)
	}

	// circle 完全一致フィルタ
	if circleFilter != "" {
		whereClause += " AND w.circle=?"
		args = append(args, circleFilter)
	}

	// series_name 完全一致フィルタ
	if seriesFilter != "" {
		whereClause += " AND w.series_name=?"
		args = append(args, seriesFilter)
	}

	for _, tid := range tagIDs {
		whereClause += " AND EXISTS(SELECT 1 FROM work_tags wt WHERE wt.work_id=w.id AND wt.tag_id=?)"
		args = append(args, tid)
	}

	// exclude_tags: 指定タグを持つ作品を除外
	for _, tid := range excludeTagIDs {
		whereClause += " AND NOT EXISTS(SELECT 1 FROM work_tags wt WHERE wt.work_id=w.id AND wt.tag_id=?)"
		args = append(args, tid)
	}

	// hidden フィルタ: "1" → 非表示のみ、それ以外 → 可視のみ(デフォルト)
	if hiddenParam == "1" {
		whereClause += " AND w.hidden=1"
	} else {
		whereClause += " AND w.hidden=0"
	}

	countQuery := "SELECT COUNT(*) FROM works w WHERE 1=1" + whereClause
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "件数取得失敗: "+err.Error())
		return
	}

	offset := (page - 1) * limit
	dataQuery := fmt.Sprintf(
		`SELECT w.id, w.rj_number, w.title, w.circle, w.age_rating,
		        (w.root_path IS NOT NULL) AS has_folder,
		        w.thumbnail_path
		 FROM works w
		 WHERE 1=1%s
		 ORDER BY w.%s %s NULLS LAST, w.id %s
		 LIMIT ? OFFSET ?`,
		whereClause, sortCol, order, order,
	)
	dataArgs := append(args, limit, offset)

	rows, err := s.db.Query(dataQuery, dataArgs...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "一覧取得失敗: "+err.Error())
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var rj, circle, ageRating, thumbPath sql.NullString
		var title string
		var hasFolder bool

		if err := rows.Scan(&id, &rj, &title, &circle, &ageRating, &hasFolder, &thumbPath); err != nil {
			respondError(w, http.StatusInternalServerError, "スキャン失敗: "+err.Error())
			return
		}

		item := map[string]any{
			"id":         id,
			"title":      title,
			"has_folder": hasFolder,
		}
		if rj.Valid {
			item["rj_number"] = rj.String
		}
		if circle.Valid {
			item["circle"] = circle.String
		}
		if ageRating.Valid {
			item["age_rating"] = ageRating.String
		}
		if thumbPath.Valid {
			item["thumbnail_url"] = fmt.Sprintf("/api/works/%d/thumbnail", id)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "行読み込み失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, worksListResponse{
		Items: items,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// ---- 作品詳細 ----------------------------------------------------------------

// handleGetWork は GET /api/works/{id} を処理する。
func (s *Server) handleGetWork(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	var (
		id           int64
		rjNumber     sql.NullString
		title        string
		circle       sql.NullString
		seriesName   sql.NullString
		purchaseDate sql.NullString
		workType     sql.NullString
		ageRating    sql.NullString
		fileFormat   sql.NullString
		fileSizeText sql.NullString
		rootPath     sql.NullString
		thumbPath    sql.NullString
		hiddenInt    int // hidden は INTEGER(0/1)。bool 変換して返す
	)
	err = s.db.QueryRow(
		`SELECT id, rj_number, title, circle, series_name, purchase_date,
		        work_type, age_rating, file_format, file_size_text,
		        root_path, thumbnail_path, hidden
		 FROM works WHERE id=?`, workID,
	).Scan(&id, &rjNumber, &title, &circle, &seriesName,
		&purchaseDate, &workType, &ageRating, &fileFormat,
		&fileSizeText, &rootPath, &thumbPath, &hiddenInt)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB 取得失敗: "+err.Error())
		return
	}

	// タグ一覧
	tagRows, err := s.db.Query(
		`SELECT t.id, t.name, t.category
		 FROM tags t
		 JOIN work_tags wt ON wt.tag_id=t.id
		 WHERE wt.work_id=?
		 ORDER BY t.category, t.name`, workID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "タグ取得失敗: "+err.Error())
		return
	}
	defer tagRows.Close()

	tags := make([]map[string]any, 0)
	for tagRows.Next() {
		var tid int64
		var name, cat string
		if err := tagRows.Scan(&tid, &name, &cat); err != nil {
			respondError(w, http.StatusInternalServerError, "タグスキャン失敗: "+err.Error())
			return
		}
		tags = append(tags, map[string]any{"id": tid, "name": name, "category": cat})
	}

	result := map[string]any{
		"id":         id,
		"title":      title,
		"has_folder": rootPath.Valid,
		"tags":       tags,
		"hidden":     hiddenInt != 0, // INTEGER 0/1 を bool に変換
	}
	setIfValid(result, "rj_number", rjNumber)
	setIfValid(result, "circle", circle)
	setIfValid(result, "series_name", seriesName)
	setIfValid(result, "purchase_date", purchaseDate)
	setIfValid(result, "work_type", workType)
	setIfValid(result, "age_rating", ageRating)
	setIfValid(result, "file_format", fileFormat)
	setIfValid(result, "file_size_text", fileSizeText)
	if thumbPath.Valid {
		result["thumbnail_url"] = fmt.Sprintf("/api/works/%d/thumbnail", id)
	}

	respondJSON(w, http.StatusOK, result)
}

// ---- 作品編集 ----------------------------------------------------------------

// handlePatchWork は PATCH /api/works/{id} を処理する(タイトル等の手動編集)。
func (s *Server) handlePatchWork(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	var body struct {
		Title  *string `json:"title"`
		Circle *string `json:"circle"`
		Hidden *bool   `json:"hidden"` // 非表示フラグ。true→非表示、false→可視
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
		return
	}

	// 存在確認(DB エラーは 500、不存在は 404 を返す)
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM works WHERE id=?)", workID).Scan(&exists); err != nil {
		respondError(w, http.StatusInternalServerError, "DB エラー: "+err.Error())
		return
	}
	if !exists {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}

	if body.Title != nil {
		if _, err := s.db.Exec(
			"UPDATE works SET title=?, updated_at=datetime('now') WHERE id=?",
			*body.Title, workID,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "更新失敗: "+err.Error())
			return
		}
	}
	if body.Circle != nil {
		if _, err := s.db.Exec(
			"UPDATE works SET circle=?, updated_at=datetime('now') WHERE id=?",
			*body.Circle, workID,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "更新失敗: "+err.Error())
			return
		}
	}
	if body.Hidden != nil {
		// bool を INTEGER(0/1) に変換して保存
		hiddenVal := 0
		if *body.Hidden {
			hiddenVal = 1
		}
		if _, err := s.db.Exec(
			"UPDATE works SET hidden=?, updated_at=datetime('now') WHERE id=?",
			hiddenVal, workID,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "更新失敗: "+err.Error())
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---- タグ操作 ----------------------------------------------------------------

// handleAddTag は POST /api/works/{id}/tags を処理する。
func (s *Server) handleAddTag(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
		return
	}
	if body.Name == "" {
		respondError(w, http.StatusBadRequest, "name が空です")
		return
	}

	// 作品存在確認(DB エラーは 500、不存在は 404 を返す)
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM works WHERE id=?)", workID).Scan(&exists); err != nil {
		respondError(w, http.StatusInternalServerError, "DB エラー: "+err.Error())
		return
	}
	if !exists {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}

	// custom タグ upsert
	if _, err := s.db.Exec(
		"INSERT INTO tags (name, category) VALUES (?, 'custom') ON CONFLICT(name, category) DO NOTHING",
		body.Name,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "タグ作成失敗: "+err.Error())
		return
	}
	var tagID int64
	if err := s.db.QueryRow(
		"SELECT id FROM tags WHERE name=? AND category='custom'", body.Name,
	).Scan(&tagID); err != nil {
		respondError(w, http.StatusInternalServerError, "タグ ID 取得失敗: "+err.Error())
		return
	}

	if _, err := s.db.Exec(
		"INSERT INTO work_tags (work_id, tag_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
		workID, tagID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "タグリンク失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{"id": tagID, "name": body.Name, "category": "custom"})
}

// handleDeleteTag は DELETE /api/works/{id}/tags/{tag_id} を処理する。
func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な work ID")
		return
	}
	tagIDStr := chi.URLParam(r, "tag_id")
	tagID, err := strconv.ParseInt(tagIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な tag_id")
		return
	}

	if _, err := s.db.Exec(
		"DELETE FROM work_tags WHERE work_id=? AND tag_id=?",
		workID, tagID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "タグ削除失敗: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- サムネイル -------------------------------------------------------------

// handleWorkThumbnail は GET /api/works/{id}/thumbnail を処理する。
func (s *Server) handleWorkThumbnail(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	var thumbPath sql.NullString
	if err := s.db.QueryRow(
		"SELECT thumbnail_path FROM works WHERE id=?", workID,
	).Scan(&thumbPath); err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "DB 取得失敗: "+err.Error())
		return
	}

	if !thumbPath.Valid {
		respondError(w, http.StatusNotFound, "サムネイルがありません")
		return
	}

	f, err := os.Open(thumbPath.String)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "サムネイルファイルが見つかりません")
			return
		}
		respondError(w, http.StatusInternalServerError, "ファイルオープン失敗: "+err.Error())
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Stat 失敗: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

// thumbCheckThrottle はサムネイルチェックを抑制する期間。
// 同じ作品に対して詳細ページを開くたびに数百ファイルの Stat が走るのを防ぐ。
const thumbCheckThrottle = 24 * time.Hour

// handleRefreshThumbnail は POST /api/works/{id}/thumbnail/refresh を処理する。
// 前回チェックから thumbCheckThrottle 未満の場合は IO を完全スキップして返す。
func (s *Server) handleRefreshThumbnail(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	var (
		rootPath       sql.NullString
		thumbCheckedAt sql.NullString
	)
	if err := s.db.QueryRow(
		"SELECT root_path, thumb_checked_at FROM works WHERE id=?", workID,
	).Scan(&rootPath, &thumbCheckedAt); err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "DB 取得失敗: "+err.Error())
		return
	}

	// root_path が NULL の作品はフォルダが存在しないのでスキップ
	if !rootPath.Valid {
		respondJSON(w, http.StatusOK, map[string]any{"refreshed": false})
		return
	}

	// 前回チェックから thumbCheckThrottle 未満なら IO を一切行わずに返す。
	// SQLite の datetime 比較: strftime('%s', 'now') で UNIX 秒を取得し秒数差で判定する。
	// (datetime(...) との比較よりも整数演算の方が精度・移植性ともに安定している)
	if thumbCheckedAt.Valid {
		var isThrottled bool
		throttleSec := int64(thumbCheckThrottle.Seconds())
		if err := s.db.QueryRow(
			"SELECT (strftime('%s', 'now') - strftime('%s', ?)) < ?",
			thumbCheckedAt.String, throttleSec,
		).Scan(&isThrottled); err == nil && isThrottled {
			respondJSON(w, http.StatusOK, map[string]any{"refreshed": false})
			return
		}
	}

	thumbsDir := filepath.Join(s.cfg.DataDir, "thumbs")
	gen := thumb.New(thumbsDir)

	regenerated, outPath, err := gen.Refresh(workID, rootPath.String)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "サムネイル再生成失敗: "+err.Error())
		return
	}

	if regenerated && outPath != "" {
		if _, uErr := s.db.Exec(
			"UPDATE works SET thumbnail_path=?, updated_at=datetime('now') WHERE id=?",
			outPath, workID,
		); uErr != nil {
			respondError(w, http.StatusInternalServerError, "thumbnail_path 更新失敗: "+uErr.Error())
			return
		}
	}

	// 再生成の有無に関わらず、チェックを実行した時刻を記録してスロットルを有効化する
	if _, uErr := s.db.Exec(
		"UPDATE works SET thumb_checked_at=datetime('now') WHERE id=?", workID,
	); uErr != nil {
		respondError(w, http.StatusInternalServerError, "thumb_checked_at 更新失敗: "+uErr.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"refreshed": regenerated})
}

// handleRebuildThumbnails は POST /api/thumbnails/rebuild を処理する。
// root_path がある全作品に対して mtime 判定付きサムネイル再生成を worker pool で並列実行する。
func (s *Server) handleRebuildThumbnails(w http.ResponseWriter, r *http.Request) {
	// root_path がある作品一覧を取得
	rows, err := s.db.Query("SELECT id, root_path FROM works WHERE root_path IS NOT NULL")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "作品一覧取得失敗: "+err.Error())
		return
	}

	type workEntry struct {
		id       int64
		rootPath string
	}
	var works []workEntry
	for rows.Next() {
		var we workEntry
		if err := rows.Scan(&we.id, &we.rootPath); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, "行読み込み失敗: "+err.Error())
			return
		}
		works = append(works, we)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "クエリエラー: "+err.Error())
		return
	}

	thumbsDir := filepath.Join(s.cfg.DataDir, "thumbs")
	gen := thumb.New(thumbsDir)

	type result struct {
		id          int64
		outPath     string
		regenerated bool
		err         error
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobs := make(chan workEntry, len(works))
	results := make(chan result, len(works))

	// ワーカー起動
	for i := 0; i < numWorkers; i++ {
		go func() {
			for we := range jobs {
				regen, outPath, err := gen.Refresh(we.id, we.rootPath)
				results <- result{id: we.id, outPath: outPath, regenerated: regen, err: err}
			}
		}()
	}

	// ジョブ投入
	for _, we := range works {
		jobs <- we
	}
	close(jobs)

	// 結果収集・DB 更新(SQLite は書き込みを直列化するためここで順次実行)
	checked := len(works)
	regenerated := 0
	for range works {
		res := <-results
		if res.err != nil {
			// ログには出力するが全体は継続
			log.Printf("サムネイル再生成失敗 work_id=%d: %v", res.id, res.err)
			continue
		}
		if res.regenerated && res.outPath != "" {
			regenerated++
			if _, uErr := s.db.Exec(
				"UPDATE works SET thumbnail_path=?, updated_at=datetime('now') WHERE id=?",
				res.outPath, res.id,
			); uErr != nil {
				// 更新失敗はログのみ、継続
				_ = uErr
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"checked":     checked,
		"regenerated": regenerated,
	})
}

// ---- エントリ一覧 -----------------------------------------------------------

// entryItem は /api/works/{id}/entries の 1 エントリ。
type entryItem struct {
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	MediaKind string `json:"media_kind"`
}

// handleWorkEntries は GET /api/works/{id}/entries?path= を処理する。
func (s *Server) handleWorkEntries(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	rootPath, err := s.getWorkRootPath(workID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB エラー: "+err.Error())
		return
	}
	if rootPath == "" {
		respondError(w, http.StatusNotFound, "作品フォルダが登録されていません")
		return
	}

	relPath := r.URL.Query().Get("path")

	// パストラバーサル検証
	resolvedPath, resolveErr := media.ResolvePath(rootPath, relPath)
	if resolveErr == media.ErrForbidden {
		respondError(w, http.StatusForbidden, "アクセス禁止")
		return
	}
	if resolveErr == media.ErrNotFound {
		respondError(w, http.StatusNotFound, "パスが見つかりません")
		return
	}
	if resolveErr != nil {
		respondError(w, http.StatusInternalServerError, "パス解決失敗: "+resolveErr.Error())
		return
	}

	// ディレクトリであることを確認
	st, err := os.Stat(resolvedPath)
	if err != nil {
		respondError(w, http.StatusNotFound, "パスが見つかりません")
		return
	}
	if !st.IsDir() {
		respondError(w, http.StatusBadRequest, "指定パスはディレクトリではありません")
		return
	}

	dirEntries, err := os.ReadDir(resolvedPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "ディレクトリ読み込み失敗: "+err.Error())
		return
	}

	// ディレクトリとファイルに分けて、それぞれ自然順ソート
	var dirs, files []entryItem
	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		item := entryItem{
			Name:  e.Name(),
			IsDir: e.IsDir(),
		}
		if !e.IsDir() {
			item.Size = info.Size()
			item.MediaKind = media.KindByExt(e.Name())
		}
		if e.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}

	// 自然順ソート
	sortEntries(dirs)
	sortEntries(files)

	// ディレクトリ → ファイルの順で結合
	result := append(dirs, files...)
	if result == nil {
		result = []entryItem{}
	}

	// 親パスを計算
	parent := ""
	if relPath != "" && relPath != "." {
		parent = filepath.Dir(relPath)
		if parent == "." {
			parent = ""
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"path":    relPath,
		"parent":  parent,
		"entries": result,
	})
}

// sortEntries は entryItem スライスを自然順でソートする。
func sortEntries(items []entryItem) {
	sort.SliceStable(items, func(i, j int) bool {
		return media.NaturalLess(items[i].Name, items[j].Name)
	})
}

// ---- ファイル配信 -----------------------------------------------------------

// handleWorkFile は GET /api/works/{id}/file?path= を処理する。
func (s *Server) handleWorkFile(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	rootPath, err := s.getWorkRootPath(workID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB エラー: "+err.Error())
		return
	}
	if rootPath == "" {
		respondError(w, http.StatusNotFound, "作品フォルダが登録されていません")
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		respondError(w, http.StatusBadRequest, "path パラメータが必要です")
		return
	}

	// パストラバーサル検証
	resolvedPath, resolveErr := media.ResolvePath(rootPath, relPath)
	if resolveErr == media.ErrForbidden {
		respondError(w, http.StatusForbidden, "アクセス禁止")
		return
	}
	if resolveErr == media.ErrNotFound {
		respondError(w, http.StatusNotFound, "ファイルが見つかりません")
		return
	}
	if resolveErr != nil {
		respondError(w, http.StatusInternalServerError, "パス解決失敗: "+resolveErr.Error())
		return
	}

	f, err := os.Open(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "ファイルが見つかりません")
			return
		}
		respondError(w, http.StatusInternalServerError, "ファイルオープン失敗: "+err.Error())
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Stat 失敗: "+err.Error())
		return
	}
	if st.IsDir() {
		respondError(w, http.StatusBadRequest, "ディレクトリは配信できません")
		return
	}

	// Content-Type を拡張子から決定
	ct := mime.TypeByExtension(filepath.Ext(resolvedPath))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)

	// media_kind が other のファイルはダウンロード扱い(Content-Disposition: attachment)
	// RFC5987 に基づいて日本語ファイル名をエンコードする
	if media.KindByExt(st.Name()) == "other" {
		encoded := url.PathEscape(st.Name())
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename*=UTF-8''%s", encoded))
	}

	// http.ServeContent が Range / 206 / HEAD を処理する
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

// ---- 再生履歴 ----------------------------------------------------------------

// handleRecordPlay は POST /api/works/{id}/plays を処理する。
func (s *Server) handleRecordPlay(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
		return
	}
	if body.Path == "" {
		respondError(w, http.StatusBadRequest, "path が空です")
		return
	}

	// 作品存在確認(DB エラーは 500、不存在は 404 を返す)
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM works WHERE id=?)", workID).Scan(&exists); err != nil {
		respondError(w, http.StatusInternalServerError, "DB エラー: "+err.Error())
		return
	}
	if !exists {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}

	if _, err := s.db.Exec(
		"INSERT INTO play_history (work_id, file_path) VALUES (?, ?)",
		workID, body.Path,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "履歴記録失敗: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// ---- ヘルパー ---------------------------------------------------------------

// getWorkRootPath は work_id から root_path を取得する。
// root_path が NULL の場合は空文字列を返す。
func (s *Server) getWorkRootPath(workID int64) (string, error) {
	var rootPath sql.NullString
	err := s.db.QueryRow("SELECT root_path FROM works WHERE id=?", workID).Scan(&rootPath)
	if err != nil {
		return "", err
	}
	if !rootPath.Valid {
		return "", nil
	}
	return rootPath.String, nil
}

// parseWorkID は chi の URL パラメータから work_id を取得する。
func parseWorkID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// setIfValid は NullString が有効な場合のみ map にセットする。
func setIfValid(m map[string]any, key string, v sql.NullString) {
	if v.Valid {
		m[key] = v.String
	}
}
