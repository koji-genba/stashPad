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
	// last_file_path は相関サブクエリで最新の play_history を取得する。
	// ph.file_path を直接参照すると GROUP BY のないカラムとなり、SQLite が任意の行を返してしまう。
	// 非表示作品(hidden=1)は JOIN 条件に AND w.hidden=0 を付けることで除外する。
	query := `
		SELECT
			w.id,
			w.title,
			w.thumbnail_path,
			MAX(ph.played_at) AS last_played_at,
			(SELECT ph2.file_path FROM play_history ph2 WHERE ph2.work_id=w.id ORDER BY ph2.played_at DESC LIMIT 1) AS last_file_path,
			COUNT(ph.id) AS play_count
		FROM play_history ph
		JOIN works w ON w.id=ph.work_id AND w.hidden=0
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
		"limit": limit,
	})
}

// itoa は int64 を文字列に変換する。
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
