package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---- サムネイル一括再生成中の並行書き込み回帰テスト (issue #85) ----------------
//
// PR #79 で runRebuildThumbnailsJob は「結果ドレイン中は DB に一切触れず、全件
// ドレイン後に短い tx でまとめて UPDATE する」構造に修正された。ドレイン中に tx を
// 張ったまま画像デコード(1 件あたり数十 ms〜、全体で数分規模になり得る)を待つと、
// SQLite の書き込みロックがジョブ全体の間保持され、scanMu 外の他の書き込み API
// (plays/PATCH 等)が busy_timeout(5秒)後に 500 になってしまう。
//
// このテストは、ジョブが worker の Refresh で確実にブロックしている状態を作った上で
// POST /api/works/{id}/plays と PATCH /api/works/{id} を発行し、両方とも 500 に
// ならないことを固定する。「ドレイン中に tx を張る」リグレッションが再導入されれば、
// この 2 つの書き込みが busy_timeout 後に 500 になり本テストは失敗する。

// fakeThumbRefresher は newThumbRefresher の差し替え用の偽物。
// Refresh は started に通知した後、release が close されるまでブロックする。
// 複数 work があっても全呼び出しが同じ release チャネルを待ってよい。
type fakeThumbRefresher struct {
	started chan struct{}
	release chan struct{}
}

func (f *fakeThumbRefresher) Refresh(workID int64, rootPath string) (bool, string, bool, error) {
	select {
	case f.started <- struct{}{}:
	case <-time.After(5 * time.Second):
	}
	select {
	case <-f.release:
	case <-time.After(5 * time.Second):
	}
	return false, "", true, nil
}

func TestRebuildThumbnails_ConcurrentWritesDoNotFail(t *testing.T) {
	fake := &fakeThumbRefresher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	orig := newThumbRefresher
	newThumbRefresher = func(thumbsDir string) thumbRefresher { return fake }
	t.Cleanup(func() { newThumbRefresher = orig })

	h, _, id := newTestServer(t)

	// POST /api/thumbnails/rebuild → 202
	req := httptest.NewRequest(http.MethodPost, "/api/thumbnails/rebuild", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rebuild status = %d, want 202, body = %s", rec.Code, rec.Body.String())
	}

	// 偽物 Refresh の開始通知を待つ。これでジョブが worker 内でブロック中(ドレイン待ち)
	// であることが保証される。
	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("タイムアウト: 偽物 Refresh の開始通知を受信できなかった")
	}

	// ジョブがブロック中の状態で並行書き込みを発行する。plays / PATCH は scanMu を
	// 取らないためロック競合はなく、旧バグ(ドレイン中の tx 保持)が再導入された場合のみ
	// busy_timeout 後に 500 になる。
	playRec := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/plays", id), `{"path":"mp3/01_intro.mp3"}`)
	if playRec.Code != http.StatusCreated {
		t.Errorf("plays status = %d, want 201, body = %s", playRec.Code, playRec.Body.String())
	}

	patchRec := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"title":"並行書き込みテスト"}`)
	if patchRec.Code != http.StatusNoContent {
		t.Errorf("patch status = %d, want 204, body = %s", patchRec.Code, patchRec.Body.String())
	}

	// ジョブを完走させる。
	close(fake.release)

	final := waitForRebuildDone(t, h)
	if final.Running {
		t.Errorf("ジョブ完走後も running=true のまま: %+v", final)
	}
}
