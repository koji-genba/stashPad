// Package main は stashPad バックエンドのエントリポイント。
// 設定読み込み → DB オープン → マイグレーション → HTTP サーバ起動 の順で初期化する。
package main

import (
	"context"
	"log"
	"net"
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

// defaultHealthzPort は STASHPAD_ADDR 未設定時のヘルスチェック用ポート。
// config.Load() のデフォルト(":8080")と整合させること。
const defaultHealthzPort = "8080"

// healthzURL は STASHPAD_ADDR 形式のリッスンアドレス(例 ":8080" や
// "0.0.0.0:8080")から、自プロセスへのヘルスチェック用 URL を組み立てる。
// ホスト部分は無視し、常に localhost 経由で接続する
// (コンテナ内から自分自身に接続するだけなので、バインドアドレスがどうであれ
// localhost で到達できる)。
func healthzURL(addr string) string {
	port := defaultHealthzPort
	if addr != "" {
		if _, p, err := net.SplitHostPort(addr); err == nil && p != "" {
			port = p
		}
	}
	return "http://localhost:" + port + "/api/healthz"
}

// runHealthcheck は `-healthcheck` 引数で起動された場合の処理本体。
// Docker HEALTHCHECK から呼ばれ、GET /api/healthz が 200 を返せば正常(exit 0)、
// それ以外(接続失敗・非 200)は異常(exit 1)とみなす。
func runHealthcheck() int {
	url := healthzURL(os.Getenv("STASHPAD_ADDR"))
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func main() {
	// Docker HEALTHCHECK 用: config.Load() より前に処理し、
	// 必須環境変数(STASHPAD_LIBRARY_ROOTS 等)の検証に影響されないようにする。
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(runHealthcheck())
	}

	// 設定読み込み
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("設定エラー: %v", err)
	}

	// ライブラリルートの存在検証(issue #70)。設定ミスや NAS 未マウントに
	// 起動時点で気付けるよう警告を出す。起動は止めない(全ルート不存在でも
	// スキャナ側の全滅ガード(issue #48)が DB の全件 NULL 化を防ぐため)。
	for _, warn := range config.CheckLibraryRoots(cfg.LibraryRoots) {
		log.Printf("警告: %s", warn)
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
