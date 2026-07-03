package api

import (
	"net/http"
	"strconv"
)

// handleListTags は GET /api/tags を処理する。
// ?category= でカテゴリ絞り込み、?q= でタグ名部分一致、作品数付きで返す。
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

	rows, err := s.db.Query(query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "タグ一覧取得失敗: "+err.Error())
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var name, cat string
		var workCount int
		if err := rows.Scan(&id, &name, &cat, &workCount); err != nil {
			respondError(w, http.StatusInternalServerError, "行スキャン失敗: "+err.Error())
			return
		}
		items = append(items, map[string]any{
			"id":         id,
			"name":       name,
			"category":   cat,
			"work_count": workCount,
		})
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "行読み込み失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleCleanupTags は POST /api/tags/cleanup を処理する。
// work_tags に 1 件も紐付かないタグを物理削除し、削除件数を返す。
func (s *Server) handleCleanupTags(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.Exec(
		`DELETE FROM tags WHERE id NOT IN (SELECT DISTINCT tag_id FROM work_tags)`,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "タグクリーンアップ失敗: "+err.Error())
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "件数取得失敗: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

// ---- 未使用インポート回避用ヘルパー ------------------------------------------

// parseIntParam はクエリパラメータを整数にパースする(エラー時はデフォルト値を返す)。
func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return defaultVal
	}
	return v
}
