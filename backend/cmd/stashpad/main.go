// Package main は stashPad バックエンドのエントリポイント。
// 設定読み込み → DB オープン → マイグレーション → HTTP サーバ起動 の順で初期化する。
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

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

	// HTTP サーバ起動
	srv := api.New(database, cfg)
	router := srv.Router()

	log.Printf("stashPad 起動: %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, router); err != nil {
		log.Fatalf("サーバ起動失敗: %v", err)
	}
}
