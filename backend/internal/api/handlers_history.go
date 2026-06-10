package api

import (
	"database/sql"
	"net/http"
	"strconv"
)

// handleHistory は GET /api/history を処理する。
// 再生履歴を作品単位でグルーピングし、最終再生日時の降順で返す。
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page := parseIntParam(pageStr, 1)
	limit := 40
	offset := (page - 1) * limit

	// 作品単位でグルーピングし、最終再生日時・最終ファイルパス・再生回数を集計
	query := `
		SELECT
			w.id,
			w.title,
			w.thumbnail_path,
			MAX(ph.played_at) AS last_played_at,
			ph.file_path AS last_file_path,
			COUNT(ph.id) AS play_count
		FROM play_history ph
		JOIN works w ON w.id=ph.work_id
		GROUP BY w.id
		ORDER BY last_played_at DESC
		LIMIT ? OFFSET ?`

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "履歴取得失敗: "+err.Error())
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var (
			workID       int64
			workTitle    string
			thumbPath    sql.NullString
			lastPlayedAt string
			lastFilePath string
			playCount    int
		)
		if err := rows.Scan(&workID, &workTitle, &thumbPath, &lastPlayedAt, &lastFilePath, &playCount); err != nil {
			respondError(w, http.StatusInternalServerError, "行スキャン失敗: "+err.Error())
			return
		}

		workObj := map[string]any{
			"id":    workID,
			"title": workTitle,
		}
		if thumbPath.Valid {
			workObj["thumbnail_url"] = "/api/works/" + itoa(workID) + "/thumbnail"
		}

		items = append(items, map[string]any{
			"work":           workObj,
			"last_played_at": lastPlayedAt,
			"last_file_path": lastFilePath,
			"play_count":     playCount,
		})
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "行読み込み失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"page":  page,
	})
}

// itoa は int64 を文字列に変換する。
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
