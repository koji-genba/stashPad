package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koji-genba/stashpad/backend/internal/config"
	"github.com/koji-genba/stashpad/backend/internal/db"
)

// ---- スキャン/サムネイル一括再生成の相互排他 (issue #49) -----------------------
//
// POST /api/scan・POST /api/thumbnails/rebuild・起動時自動スキャンの 3 経路は
// 同じ作品群を触るため、Server.scanMu で相互排他する。TryLock 失敗時は
// 409 + {"error": "..."} を返す。

const wantScanConflictMsg = "スキャンまたはサムネイル一括再生成が実行中です"

// testServerAndHandler は newTestServer 相当のセットアップを行うが、
// scanMu を直接 Lock/TryLock できるように *Server も返す。
// (newTestServer は Router() 済みの http.Handler しか返さないため、
// このテストファイル専用に *Server を保持するバリアントを用意する。)
func testServerAndHandler(t *testing.T) (srv *Server, handler http.Handler) {
	t.Helper()
	tmp := t.TempDir()

	database, err := db.Open(filepath.Join(tmp, "test.db"))
	if err != nil {
		t.Fatalf("DB オープン失敗: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	root := filepath.Join(tmp, "RJ000001_テスト作品")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "audio.mp3"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		"INSERT INTO works (rj_number, title, root_path) VALUES ('RJ000001', 'テスト作品', ?)", root,
	); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{LibraryRoots: []string{tmp}, DataDir: tmp, Addr: ":0"}
	srv = New(database, cfg)
	return srv, srv.Router()
}

// scanMu 保持中に POST /api/scan → 409、ボディが {"error": "..."}。
func TestHandleScanConflictWhenLocked(t *testing.T) {
	srv, h := testServerAndHandler(t)

	if !srv.scanMu.TryLock() {
		t.Fatal("前提: scanMu の取得に失敗した")
	}
	defer srv.scanMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON デコード失敗: %v, body=%s", err, rec.Body.String())
	}
	if body.Error != wantScanConflictMsg {
		t.Errorf("error = %q, want %q", body.Error, wantScanConflictMsg)
	}
}

// scanMu 保持中に POST /api/thumbnails/rebuild → 409(スキャンとロックを共有していることの証明)。
func TestHandleRebuildThumbnailsConflictWhenLocked(t *testing.T) {
	srv, h := testServerAndHandler(t)

	if !srv.scanMu.TryLock() {
		t.Fatal("前提: scanMu の取得に失敗した")
	}
	defer srv.scanMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/thumbnails/rebuild", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), wantScanConflictMsg) {
		t.Errorf("body = %s, want to contain %q", rec.Body.String(), wantScanConflictMsg)
	}
}

// ロック解放後に POST /api/scan → 200 で正常応答(ロックが正しく解放されること)。
func TestHandleScanSucceedsAfterUnlock(t *testing.T) {
	srv, h := testServerAndHandler(t)

	// 一度取得してすぐ解放しておく(ロックが残留しないことの確認を兼ねる)
	if !srv.scanMu.TryLock() {
		t.Fatal("前提: scanMu の取得に失敗した")
	}
	srv.scanMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

// handleScan 自体が成功時にロックを解放すること(defer Unlock の確認)。
// 直列に 2 回叩いて両方成功すれば、1 回目のハンドラ内で確実に Unlock されている。
func TestHandleScanReleasesLockAfterSuccess(t *testing.T) {
	_, h := testServerAndHandler(t)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%d 回目: status = %d, body = %s", i+1, rec.Code, rec.Body.String())
		}
	}
}
