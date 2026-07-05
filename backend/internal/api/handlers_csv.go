package api

import (
	"errors"
	"net/http"

	"github.com/koji-genba/stashpad/backend/internal/csvimport"
)

// maxCSVUploadBytes は POST /api/import/csv が受け付けるリクエストボディの上限。
// CSV インポートは想定サイズが小さいため、不正/巨大なアップロードでメモリを
// 圧迫しないよう上限を設ける(issue #70)。
const maxCSVUploadBytes = 32 << 20 // 32MB

// handleImportCSV は POST /api/import/csv を処理する。
// multipart フォームで CSV ファイルを受け取り、upsert 結果を返す。
func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	// リクエストボディに上限を設ける。超過すると以降の Body 読み取り
	// (ParseMultipartForm 内部)が http.MaxBytesError を返す(issue #70)。
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVUploadBytes)

	// 最大 32MB まで受け付ける
	if err := r.ParseMultipartForm(maxCSVUploadBytes); err != nil {
		status, msg := classifyMultipartParseError(err)
		respondError(w, status, msg)
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
		respondInternalError(w, "CSV インポート失敗", err)
		return
	}

	respondJSON(w, http.StatusOK, res)
}

// classifyMultipartParseError は ParseMultipartForm のエラーを HTTP ステータスに分類する。
// http.MaxBytesReader によるボディサイズ超過(*http.MaxBytesError)は 413、
// それ以外の multipart パースエラーは従来通り 400 として扱う。
func classifyMultipartParseError(err error) (status int, msg string) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge, "アップロードサイズが大きすぎます: " + err.Error()
	}
	return http.StatusBadRequest, "multipart パース失敗: " + err.Error()
}
