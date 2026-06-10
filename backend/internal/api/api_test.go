package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koji-genba/stashpad/backend/internal/config"
	"github.com/koji-genba/stashpad/backend/internal/db"
)

// newTestServer はテスト用の DB・作品フォルダ・ルーターを組み立てる。
// 作品フォルダ構成:
//
//	mp3/01_intro.mp3, mp3/2_main.mp3, mp3/10_end.mp3
//	表紙.jpg, 台本.txt
func newTestServer(t *testing.T) (handler http.Handler, database *sql.DB, workID int64) {
	t.Helper()
	tmp := t.TempDir()

	database, err := db.Open(filepath.Join(tmp, "test.db"))
	if err != nil {
		t.Fatalf("DB オープン失敗: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	root := filepath.Join(tmp, "RJ000001_テスト作品")
	if err := os.MkdirAll(filepath.Join(root, "mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"mp3/01_intro.mp3", "mp3/2_main.mp3", "mp3/10_end.mp3", "表紙.jpg", "台本.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := database.Exec(
		"INSERT INTO works (rj_number, title, root_path) VALUES ('RJ000001', 'テスト作品', ?)", root,
	)
	if err != nil {
		t.Fatal(err)
	}
	workID, _ = res.LastInsertId()

	cfg := &config.Config{LibraryRoots: []string{tmp}, DataDir: tmp, Addr: ":0"}
	return New(database, cfg).Router(), database, workID
}

func doGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// エントリ一覧: ディレクトリ→ファイルの順、それぞれ自然順でソートされること
func TestEntriesNaturalOrder(t *testing.T) {
	h, _, id := newTestServer(t)

	w := doGet(t, h, urlf("/api/works/%d/entries?path=", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Entries []struct {
			Name      string `json:"name"`
			IsDir     bool   `json:"is_dir"`
			MediaKind string `json:"media_kind"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range body.Entries {
		names = append(names, e.Name)
	}
	want := []string{"mp3", "台本.txt", "表紙.jpg"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("entries = %v, want %v", names, want)
	}

	// サブディレクトリの自然順(01 < 2 < 10)
	w = doGet(t, h, urlf("/api/works/%d/entries?path=mp3", id))
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	names = nil
	for _, e := range body.Entries {
		names = append(names, e.Name)
	}
	want = []string{"01_intro.mp3", "2_main.mp3", "10_end.mp3"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("mp3 entries = %v, want %v", names, want)
	}
	for _, e := range body.Entries {
		if e.MediaKind != "audio" {
			t.Errorf("media_kind(%s) = %q, want audio", e.Name, e.MediaKind)
		}
	}
}

// パストラバーサルが API 経由で 403 になること(セキュリティ境界の結合確認)
func TestEntriesAndFileTraversalForbidden(t *testing.T) {
	h, _, id := newTestServer(t)

	for _, ep := range []string{"entries", "file"} {
		for _, p := range []string{"../../etc/passwd", "..", `..\..\evil`} {
			w := doGet(t, h, urlf("/api/works/%d/%s?path=", id, ep)+url.QueryEscape(p))
			if w.Code != http.StatusForbidden {
				t.Errorf("%s path=%q: status = %d, want 403", ep, p, w.Code)
			}
		}
	}
}

// ファイル配信: 200 / Range 206 / 存在しないパス 404
func TestFileServing(t *testing.T) {
	h, _, id := newTestServer(t)

	w := doGet(t, h, urlf("/api/works/%d/file?path=", id)+url.QueryEscape("mp3/01_intro.mp3"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if w.Body.String() != "0123456789" {
		t.Errorf("body = %q", w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet,
		urlf("/api/works/%d/file?path=", id)+url.QueryEscape("mp3/01_intro.mp3"), nil)
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Errorf("Range status = %d, want 206", rec.Code)
	}
	if rec.Body.String() != "2345" {
		t.Errorf("Range body = %q, want 2345", rec.Body.String())
	}

	w = doGet(t, h, urlf("/api/works/%d/file?path=", id)+url.QueryEscape("mp3/missing.mp3"))
	if w.Code != http.StatusNotFound {
		t.Errorf("missing file status = %d, want 404", w.Code)
	}
}

// 再生記録 → 履歴一覧の結合確認
func TestPlaysAndHistory(t *testing.T) {
	h, _, id := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost,
		urlf("/api/works/%d/plays", id),
		strings.NewReader(`{"path":"mp3/01_intro.mp3"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("plays status = %d", rec.Code)
	}

	w := doGet(t, h, "/api/history")
	if w.Code != http.StatusOK {
		t.Fatalf("history status = %d", w.Code)
	}
	var body struct {
		Items []struct {
			Work struct {
				ID int64 `json:"id"`
			} `json:"work"`
			LastFilePath string `json:"last_file_path"`
			PlayCount    int    `json:"play_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Work.ID != id ||
		body.Items[0].LastFilePath != "mp3/01_intro.mp3" || body.Items[0].PlayCount != 1 {
		t.Errorf("history items = %+v", body.Items)
	}
}

func urlf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
