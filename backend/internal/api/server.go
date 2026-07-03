// Package api は HTTP ハンドラと chi ルーターを提供する。
// 全ルートは単一のミドルウェアチェーン経由とし、後から認証等を挟めるようにする。
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/koji-genba/stashpad/backend/internal/config"
	"github.com/koji-genba/stashpad/backend/internal/web"
)

// Server は依存オブジェクトを保持し、ハンドラをメソッドとして提供する。
type Server struct {
	db  *sql.DB
	cfg *config.Config

	// scanMu はスキャンとサムネイル一括再生成の相互排他に使う。
	// POST /api/scan・POST /api/thumbnails/rebuild・起動時自動スキャンは
	// 同じ作品群を触るため同一ロックを共有する。TryLock 失敗時は 409 を返す
	// (起動時自動スキャンのみブロッキング Lock でよい)。
	scanMu sync.Mutex
}

// New は Server を生成する。
func New(db *sql.DB, cfg *config.Config) *Server {
	return &Server{db: db, cfg: cfg}
}

// Router は chi.Router を構築して返す。
// 将来の認証ミドルウェアは middlewares スライスに追加するだけでよい構造にしている。
func (s *Server) Router(middlewares ...func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()

	// 標準ミドルウェア
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// 呼び出し元から渡された追加ミドルウェア(認証など)
	for _, m := range middlewares {
		r.Use(m)
	}

	r.Route("/api", func(r chi.Router) {
		// ヘルスチェック
		r.Get("/healthz", s.handleHealthz)

		// スキャン
		r.Post("/scan", s.handleScan)

		// CSV インポート
		r.Post("/import/csv", s.handleImportCSV)

		// 作品
		r.Get("/works", s.handleListWorks)

		r.Route("/works/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetWork)
			r.Patch("/", s.handlePatchWork)
			// thumbnail / file は http.ServeContent を使うため HEAD も受け付ける
			// (プレイヤーが Content-Length を事前取得するケースに対応)
			r.Get("/thumbnail", s.handleWorkThumbnail)
			r.Head("/thumbnail", s.handleWorkThumbnail)
			r.Post("/thumbnail/refresh", s.handleRefreshThumbnail)
			r.Get("/entries", s.handleWorkEntries)
			r.Get("/file", s.handleWorkFile)
			r.Head("/file", s.handleWorkFile)
			r.Post("/tags", s.handleAddTag)
			r.Delete("/tags/{tag_id}", s.handleDeleteTag)
			r.Post("/plays", s.handleRecordPlay)
		})

		// サムネイル一括再生成
		r.Post("/thumbnails/rebuild", s.handleRebuildThumbnails)

		// タグ
		r.Get("/tags", s.handleListTags)
		r.Post("/tags/cleanup", s.handleCleanupTags)

		// サークル
		r.Get("/circles", s.handleListCircles)

		// 再生履歴
		r.Get("/history", s.handleHistory)
	})

	// /api 以外は embed したフロントエンド(SPA フォールバック付き)
	r.Handle("/*", web.Handler())

	return r
}

// respondJSON はレスポンスを JSON で書き出す。
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// ヘッダーは送信済みなのでステータスは変えられない。気付くためにログだけ残す。
		log.Printf("respondJSON: JSON エンコード失敗: %v", err)
	}
}

// respondError はエラーレスポンスを返す。
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// scanConflictMsg はスキャン系処理が競合した際のエラーメッセージ。
// POST /api/scan と POST /api/thumbnails/rebuild で共通のロック・文言を使う。
const scanConflictMsg = "スキャンまたはサムネイル一括再生成が実行中です"

// tryLockScan は scanMu の取得を試み、失敗時は 409 を書き込んで false を返す。
// 呼び出し元は true が返った場合のみ処理を続行し、defer s.scanMu.Unlock() を行うこと。
func (s *Server) tryLockScan(w http.ResponseWriter) bool {
	if !s.scanMu.TryLock() {
		respondError(w, http.StatusConflict, scanConflictMsg)
		return false
	}
	return true
}

// handleHealthz は GET /api/healthz を処理する。
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
