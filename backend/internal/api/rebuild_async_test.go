package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// ---- サムネイル一括再生成の非同期化 (issue #55) --------------------------------
//
// POST /api/thumbnails/rebuild は次を守らなければならない:
//   - 作品一覧の取得と total 確定だけを同期で行い、202 Accepted を即座に返す
//     (worker pool 実行と DB 更新は goroutine に委譲し、リクエストをブロックしない)
//   - レスポンスボディは running=true と確定済みの total を含む進捗スナップショット
//   - scanMu を TryLock できない場合は(スキャンや別の一括再生成と競合)409 を返す
//     (この経路自体は scan_guard_test.go でカバー済み)
//
// GET /api/thumbnails/rebuild/status は次を守らなければならない:
//   - 一度も実行していない場合は 200 + 全て zero value
//   - ジョブ実行中/実行後は {"running","checked","regenerated","total"} を返す
//   - scanMu を使わないため、スキャン/一括再生成のロック中でも 200 を返せる

// rebuildStatusBody は GET/POST の JSON ボディをデコードするためのテスト用構造体。
type rebuildStatusBody struct {
	Running     bool `json:"running"`
	Checked     int  `json:"checked"`
	Regenerated int  `json:"regenerated"`
	Total       int  `json:"total"`
}

// waitForRebuildDone は running=false になるまで GET status をポーリングする。
// テスト用のワーカー数・作品数であれば数百 ms 以内に完了するはずなので、
// タイムアウト(5秒)を超えたら Fatal で打ち切る。
func waitForRebuildDone(t *testing.T, h http.Handler) rebuildStatusBody {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last rebuildStatusBody
	for time.Now().Before(deadline) {
		w := doGet(t, h, "/api/thumbnails/rebuild/status")
		if w.Code != http.StatusOK {
			t.Fatalf("status polling: code = %d, body = %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &last); err != nil {
			t.Fatalf("JSON デコード失敗: %v", err)
		}
		if !last.Running {
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("タイムアウト: ジョブが完了しなかった(最終スナップショット = %+v)", last)
	return last
}

// 実行前: GET status → 200 で全て zero value。
func TestRebuildThumbnailsStatus_InitialZero(t *testing.T) {
	h, _, _ := newTestServer(t)

	w := doGet(t, h, "/api/thumbnails/rebuild/status")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var body rebuildStatusBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON デコード失敗: %v", err)
	}
	if body.Running || body.Checked != 0 || body.Regenerated != 0 || body.Total != 0 {
		t.Errorf("初回の status = %+v, want 全て zero value", body)
	}
}

// POST → 202 + running=true + total 確定。その後 status をポーリングすると
// running=false かつ checked=total になる。
func TestRebuildThumbnailsAsync_AcceptedThenCompletes(t *testing.T) {
	h, database, _ := newTestServer(t)

	// 表紙ありの作品を追加(再生成が実際に起きるケースを混ぜる)
	root := makeWorkDir(t, database, "RJ850001", nil)
	writePNG(t, filepath.Join(root, "cover.png"), 50, 50)

	req := httptest.NewRequest(http.MethodPost, "/api/thumbnails/rebuild", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", rec.Code, rec.Body.String())
	}

	var accepted rebuildStatusBody
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("JSON デコード失敗: %v", err)
	}
	if !accepted.Running {
		t.Error("202 応答で running=true になっていない")
	}
	// newTestServer の作品(表紙.jpg は非画像でデコード失敗) + RJ850001(cover.png 有効) = 2
	if accepted.Total != 2 {
		t.Errorf("total = %d, want 2", accepted.Total)
	}

	final := waitForRebuildDone(t, h)
	if final.Checked != final.Total {
		t.Errorf("checked = %d, total = %d: 完了後は一致するはず", final.Checked, final.Total)
	}
	if final.Regenerated != 1 {
		t.Errorf("regenerated = %d, want 1", final.Regenerated)
	}
}

// GET status は scanMu を使わないため、スキャン/一括再生成のロック中でも 200 を返す。
func TestRebuildThumbnailsStatus_AvailableWhileScanLocked(t *testing.T) {
	srv, h := testServerAndHandler(t)

	if !srv.scanMu.TryLock() {
		t.Fatal("前提: scanMu の取得に失敗した")
	}
	defer srv.scanMu.Unlock()

	w := doGet(t, h, "/api/thumbnails/rebuild/status")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200(status は scanMu を使わない)", w.Code)
	}
}
