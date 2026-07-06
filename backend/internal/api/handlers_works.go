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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/koji-genba/stashpad/backend/internal/media"
	"github.com/koji-genba/stashpad/backend/internal/thumb"
)

// ---- 作品一覧 ----------------------------------------------------------------

// workListItem は GET /api/works の items 要素。
//
// NULL 許容カラム(rj_number/circle/age_rating/thumbnail_url)は *string にして
// omitempty を付けない。値が NULL の場合は JSON で明示的に `null` を返す
// (以前は setIfValid でキー自体を省略していたが、フロントの `string | null` 型定義と
// 実際の契約を一致させるため typed struct + 明示 null に統一する。issue #57/#38-2)。
type workListItem struct {
	ID           int64   `json:"id"`
	RJNumber     *string `json:"rj_number"`
	Title        string  `json:"title"`
	Circle       *string `json:"circle"`
	AgeRating    *string `json:"age_rating"`
	Rating       *int    `json:"rating"`
	HasFolder    bool    `json:"has_folder"`
	ThumbnailURL *string `json:"thumbnail_url"`
	Favorited    bool    `json:"favorited"`
}

// worksListResponse は GET /api/works のレスポンス。
type worksListResponse struct {
	Items []workListItem `json:"items"`
	Total int            `json:"total"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
}

// handleListWorks は GET /api/works を処理する。
func (s *Server) handleListWorks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	keyword := q.Get("q")
	tagsParam := q.Get("tags")
	circleFilter := q.Get("circle")
	seriesFilter := q.Get("series")
	workTypeFilter := q.Get("work_type")
	ageRatingFilter := q.Get("age_rating")
	sortBy := q.Get("sort")
	order := q.Get("order")
	pageStr := q.Get("page")
	limitStr := q.Get("limit")
	// hidden パラメータ: "1" → 非表示作品のみ、それ以外(未指定/"0") → 可視作品のみ
	hiddenParam := q.Get("hidden")
	// favorite パラメータ: "1" → お気に入りのみ
	favoriteParam := q.Get("favorite")

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

	// ソートカラムのホワイトリスト。
	// favorited_at / last_played / play_count は列そのものではなく式(相関サブクエリ含む)なので
	// テーブル修飾やサブクエリを直接値に持たせる。
	// last_played は MAX(played_at) が未再生で自然に NULL になるが、play_count は
	// COUNT(*) が未再生で 0 になり NULL にならないため NULLIF で 0 → NULL に変換し、
	// NULLS LAST で未再生が常に末尾に来るようにする。
	allowedSort := map[string]string{
		"purchase_date": "w.purchase_date",
		"rj_number":     "CAST(SUBSTR(w.rj_number, 3) AS INTEGER)",
		"title":         "w.title",
		"created_at":    "w.created_at",
		"circle":        "w.circle",
		"rating":        "w.rating",
		"favorited_at":  "w.favorited_at",
		"last_played":   "(SELECT MAX(ph.played_at) FROM play_history ph WHERE ph.work_id=w.id)",
		"play_count":    "(SELECT NULLIF(COUNT(*), 0) FROM play_history ph WHERE ph.work_id=w.id)",
	}
	sortExpr, ok := allowedSort[sortBy]
	if !ok {
		sortExpr = allowedSort["purchase_date"]
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
		like := likeContains(term)
		whereClause += " AND (w.title LIKE ? ESCAPE '\\' OR w.circle LIKE ? ESCAPE '\\' OR w.rj_number LIKE ? ESCAPE '\\')"
		args = append(args, like, like, like)
	}
	for _, term := range excludeTerms {
		like := likeContains(term)
		// circle・rj_number は NULL の可能性があるため COALESCE で空文字に変換して
		// NOT 判定する(NULL LIKE ? は NULL になり NOT(... OR NULL) も NULL になって
		// 行自体が WHERE から除外されてしまうため。PR #79 レビュー指摘: rj_number だけ
		// COALESCE が漏れていた)。
		whereClause += " AND NOT (w.title LIKE ? ESCAPE '\\' OR COALESCE(w.circle, '') LIKE ? ESCAPE '\\' OR COALESCE(w.rj_number, '') LIKE ? ESCAPE '\\')"
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

	// work_type 完全一致フィルタ
	if workTypeFilter != "" {
		whereClause += " AND w.work_type=?"
		args = append(args, workTypeFilter)
	}

	// age_rating 完全一致フィルタ
	if ageRatingFilter != "" {
		whereClause += " AND w.age_rating=?"
		args = append(args, ageRatingFilter)
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

	// favorite フィルタ: "1" → お気に入りのみ
	if favoriteParam == "1" {
		whereClause += " AND w.favorited_at IS NOT NULL"
	}

	countQuery := "SELECT COUNT(*) FROM works w WHERE 1=1" + whereClause
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		respondInternalError(w, "件数取得失敗", err)
		return
	}

	offset := (page - 1) * limit
	dataQuery := fmt.Sprintf(
		`SELECT w.id, w.rj_number, w.title, w.circle, w.age_rating, w.rating,
		        (w.root_path IS NOT NULL) AS has_folder,
		        w.thumbnail_path, w.favorited_at
		 FROM works w
		 WHERE 1=1%s
		 ORDER BY %s %s NULLS LAST, w.id %s
		 LIMIT ? OFFSET ?`,
		whereClause, sortExpr, order, order,
	)
	// 将来 args を再利用する変更で append が裏配列を破壊するのを防ぐため、
	// 独立スライスへコピーしてから limit/offset を足す。
	dataArgs := append(slices.Clone(args), limit, offset)

	rows, err := s.db.Query(dataQuery, dataArgs...)
	if err != nil {
		respondInternalError(w, "一覧取得失敗", err)
		return
	}
	defer rows.Close()

	items := make([]workListItem, 0)
	for rows.Next() {
		var id int64
		var rj, circle, ageRating, thumbPath, favoritedAt sql.NullString
		var rating sql.NullInt64
		var title string
		var hasFolder bool

		if err := rows.Scan(&id, &rj, &title, &circle, &ageRating, &rating, &hasFolder, &thumbPath, &favoritedAt); err != nil {
			respondInternalError(w, "スキャン失敗", err)
			return
		}

		item := workListItem{
			ID:        id,
			RJNumber:  nullableString(rj),
			Title:     title,
			Circle:    nullableString(circle),
			AgeRating: nullableString(ageRating),
			Rating:    nullableInt(rating),
			HasFolder: hasFolder,
			Favorited: favoritedAt.Valid,
		}
		if thumbPath.Valid {
			item.ThumbnailURL = strPtr(fmt.Sprintf("/api/works/%d/thumbnail", id))
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, "行読み込み失敗", err)
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

// workTagItem は GET /api/works/{id} の tags 要素。
type workTagItem struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// workDetailResponse は GET /api/works/{id} のレスポンス。
// workListItem と同様、NULL 許容カラムは *string で明示的に null を返す(issue #57/#38-2)。
type workDetailResponse struct {
	ID           int64         `json:"id"`
	RJNumber     *string       `json:"rj_number"`
	Title        string        `json:"title"`
	Circle       *string       `json:"circle"`
	SeriesName   *string       `json:"series_name"`
	PurchaseDate *string       `json:"purchase_date"`
	WorkType     *string       `json:"work_type"`
	AgeRating    *string       `json:"age_rating"`
	Rating       *int          `json:"rating"`
	FileFormat   *string       `json:"file_format"`
	FileSizeText *string       `json:"file_size_text"`
	HasFolder    bool          `json:"has_folder"`
	Tags         []workTagItem `json:"tags"`
	Hidden       bool          `json:"hidden"`
	Favorited    bool          `json:"favorited"`
	ThumbnailURL *string       `json:"thumbnail_url"`
}

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
		rating       sql.NullInt64
		rootPath     sql.NullString
		thumbPath    sql.NullString
		hiddenInt    int // hidden は INTEGER(0/1)。bool 変換して返す
		favoritedAt  sql.NullString
	)
	err = s.db.QueryRow(
		`SELECT id, rj_number, title, circle, series_name, purchase_date,
		        work_type, age_rating, rating, file_format, file_size_text,
		        root_path, thumbnail_path, hidden, favorited_at
		 FROM works WHERE id=?`, workID,
	).Scan(&id, &rjNumber, &title, &circle, &seriesName,
		&purchaseDate, &workType, &ageRating, &rating, &fileFormat,
		&fileSizeText, &rootPath, &thumbPath, &hiddenInt, &favoritedAt)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}
	if err != nil {
		respondInternalError(w, "DB 取得失敗", err)
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
		respondInternalError(w, "タグ取得失敗", err)
		return
	}
	defer tagRows.Close()

	tags := make([]workTagItem, 0)
	for tagRows.Next() {
		var tid int64
		var name, cat string
		if err := tagRows.Scan(&tid, &name, &cat); err != nil {
			respondInternalError(w, "タグスキャン失敗", err)
			return
		}
		tags = append(tags, workTagItem{ID: tid, Name: name, Category: cat})
	}
	if err := tagRows.Err(); err != nil {
		respondInternalError(w, "タグ取得失敗", err)
		return
	}

	result := workDetailResponse{
		ID:           id,
		RJNumber:     nullableString(rjNumber),
		Title:        title,
		Circle:       nullableString(circle),
		SeriesName:   nullableString(seriesName),
		PurchaseDate: nullableString(purchaseDate),
		WorkType:     nullableString(workType),
		AgeRating:    nullableString(ageRating),
		Rating:       nullableInt(rating),
		FileFormat:   nullableString(fileFormat),
		FileSizeText: nullableString(fileSizeText),
		HasFolder:    rootPath.Valid,
		Tags:         tags,
		Hidden:       hiddenInt != 0, // INTEGER 0/1 を bool に変換
		Favorited:    favoritedAt.Valid,
	}
	if thumbPath.Valid {
		result.ThumbnailURL = strPtr(fmt.Sprintf("/api/works/%d/thumbnail", id))
	}

	respondJSON(w, http.StatusOK, result)
}

// ---- 作品編集 ----------------------------------------------------------------

// maxTitleRunes / maxCircleRunes は手動編集で許容する文字数の上限(rune 数)。
// 極端に長い値が UI 崩れや DB 肥大化の原因になるのを防ぐための緩い上限であり、
// 厳密な仕様値ではない(issue #63)。
const (
	maxTitleRunes  = 200
	maxCircleRunes = 200
)

// handlePatchWork は PATCH /api/works/{id} を処理する(タイトル等の手動編集)。
//
// title / circle / hidden / favorite / manually_edited への反映は単一の UPDATE 文に
// 統合する(issue #63)。各フィールドは JSON に「キーが存在する場合のみ」動的に
// SET 句へ積み、存在しないフィールドには触れない(以前の複数 db.Exec と同じ部分更新の意味論を維持)。
func (s *Server) handlePatchWork(w http.ResponseWriter, r *http.Request) {
	workID, err := parseWorkID(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "不正な ID")
		return
	}

	var body struct {
		Title    *string `json:"title"`
		Circle   *string `json:"circle"`
		Hidden   *bool   `json:"hidden"`   // 非表示フラグ。true→非表示、false→可視
		Favorite *bool   `json:"favorite"` // お気に入りフラグ。true→登録、false→解除
		Rating   *int    `json:"rating"`   // 1〜5。null は評価解除
	}
	raw := map[string]json.RawMessage{}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&raw); err != nil {
		respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
		return
	}
	for key, value := range raw {
		switch key {
		case "title":
			if err := json.Unmarshal(value, &body.Title); err != nil {
				respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
				return
			}
		case "circle":
			if err := json.Unmarshal(value, &body.Circle); err != nil {
				respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
				return
			}
		case "hidden":
			if err := json.Unmarshal(value, &body.Hidden); err != nil {
				respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
				return
			}
		case "favorite":
			if err := json.Unmarshal(value, &body.Favorite); err != nil {
				respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
				return
			}
		case "rating":
			if string(value) != "null" {
				if err := json.Unmarshal(value, &body.Rating); err != nil {
					respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
					return
				}
			}
		}
	}

	// title: TrimSpace 後に空文字になる場合は 400(作品名を空にできてしまう
	// バグの修正)。長さも緩く上限を設ける。
	var trimmedTitle string
	if body.Title != nil {
		trimmedTitle = strings.TrimSpace(*body.Title)
		if trimmedTitle == "" {
			respondError(w, http.StatusBadRequest, "作品名を空にすることはできません")
			return
		}
		if utf8.RuneCountInString(trimmedTitle) > maxTitleRunes {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("作品名は%d文字以内で指定してください", maxTitleRunes))
			return
		}
	}

	// circle: 空文字は「サークル情報の削除」として NULL 化を許す。circle は元々
	// NULL 許容カラムで、CSV インポート側(nullIfEmpty)も空文字を NULL に揃えている。
	var trimmedCircle string
	var circleClear bool
	if body.Circle != nil {
		trimmedCircle = strings.TrimSpace(*body.Circle)
		if trimmedCircle == "" {
			circleClear = true
		} else if utf8.RuneCountInString(trimmedCircle) > maxCircleRunes {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("サークル名は%d文字以内で指定してください", maxCircleRunes))
			return
		}
	}
	if body.Rating != nil && (*body.Rating < 1 || *body.Rating > 5) {
		respondError(w, http.StatusBadRequest, "評価は1〜5で指定してください")
		return
	}

	// 存在確認(DB エラーは 500、不存在は 404 を返す)
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM works WHERE id=?)", workID).Scan(&exists); err != nil {
		respondInternalError(w, "DB エラー", err)
		return
	}
	if !exists {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}

	// 動的 SET 句の組み立て。JSON にキーが存在するフィールドのみ積む。
	setClauses := make([]string, 0, 5)
	args := make([]any, 0, 5)

	if body.Title != nil {
		setClauses = append(setClauses, "title=?")
		args = append(args, trimmedTitle)
	}
	if body.Circle != nil {
		if circleClear {
			setClauses = append(setClauses, "circle=NULL")
		} else {
			setClauses = append(setClauses, "circle=?")
			args = append(args, trimmedCircle)
		}
	}
	if body.Hidden != nil {
		// bool を INTEGER(0/1) に変換して保存
		hiddenVal := 0
		if *body.Hidden {
			hiddenVal = 1
		}
		setClauses = append(setClauses, "hidden=?")
		args = append(args, hiddenVal)
	}
	if body.Favorite != nil {
		// true → 現在時刻を記録(登録順ソートに使う)、false → NULL に戻す
		if *body.Favorite {
			setClauses = append(setClauses, "favorited_at=datetime('now')")
		} else {
			setClauses = append(setClauses, "favorited_at=NULL")
		}
	}
	if _, ok := raw["rating"]; ok {
		if body.Rating == nil {
			setClauses = append(setClauses, "rating=NULL")
		} else {
			setClauses = append(setClauses, "rating=?")
			args = append(args, *body.Rating)
		}
	}
	// Title または Circle が非 nil のときだけ manually_edited=1 を立てる
	// (hidden/favorite のみの PATCH では立てない。issue #64 案 A の挙動を維持)。
	if body.Title != nil || body.Circle != nil {
		setClauses = append(setClauses, "manually_edited=1")
	}

	// SET 句が1つも無い(空 body 等)場合は何もせず 204 を返す(以前からの挙動を維持)。
	if len(setClauses) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	setClauses = append(setClauses, "updated_at=datetime('now')")
	args = append(args, workID)
	query := "UPDATE works SET " + strings.Join(setClauses, ", ") + " WHERE id=?"
	if _, err := s.db.Exec(query, args...); err != nil {
		respondInternalError(w, "更新失敗", err)
		return
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
	// タグ名は TrimSpace 後に空文字、または 100 rune 超過なら 400。
	name := strings.TrimSpace(body.Name)
	const maxTagNameRunes = 100
	if name == "" || utf8.RuneCountInString(name) > maxTagNameRunes {
		respondError(w, http.StatusBadRequest, "タグ名は1〜100文字で指定してください")
		return
	}

	// 作品存在確認(DB エラーは 500、不存在は 404 を返す)
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM works WHERE id=?)", workID).Scan(&exists); err != nil {
		respondInternalError(w, "DB エラー", err)
		return
	}
	if !exists {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}

	// custom タグ upsert
	if _, err := s.db.Exec(
		"INSERT INTO tags (name, category) VALUES (?, 'custom') ON CONFLICT(name, category) DO NOTHING",
		name,
	); err != nil {
		respondInternalError(w, "タグ作成失敗", err)
		return
	}
	var tagID int64
	if err := s.db.QueryRow(
		"SELECT id FROM tags WHERE name=? AND category='custom'", name,
	).Scan(&tagID); err != nil {
		respondInternalError(w, "タグ ID 取得失敗", err)
		return
	}

	if _, err := s.db.Exec(
		"INSERT INTO work_tags (work_id, tag_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
		workID, tagID,
	); err != nil {
		respondInternalError(w, "タグリンク失敗", err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{"id": tagID, "name": name, "category": "custom"})
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
		respondInternalError(w, "タグ削除失敗", err)
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
		respondInternalError(w, "DB 取得失敗", err)
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
		respondInternalError(w, "ファイルオープン失敗", err)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		respondInternalError(w, "Stat 失敗", err)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	// ブラウザキャッシュを 1 時間効かせる(http.ServeContent が付ける Last-Modified で
	// 更新検知できる)。将来の認証導入を見据えて private にする。
	w.Header().Set("Cache-Control", "private, max-age=3600")
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
		thumbnailPath  sql.NullString
	)
	if err := s.db.QueryRow(
		"SELECT root_path, thumb_checked_at, thumbnail_path FROM works WHERE id=?", workID,
	).Scan(&rootPath, &thumbCheckedAt, &thumbnailPath); err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	} else if err != nil {
		respondInternalError(w, "DB 取得失敗", err)
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

	regenerated, outPath, candidateFound, err := gen.Refresh(workID, rootPath.String)
	if err != nil {
		respondInternalError(w, "サムネイル再生成失敗", err)
		return
	}

	if !candidateFound && thumbnailPath.Valid {
		if _, uErr := s.db.Exec(
			"UPDATE works SET thumbnail_path=NULL, updated_at=datetime('now') WHERE id=?",
			workID,
		); uErr != nil {
			respondInternalError(w, "thumbnail_path クリア失敗", uErr)
			return
		}
	} else if regenerated && outPath != "" {
		if _, uErr := s.db.Exec(
			"UPDATE works SET thumbnail_path=?, updated_at=datetime('now') WHERE id=?",
			outPath, workID,
		); uErr != nil {
			respondInternalError(w, "thumbnail_path 更新失敗", uErr)
			return
		}
	}

	// 再生成の有無に関わらず、チェックを実行した時刻を記録してスロットルを有効化する
	if _, uErr := s.db.Exec(
		"UPDATE works SET thumb_checked_at=datetime('now') WHERE id=?", workID,
	); uErr != nil {
		respondInternalError(w, "thumb_checked_at 更新失敗", uErr)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"refreshed": regenerated})
}

// rebuildWorkEntry は一括再生成対象の作品(id, root_path)。
type rebuildWorkEntry struct {
	id       int64
	rootPath string
}

// handleRebuildThumbnails は POST /api/thumbnails/rebuild を処理する。
// root_path がある全作品に対して mtime 判定付きサムネイル再生成を worker pool で並列実行する。
// scanMu を TryLock できない場合(スキャンや別の一括再生成と競合)は 409 を返す。
// スキャンと同一の作品群を触るため scanMu を共有する(専用ロックにはしない)。
//
// 大規模ライブラリでは全作品の walk + 画像デコードに数分かかり、同期実行だと
// プロキシ/ブラウザのタイムアウトでレスポンスを受け取れなくなる(issue #55)。
// そのため作品一覧の取得と total 確定までは同期で行い、実際の worker pool 実行と
// DB 更新は goroutine に委譲して 202 Accepted を即座に返す。進捗は
// GET /api/thumbnails/rebuild/status でポーリングする。
func (s *Server) handleRebuildThumbnails(w http.ResponseWriter, r *http.Request) {
	if !s.tryLockScan(w) {
		return
	}

	// root_path がある作品一覧を取得(total 確定まではリクエスト処理内で同期に行う)
	rows, err := s.db.Query("SELECT id, root_path FROM works WHERE root_path IS NOT NULL")
	if err != nil {
		s.scanMu.Unlock()
		respondInternalError(w, "作品一覧取得失敗", err)
		return
	}

	var works []rebuildWorkEntry
	for rows.Next() {
		var we rebuildWorkEntry
		if err := rows.Scan(&we.id, &we.rootPath); err != nil {
			rows.Close()
			s.scanMu.Unlock()
			respondInternalError(w, "行読み込み失敗", err)
			return
		}
		works = append(works, we)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		s.scanMu.Unlock()
		respondInternalError(w, "クエリエラー", err)
		return
	}

	s.rebuildProgress.start(len(works))

	// worker pool 実行と DB 更新はリクエスト goroutine をブロックしないよう別 goroutine に
	// 委譲する。scanMu の解放と進捗の終了処理は runRebuildThumbnailsJob 側の defer で行う。
	go s.runRebuildThumbnailsJob(works)

	respondJSON(w, http.StatusAccepted, s.rebuildProgress.snapshot())
}

// runRebuildThumbnailsJob は worker pool でサムネイルを並列再生成し、結果を DB に反映する。
// handleRebuildThumbnails が TryLock した scanMu をここで解放し、進捗を finish する。
func (s *Server) runRebuildThumbnailsJob(works []rebuildWorkEntry) {
	// パニックで scanMu が永久に取得されたままになるのを防ぐ(goroutine 内の未回収
	// panic はプロセス全体を落とすため recover 自体も必須)。他の defer(Unlock・
	// finish)より後に実行されるよう先頭で defer しておく。
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("サムネイル一括再生成ジョブがパニックしました: %v", rec)
		}
	}()
	defer s.scanMu.Unlock()
	defer s.rebuildProgress.finish()

	thumbsDir := filepath.Join(s.cfg.DataDir, "thumbs")
	gen := thumb.New(thumbsDir)

	type result struct {
		id          int64
		outPath     string
		regenerated bool
		found       bool
		err         error
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobs := make(chan rebuildWorkEntry, len(works))
	results := make(chan result, len(works))

	// ワーカー起動
	for i := 0; i < numWorkers; i++ {
		go func() {
			for we := range jobs {
				regen, outPath, found, err := gen.Refresh(we.id, we.rootPath)
				results <- result{id: we.id, outPath: outPath, regenerated: regen, found: found, err: err}
			}
		}()
	}

	// ジョブ投入
	for _, we := range works {
		jobs <- we
	}
	close(jobs)

	// 結果のドレイン中は DB に一切触れない。画像デコードは 1 件あたり数十 ms〜
	// かかることがあり、全件で数分規模になり得る。ここで tx を張ったまま drain すると
	// SQLite の書き込みロックがジョブ全体の間保持され、scanMu 外の他の書き込み API
	// (plays/PATCH/タグ/履歴削除/CSV インポート等)が busy_timeout 後に 500 になって
	// しまう(PR #79 レビュー指摘)。そのため drain 中は進捗更新(addChecked)と
	// (id, outPath) の収集のみ行い、DB 更新は全件ドレイン後にまとめて行う。
	type update struct {
		id      int64
		outPath string
	}
	var toUpdate []update
	var toClear []int64

	for range works {
		res := <-results
		if res.err != nil {
			// ログには出力するが全体は継続
			log.Printf("サムネイル再生成失敗 work_id=%d: %v", res.id, res.err)
			s.rebuildProgress.addChecked(1)
			continue
		}
		if !res.found {
			toClear = append(toClear, res.id)
		} else if res.regenerated && res.outPath != "" {
			toUpdate = append(toUpdate, update{id: res.id, outPath: res.outPath})
		}
		s.rebuildProgress.addChecked(1)
	}

	if len(toUpdate) == 0 && len(toClear) == 0 {
		return
	}

	// 全件ドレイン後、短い tx でまとめて UPDATE → Commit する。
	// UPDATE をまとめて 1 トランザクションにすることで、作品数分の fsync を避ける。
	// tx が取得できない場合は s.db.Exec への個別コミットにフォールバックする。
	//
	// execer はここでのみ使う最小限のインターフェース(*sql.DB と *sql.Tx の両方を満たす)。
	type execer interface {
		Exec(query string, args ...any) (sql.Result, error)
	}
	var execTarget execer = s.db
	tx, txErr := s.db.Begin()
	if txErr != nil {
		log.Printf("サムネイル一括再生成: トランザクション開始失敗、個別コミットにフォールバックします: %v", txErr)
	} else {
		execTarget = tx
		// Commit 済みの tx に対する Rollback は何もせずエラーを返すだけなので、
		// 正常系の Commit 後に無条件で defer しても問題ない(早期 return・panic からの保護)。
		defer func() { _ = tx.Rollback() }()
	}

	for _, id := range toClear {
		if _, uErr := execTarget.Exec(
			"UPDATE works SET thumbnail_path=NULL, updated_at=datetime('now') WHERE id=? AND thumbnail_path IS NOT NULL",
			id,
		); uErr != nil {
			log.Printf("thumbnail_path クリア失敗 work_id=%d: %v", id, uErr)
		}
	}

	for _, u := range toUpdate {
		if _, uErr := execTarget.Exec(
			"UPDATE works SET thumbnail_path=?, updated_at=datetime('now') WHERE id=?",
			u.outPath, u.id,
		); uErr != nil {
			log.Printf("thumbnail_path 更新失敗 work_id=%d: %v", u.id, uErr)
		} else {
			s.rebuildProgress.addRegenerated(1)
		}
	}

	if tx != nil {
		if cErr := tx.Commit(); cErr != nil {
			log.Printf("サムネイル一括再生成: トランザクションのコミット失敗: %v", cErr)
		}
	}
}

// handleRebuildThumbnailsStatus は GET /api/thumbnails/rebuild/status を処理する。
// 一度も一括再生成を実行していない場合は zero value のスナップショットを 200 で返す。
func (s *Server) handleRebuildThumbnailsStatus(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, s.rebuildProgress.snapshot())
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
		respondInternalError(w, "DB エラー", err)
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
		respondInternalError(w, "パス解決失敗", resolveErr)
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
		respondInternalError(w, "ディレクトリ読み込み失敗", err)
		return
	}

	// ディレクトリとファイルに分けて、それぞれ自然順ソート
	var dirs, files []entryItem
	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			// 壊れた symlink・ReadDir と Info の間に消えたファイル等。一覧からは
			// 除外する(従来どおり)が、静かに消えると原因調査が難しいためログに残す(issue #70)。
			log.Printf("エントリ情報取得失敗のためスキップ: %s: %v", filepath.Join(resolvedPath, e.Name()), err)
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

	respondJSON(w, http.StatusOK, entriesListResponse{
		Path:    relPath,
		Parent:  parent,
		Entries: result,
	})
}

// entriesListResponse は GET /api/works/{id}/entries のレスポンス。
// このエンドポイントに NULL 許容フィールドは無いため純粋な typed struct 化のみ
// (契約は変わらない。issue #38-2)。
type entriesListResponse struct {
	Path    string      `json:"path"`
	Parent  string      `json:"parent"`
	Entries []entryItem `json:"entries"`
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
		respondInternalError(w, "DB エラー", err)
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
		respondInternalError(w, "パス解決失敗", resolveErr)
		return
	}

	f, err := os.Open(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "ファイルが見つかりません")
			return
		}
		respondInternalError(w, "ファイルオープン失敗", err)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		respondInternalError(w, "Stat 失敗", err)
		return
	}
	if st.IsDir() {
		respondError(w, http.StatusBadRequest, "ディレクトリは配信できません")
		return
	}

	// Content-Type を拡張子から決定。media.MimeByExt の明示テーブルを優先し
	// (mime.TypeByExtension は環境依存で distroless 等では .flac 等が外れることがある)、
	// 対応外の拡張子のみ mime.TypeByExtension にフォールバックする。
	ct := media.MimeByExt(resolvedPath)
	if ct == "" {
		ct = mime.TypeByExtension(filepath.Ext(resolvedPath))
	}
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

// maxPlayPathBytes は再生記録 path の長さ上限(byte 数)。DB 肥大化・異常系入力の
// 防止が目的の緩い上限で、実際のファイルパスがこれを超えることは想定しない(issue #63)。
const maxPlayPathBytes = 1000

// handleRecordPlay は POST /api/works/{id}/plays を処理する。
//
// path は media.ResolvePath(handleWorkFile 等と同じパストラバーサル検証)で
// 作品ルート配下に実在することを確認してから記録する。ブラウズ系エンドポイントは
// トラバーサル検出を 403 で返すが、本エンドポイントは「不正な再生記録リクエスト」
// として 400 に、対象ファイル不在は 404 にマップする(issue #63 の仕様に合わせた設計判断)。
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
	if len(body.Path) > maxPlayPathBytes {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("path は%dバイト以内で指定してください", maxPlayPathBytes))
		return
	}

	// 作品のルートフォルダを取得(不存在は 404、DB エラーは 500)。
	rootPath, err := s.getWorkRootPath(workID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "作品が見つかりません")
		return
	}
	if err != nil {
		respondInternalError(w, "DB エラー", err)
		return
	}
	if rootPath == "" {
		// root_path が NULL(フォルダ未登録)の作品はパスの実在を検証できないため拒否する。
		respondError(w, http.StatusNotFound, "作品フォルダが登録されていません")
		return
	}

	// パストラバーサル検証。ErrForbidden→400、ErrNotFound→404 にマップする
	// (handleWorkFile 等のブラウズ系は 403/404 だが、再生記録はここでは 400/404 とする)。
	if _, resolveErr := media.ResolvePath(rootPath, body.Path); resolveErr == media.ErrForbidden {
		respondError(w, http.StatusBadRequest, "不正な path です")
		return
	} else if resolveErr == media.ErrNotFound {
		respondError(w, http.StatusNotFound, "ファイルが見つかりません")
		return
	} else if resolveErr != nil {
		respondInternalError(w, "パス解決失敗", resolveErr)
		return
	}

	if _, err := s.db.Exec(
		"INSERT INTO play_history (work_id, file_path) VALUES (?, ?)",
		workID, body.Path,
	); err != nil {
		respondInternalError(w, "履歴記録失敗", err)
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

// nullableString は sql.NullString を *string に変換する。NULL(Valid=false)なら nil を
// 返し、typed struct 経由で JSON エンコードした際にキー自体は残したまま値だけ `null` になる
// (以前の setIfValid はキーそのものを省略していたが、それを廃止した。issue #57/#38-2)。
func nullableString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

// nullableInt は sql.NullInt64 を *int に変換する。rating など小さい整数値用。
func nullableInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

// strPtr は string の値から *string を作る小さなヘルパー(複合リテラルの & を避けるため)。
func strPtr(s string) *string {
	return &s
}
