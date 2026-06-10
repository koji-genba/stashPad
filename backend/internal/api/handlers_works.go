package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
		for _, ts := range strings.Split(tagsParam, ",") {
			ts = strings.TrimSpace(ts)
			if id, err := strconv.ParseInt(ts, 10, 64); err == nil {
				tagIDs = append(tagIDs, id)
			}
		}
	}

	// クエリ組み立て
	args := []any{}
	whereClause := ""

	if keyword != "" {
		whereClause += " AND (w.title LIKE ? OR w.circle LIKE ? OR w.rj_number LIKE ?)"
		like := "%" + keyword + "%"
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
	)
	err = s.db.QueryRow(
		`SELECT id, rj_number, title, circle, series_name, purchase_date,
		        work_type, age_rating, file_format, file_size_text,
		        root_path, thumbnail_path
		 FROM works WHERE id=?`, workID,
	).Scan(&id, &rjNumber, &title, &circle, &seriesName,
		&purchaseDate, &workType, &ageRating, &fileFormat,
		&fileSizeText, &rootPath, &thumbPath)
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
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
		return
	}

	// 存在確認
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM works WHERE id=?)", workID).Scan(&exists); err != nil || !exists {
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

	// 作品存在確認
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM works WHERE id=?)", workID).Scan(&exists); err != nil || !exists {
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

	// 作品存在確認
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM works WHERE id=?)", workID).Scan(&exists); err != nil || !exists {
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
