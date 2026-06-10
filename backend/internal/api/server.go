// Package api は HTTP ハンドラと chi ルーターを提供する。
// 全ルートは単一のミドルウェアチェーン経由とし、後から認証等を挟めるようにする。
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/koji-genba/stashpad/backend/internal/config"
	"github.com/koji-genba/stashpad/backend/internal/web"
)

// Server は依存オブジェクトを保持し、ハンドラをメソッドとして提供する。
type Server struct {
	db  *sql.DB
	cfg *config.Config
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
			r.Get("/thumbnail", s.handleWorkThumbnail)
			r.Get("/entries", s.handleWorkEntries)
			r.Get("/file", s.handleWorkFile)
			r.Post("/tags", s.handleAddTag)
			r.Delete("/tags/{tag_id}", s.handleDeleteTag)
			r.Post("/plays", s.handleRecordPlay)
		})

		// タグ
		r.Get("/tags", s.handleListTags)

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
	json.NewEncoder(w).Encode(v)
}

// respondError はエラーレスポンスを返す。
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// handleHealthz は GET /api/healthz を処理する。
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
