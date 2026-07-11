package api

import (
	"context"
	"errors"
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
		// 全ルート不読(NAS 未マウント等)はユーザーが対処できる一時障害なので、
		// 固定の 500 文言に丸めず具体的なメッセージを 503 で返す(パス等の内部情報は含まれない)
		if errors.Is(err, scanner.ErrAllRootsUnreadable) {
			respondError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondInternalError(w, "スキャン失敗", err)
		return
	}

	respondJSON(w, http.StatusOK, res)
}

// StartStartupScan は起動時自動スキャン(STASHPAD_SCAN_ON_START)を jobsWG に
// 登録してバックグラウンドで開始する。main.go から呼ばれる想定
// (シャットダウン時に CancelJobs/WaitJobs で安全に畳めるよう startJob 経由にする。issue #83)。
func (s *Server) StartStartupScan() {
	s.startJob(s.RunStartupScan)
}

// RunStartupScan は起動時自動スキャン(STASHPAD_SCAN_ON_START)の本体。
// StartStartupScan から jobsWG 登録済みの goroutine として呼ばれる想定。
// 起動時はブロッキング Lock でよい(他の経路と競合していれば完了を待ってから実行する)。
func (s *Server) RunStartupScan() {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	thumbsDir := filepath.Join(s.cfg.DataDir, "thumbs")
	gen := thumb.New(thumbsDir)

	res, err := scanner.ScanContext(s.jobCtx, s.db, s.cfg.LibraryRoots, gen)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("起動時スキャン中断(シャットダウン): %v", err)
			return
		}
		log.Printf("起動時スキャン失敗: %v", err)
		return
	}
	log.Printf("起動時スキャン完了: found=%d new=%d linked=%d missing=%d",
		res.WorksFound, res.NewlyRegistered, res.LinkedToCSV, res.MissingMarked)
}
