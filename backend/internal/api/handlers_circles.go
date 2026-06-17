package api

import "net/http"

// handleListCircles は GET /api/circles を処理する。
// サークル名・作品数の一覧を返す。NULL または空文字のサークルは除外する。
// ?q= でサークル名の部分一致絞り込みが可能。
func (s *Server) handleListCircles(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("q")

	args := []any{}
	// 非表示作品(hidden=1)を除外する。circle が NULL または空文字の作品も除外。
	whereClause := "WHERE circle IS NOT NULL AND circle != '' AND hidden=0"

	if keyword != "" {
		whereClause += " AND circle LIKE ?"
		args = append(args, "%"+keyword+"%")
	}

	query := `SELECT circle, COUNT(*) AS work_count
	          FROM works ` + whereClause + `
	          GROUP BY circle
	          ORDER BY work_count DESC, circle ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "サークル一覧取得失敗: "+err.Error())
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var name string
		var workCount int
		if err := rows.Scan(&name, &workCount); err != nil {
			respondError(w, http.StatusInternalServerError, "行スキャン失敗: "+err.Error())
			return
		}
		items = append(items, map[string]any{
			"name":       name,
			"work_count": workCount,
		})
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "行読み込み失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}
