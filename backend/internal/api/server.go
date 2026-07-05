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

	// rebuildProgress はサムネイル一括再生成ジョブ(POST /api/thumbnails/rebuild)の
	// 進捗。ジョブ実行は goroutine に委譲されるため(issue #55)、GET
	// /api/thumbnails/rebuild/status から並行に読み取られる。
	rebuildProgress rebuildProgress
}

// rebuildStatusSnapshot は進捗のスナップショット。POST /api/thumbnails/rebuild(202)
// と GET /api/thumbnails/rebuild/status の両方でこの形を返す。
type rebuildStatusSnapshot struct {
	Running     bool `json:"running"`
	Checked     int  `json:"checked"`
	Regenerated int  `json:"regenerated"`
	Total       int  `json:"total"`
}

// rebuildProgress はサムネイル一括再生成ジョブの進捗。ジョブ goroutine と
// GET /api/thumbnails/rebuild/status から並行アクセスされるため mu で保護する。
type rebuildProgress struct {
	mu          sync.Mutex
	running     bool
	checked     int
	regenerated int
	total       int
}

// snapshot は現在の進捗を JSON レスポンス用構造体としてコピーする。
func (p *rebuildProgress) snapshot() rebuildStatusSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return rebuildStatusSnapshot{
		Running:     p.running,
		Checked:     p.checked,
		Regenerated: p.regenerated,
		Total:       p.total,
	}
}

// start はジョブ開始時に呼び出し、カウンタをリセットして running=true にする。
func (p *rebuildProgress) start(total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = true
	p.checked = 0
	p.regenerated = 0
	p.total = total
}

// addChecked は checked カウンタに n を加算する。
func (p *rebuildProgress) addChecked(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.checked += n
}

// addRegenerated は regenerated カウンタに n を加算する。
func (p *rebuildProgress) addRegenerated(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.regenerated += n
}

// finish はジョブ終了時に呼び出し、running=false にする(カウンタは最終値のまま残す)。
func (p *rebuildProgress) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = false
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

		// ユーザー付与メタデータのエクスポート/インポート(バックアップ・復元用。issue #78)
		r.Get("/export", s.handleExportMetadata)
		r.Post("/import/metadata", s.handleImportMetadata)

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

		// サムネイル一括再生成(非同期。進捗は status で確認する)
		r.Post("/thumbnails/rebuild", s.handleRebuildThumbnails)
		r.Get("/thumbnails/rebuild/status", s.handleRebuildThumbnailsStatus)

		// タグ
		r.Get("/tags", s.handleListTags)
		r.Post("/tags/cleanup", s.handleCleanupTags)

		// サークル
		r.Get("/circles", s.handleListCircles)

		// 再生履歴
		r.Get("/history", s.handleHistory)
		r.Delete("/history", s.handleDeleteHistory)
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

// respondInternalError は 500 系のレスポンスを返す。
// err.Error() をそのままクライアントに返すと内部実装の詳細(SQL・ファイルパス等)が
// 漏れるため、クライアントには固定の日本語メッセージのみを返し、詳細は log.Printf で
// サーバログにだけ残す(issue #70)。400/404 系は操作ヒントとして err.Error() を
// そのまま返す方針を維持しており、本ヘルパーは 500 系のみに使うこと。
func respondInternalError(w http.ResponseWriter, msg string, err error) {
	log.Printf("%s: %v", msg, err)
	respondError(w, http.StatusInternalServerError, msg)
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
