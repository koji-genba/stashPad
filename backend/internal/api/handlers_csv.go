package api

import (
	"net/http"

	"github.com/koji-genba/stashpad/backend/internal/csvimport"
)

// handleImportCSV は POST /api/import/csv を処理する。
// multipart フォームで CSV ファイルを受け取り、upsert 結果を返す。
func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	// 最大 32MB まで受け付ける
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "multipart パース失敗: "+err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "file フィールドが見つかりません: "+err.Error())
		return
	}
	defer file.Close()

	res, err := csvimport.Import(s.db, file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "CSV インポート失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, res)
}
