package api

import "net/http"

// circleFacetItem は GET /api/circles の items 要素。
type circleFacetItem struct {
	Name      string `json:"name"`
	WorkCount int    `json:"work_count"`
}

// circlesListResponse は GET /api/circles のレスポンス。
type circlesListResponse struct {
	Items []circleFacetItem `json:"items"`
}

// handleListCircles は GET /api/circles を処理する。
// サークル名・作品数の一覧を返す。NULL または空文字のサークルは除外する。
// ?q= でサークル名の部分一致絞り込み、?limit= で件数上限を指定できる
// (未指定なら全件。1〜1000 にクランプ。issue #38-3)。
func (s *Server) handleListCircles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keyword := q.Get("q")

	args := []any{}
	// 非表示作品(hidden=1)を除外する。circle が NULL または空文字の作品も除外。
	whereClause := "WHERE circle IS NOT NULL AND circle != '' AND hidden=0"

	if keyword != "" {
		whereClause += " AND circle LIKE ? ESCAPE '\\'"
		args = append(args, likeContains(keyword))
	}

	query := `SELECT circle, COUNT(*) AS work_count
	          FROM works ` + whereClause + `
	          GROUP BY circle
	          ORDER BY work_count DESC, circle ASC`
	if limit, ok := parseLimitParam(q.Get("limit")); ok {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "サークル一覧取得失敗: "+err.Error())
		return
	}
	defer rows.Close()

	items := make([]circleFacetItem, 0)
	for rows.Next() {
		var name string
		var workCount int
		if err := rows.Scan(&name, &workCount); err != nil {
			respondError(w, http.StatusInternalServerError, "行スキャン失敗: "+err.Error())
			return
		}
		items = append(items, circleFacetItem{Name: name, WorkCount: workCount})
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "行読み込み失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, circlesListResponse{Items: items})
}
