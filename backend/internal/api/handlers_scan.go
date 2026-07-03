package api

import (
	"log"
	"net/http"
	"path/filepath"

	"github.com/koji-genba/stashpad/backend/internal/scanner"
	"github.com/koji-genba/stashpad/backend/internal/thumb"
)

// handleScan は POST /api/scan を処理する。
// ライブラリスキャンを同期実行し、結果サマリを返す。
// scanMu を TryLock できない場合(サムネイル一括再生成や起動時スキャンと競合)は 409 を返す。
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if !s.tryLockScan(w) {
		return
	}
	defer s.scanMu.Unlock()

	thumbsDir := filepath.Join(s.cfg.DataDir, "thumbs")
	gen := thumb.New(thumbsDir)

	res, err := scanner.Scan(s.db, s.cfg.LibraryRoots, gen)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "スキャン失敗: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, res)
}

// RunStartupScan は起動時自動スキャン(STASHPAD_SCAN_ON_START)を実行する。
// main.go から go srv.RunStartupScan() として呼ばれる想定。
// 起動時はブロッキング Lock でよい(他の経路と競合していれば完了を待ってから実行する)。
func (s *Server) RunStartupScan() {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	thumbsDir := filepath.Join(s.cfg.DataDir, "thumbs")
	gen := thumb.New(thumbsDir)

	res, err := scanner.Scan(s.db, s.cfg.LibraryRoots, gen)
	if err != nil {
		log.Printf("起動時スキャン失敗: %v", err)
		return
	}
	log.Printf("起動時スキャン完了: found=%d new=%d linked=%d missing=%d",
		res.WorksFound, res.NewlyRegistered, res.LinkedToCSV, res.MissingMarked)
}
