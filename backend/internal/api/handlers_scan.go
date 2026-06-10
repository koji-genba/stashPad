package api

import (
	"net/http"
	"path/filepath"

	"github.com/koji-genba/stashpad/backend/internal/scanner"
	"github.com/koji-genba/stashpad/backend/internal/thumb"
)

// handleScan は POST /api/scan を処理する。
// ライブラリスキャンを同期実行し、結果サマリを返す。
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	thumbsDir := filepath.Join(s.cfg.DataDir, "thumbs")
	gen := thumb.New(thumbsDir)

	res, err := scanner.Scan(s.db, s.cfg.LibraryRoots, gen)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "スキャン失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, res)
}
