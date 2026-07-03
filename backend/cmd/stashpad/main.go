// Package main は stashPad バックエンドのエントリポイント。
// 設定読み込み → DB オープン → マイグレーション → HTTP サーバ起動 の順で初期化する。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/koji-genba/stashpad/backend/internal/api"
	"github.com/koji-genba/stashpad/backend/internal/config"
	"github.com/koji-genba/stashpad/backend/internal/db"
)

func main() {
	// 設定読み込み
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("設定エラー: %v", err)
	}

	// data ディレクトリとサムネイルディレクトリを作成
	thumbsDir := filepath.Join(cfg.DataDir, "thumbs")
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		log.Fatalf("thumbs ディレクトリ作成失敗: %v", err)
	}

	// DB オープン(マイグレーション含む)
	dbPath := filepath.Join(cfg.DataDir, "stashpad.db")
	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("DB 初期化失敗: %v", err)
	}
	defer database.Close()

	// HTTP サーバ起動の準備(scanMu を共有するため先に構築する)
	svc := api.New(database, cfg)

	// 起動時自動スキャン(STASHPAD_SCAN_ON_START)。
	// 大規模ライブラリでも起動をブロックしないようバックグラウンドで実行する。
	// POST /api/scan・POST /api/thumbnails/rebuild と scanMu を共有し相互排他する。
	if cfg.ScanOnStart {
		go svc.RunStartupScan()
	}

	router := svc.Router()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		// Range 配信(長時間ストリーミング)があるため WriteTimeout は設定しない
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("サーバ起動失敗: %v", err)
		}
	}()
	log.Printf("stashPad 起動: %s", cfg.Addr)

	<-ctx.Done()
	log.Printf("シグナル受信、シャットダウン中(最大 10 秒)...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown 失敗: %v", err)
	}
}
