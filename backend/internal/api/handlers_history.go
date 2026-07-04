package api

import (
	"database/sql"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// historySortColumns は sort パラメータと ORDER BY 句の対応(ホワイトリスト)。
// 値はサブクエリのカラム名のみで、パラメータ値を SQL に直接埋めない。
var historySortColumns = map[string]string{
	"last_played": "last_played_at",
	"play_count":  "play_count",
}

// handleHistory は GET /api/history を処理する。
// 再生履歴を作品単位でグルーピングし、最終再生日時の降順で返す。
//
// クエリパラメータ:
//   - page   ページ番号(1 始まり)
//   - q      作品タイトルの部分一致フィルタ
//   - sort   last_played(既定) | play_count
//   - order  desc(既定) | asc
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parseIntParam(q.Get("page"), 1)
	if page < 1 {
		page = 1
	}
	limit := 40
	offset := (page - 1) * limit

	keyword := strings.TrimSpace(q.Get("q"))

	// sort/order はホワイトリスト照合のみで SQL に渡す(インジェクション防止)。
	sortCol, ok := historySortColumns[q.Get("sort")]
	if !ok {
		sortCol = "last_played_at"
	}
	order := "DESC"
	if q.Get("order") == "asc" {
		order = "ASC"
	}

	// 作品単位の集計を window 関数で行う。
	// 各 play_history を作品ごとに played_at 降順で番号付け(rn)し、rn=1 の行を採用することで
	// 最終再生ファイル(last_file_path)を相関サブクエリ無しに取得する。
	// COUNT(*) OVER で同一作品の再生回数を、MAX(played_at) OVER で最終再生日時を求める。
	// 非表示作品(hidden=1)は JOIN 条件で除外する。
	// q は作品単位の all-or-nothing なフィルタなので、内側で先に絞っても play_count は変わらない。
	innerWhere := ""
	args := []any{}
	if keyword != "" {
		innerWhere = " AND w.title LIKE ? ESCAPE '\\'"
		args = append(args, likeContains(keyword))
	}

	// total は一覧に出る行の総数(= 履歴を持つ作品数)。メインクエリと同じ JOIN/WHERE
	// (hidden 除外・q フィルタ)を適用した上で作品単位に DISTINCT した件数を数える。
	// play_history の生行数(作品ごとに複数行ありうる)と混同しないよう、メインクエリ側の
	// rn=1 集約と揃えて「作品単位」で数える。
	countQuery := `
		SELECT COUNT(*) FROM (
			SELECT DISTINCT w.id
			FROM play_history ph
			JOIN works w ON w.id=ph.work_id AND w.hidden=0` + innerWhere + `
		)`
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "件数取得失敗: "+err.Error())
		return
	}

	query := `
		SELECT id, title, thumbnail_path, last_played_at, last_file_path, play_count
		FROM (
			SELECT
				w.id              AS id,
				w.title           AS title,
				w.thumbnail_path  AS thumbnail_path,
				ph.file_path      AS last_file_path,
				MAX(ph.played_at) OVER (PARTITION BY w.id) AS last_played_at,
				COUNT(*)          OVER (PARTITION BY w.id) AS play_count,
				ROW_NUMBER()      OVER (PARTITION BY w.id ORDER BY ph.played_at DESC, ph.id DESC) AS rn
			FROM play_history ph
			JOIN works w ON w.id=ph.work_id AND w.hidden=0` + innerWhere + `
		)
		WHERE rn=1
		ORDER BY ` + sortCol + ` ` + order + `, id DESC
		LIMIT ? OFFSET ?`
	// countQuery で args を使い切っているため、limit/offset の追加で裏配列を
	// 破壊しないよう独立スライスへコピーしてから足す。
	dataArgs := append(slices.Clone(args), limit, offset)

	rows, err := s.db.Query(query, dataArgs...)
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
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// itoa は int64 を文字列に変換する。
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// handleDeleteHistory は DELETE /api/history を処理する。
// work_id を指定すると当該作品の履歴のみ削除し、指定がなければ全件削除する。
//
// クエリパラメータ:
//   - work_id  省略可。数値でない場合は 400
func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	workIDStr := strings.TrimSpace(r.URL.Query().Get("work_id"))

	var (
		res sql.Result
		err error
	)
	if workIDStr == "" {
		res, err = s.db.Exec("DELETE FROM play_history")
	} else {
		workID, parseErr := strconv.ParseInt(workIDStr, 10, 64)
		if parseErr != nil {
			respondError(w, http.StatusBadRequest, "不正な work_id")
			return
		}
		res, err = s.db.Exec("DELETE FROM play_history WHERE work_id=?", workID)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "履歴削除失敗: "+err.Error())
		return
	}

	deleted, err := res.RowsAffected()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "件数取得失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}
