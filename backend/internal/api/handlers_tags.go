package api

import (
	"net/http"
	"strconv"
	"strings"
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
		whereClause += " AND t.name LIKE ?"
		args = append(args, "%"+keyword+"%")
	}

	query := `
		SELECT t.id, t.name, t.category, COUNT(wt.work_id) AS work_count
		FROM tags t
		LEFT JOIN work_tags wt ON wt.tag_id=t.id
		WHERE 1=1` + whereClause + `
		GROUP BY t.id
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

// splitCSV はカンマ区切り文字列を分割してトリムしたスライスを返す。
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
