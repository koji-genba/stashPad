package api

import (
	"net/http"
)

// tagFacetItem は GET /api/tags の items 要素。
type tagFacetItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	WorkCount int    `json:"work_count"`
}

// tagsListResponse は GET /api/tags のレスポンス。
type tagsListResponse struct {
	Items []tagFacetItem `json:"items"`
}

// tagCleanupResult は POST /api/tags/cleanup のレスポンス。
type tagCleanupResult struct {
	Deleted int64 `json:"deleted"`
}

// handleListTags は GET /api/tags を処理する。
// ?category= でカテゴリ絞り込み、?q= でタグ名部分一致、作品数付きで返す。
// ?limit= で件数上限を指定できる(未指定なら全件。1〜1000 にクランプ。issue #38-3)。
func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := q.Get("category")
	keyword := q.Get("q")

	args := []any{}
	whereClause := ""

	if category != "" {
		whereClause += " AND t.category=?"
		args = append(args, category)
	}
	if keyword != "" {
		whereClause += " AND t.name LIKE ? ESCAPE '\\'"
		args = append(args, likeContains(keyword))
	}

	// 可視作品(hidden=0)のみカウント対象とする。
	// LEFT JOIN works で hidden=0 を条件に付けることで、
	// 非表示作品しか持たないタグは HAVING COUNT(w.id) > 0 で除外される。
	query := `
		SELECT t.id, t.name, t.category, COUNT(w.id) AS work_count
		FROM tags t
		LEFT JOIN work_tags wt ON wt.tag_id=t.id
		LEFT JOIN works w ON w.id=wt.work_id AND w.hidden=0
		WHERE 1=1` + whereClause + `
		GROUP BY t.id
		HAVING COUNT(w.id) > 0
		ORDER BY work_count DESC, t.name ASC`
	if limit, ok := parseLimitParam(q.Get("limit")); ok {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, "タグ一覧取得失敗", err)
		return
	}
	defer rows.Close()

	items := make([]tagFacetItem, 0)
	for rows.Next() {
		var id int64
		var name, cat string
		var workCount int
		if err := rows.Scan(&id, &name, &cat, &workCount); err != nil {
			respondInternalError(w, "行スキャン失敗", err)
			return
		}
		items = append(items, tagFacetItem{ID: id, Name: name, Category: cat, WorkCount: workCount})
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, "行読み込み失敗", err)
		return
	}

	respondJSON(w, http.StatusOK, tagsListResponse{Items: items})
}

// handleCleanupTags は POST /api/tags/cleanup を処理する。
// work_tags に 1 件も紐付かないタグを物理削除し、削除件数を返す。
func (s *Server) handleCleanupTags(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.Exec(
		`DELETE FROM tags WHERE id NOT IN (SELECT DISTINCT tag_id FROM work_tags)`,
	)
	if err != nil {
		respondInternalError(w, "タグクリーンアップ失敗", err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		respondInternalError(w, "件数取得失敗", err)
		return
	}
	respondJSON(w, http.StatusOK, tagCleanupResult{Deleted: n})
}
