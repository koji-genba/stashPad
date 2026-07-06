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

// ---- GET /api/works circle/series フィルタ・sort=circle のテスト ---------------

func TestListWorksCircleSeriesFilter(t *testing.T) {
	h, database, _ := newTestServer(t)

	// 追加作品を挿入
	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle, series_name, root_path)
		VALUES
		  ('RJ000002', 'サークルA作品1', 'サークルA', 'シリーズX', '/tmp/fake2'),
		  ('RJ000003', 'サークルA作品2', 'サークルA', 'シリーズY', '/tmp/fake3'),
		  ('RJ000004', 'サークルB作品1', 'サークルB', 'シリーズX', '/tmp/fake4')
	`); err != nil {
		t.Fatal(err)
	}

	// circle フィルタ
	w := doGet(t, h, "/api/works?circle="+url.QueryEscape("サークルA"))
	if w.Code != http.StatusOK {
		t.Fatalf("circle filter status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 2 {
		t.Errorf("circle=サークルA total = %d, want 2", body.Total)
	}

	// series フィルタ
	w = doGet(t, h, "/api/works?series="+url.QueryEscape("シリーズX"))
	if w.Code != http.StatusOK {
		t.Fatalf("series filter status = %d, body = %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 2 {
		t.Errorf("series=シリーズX total = %d, want 2", body.Total)
	}

	// circle + series 組み合わせ
	w = doGet(t, h, "/api/works?circle="+url.QueryEscape("サークルA")+"&series="+url.QueryEscape("シリーズX"))
	if w.Code != http.StatusOK {
		t.Fatalf("circle+series filter status = %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 {
		t.Errorf("circle+series total = %d, want 1", body.Total)
	}
}

func TestListWorksWorkTypeAgeRatingFilter(t *testing.T) {
	h, database, _ := newTestServer(t)

	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, work_type, age_rating)
		VALUES
		  ('RJ000020', '全年齢ボイス', 'ボイス・ASMR', '全年齢'),
		  ('RJ000021', 'R15ボイス', 'ボイス・ASMR', 'R-15'),
		  ('RJ000022', 'R18マンガ', 'マンガ', 'R-18')
	`); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		want int
	}{
		{"work_type", "/api/works?work_type=" + url.QueryEscape("ボイス・ASMR"), 2},
		{"age_rating", "/api/works?age_rating=" + url.QueryEscape("R-18"), 1},
		{
			"combined",
			"/api/works?work_type=" + url.QueryEscape("ボイス・ASMR") + "&age_rating=" + url.QueryEscape("R-15"),
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doGet(t, h, tc.path)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			var body struct {
				Total int `json:"total"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Total != tc.want {
				t.Errorf("total = %d, want %d", body.Total, tc.want)
			}
		})
	}
}

func TestListWorksSortCircle(t *testing.T) {
	h, database, _ := newTestServer(t)

	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle)
		VALUES
		  ('RJ000010', '作品Z', 'ZZZ'),
		  ('RJ000011', '作品A', 'AAA')
	`); err != nil {
		t.Fatal(err)
	}

	// sort=circle&order=asc
	w := doGet(t, h, "/api/works?sort=circle&order=asc")
	if w.Code != http.StatusOK {
		t.Fatalf("sort=circle status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []struct {
			Circle *string `json:"circle"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// circle NULL の作品が NULLS LAST で後ろに来て、AAA が ZZZ より前のはず
	var circles []string
	for _, item := range body.Items {
		if item.Circle != nil {
			circles = append(circles, *item.Circle)
		}
	}
	if len(circles) >= 2 && circles[0] != "AAA" {
		t.Errorf("sort=circle asc: first non-null circle = %q, want AAA", circles[0])
	}
}

func TestListWorksSortRJNumber(t *testing.T) {
	h, database, _ := newTestServer(t)

	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title)
		VALUES
		  ('RJ000020', 'RJ20'),
		  ('RJ000010', 'RJ10'),
		  ('RJ000030', 'RJ30')
	`); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		order string
		want  []string
	}{
		{"asc", []string{"RJ000001", "RJ000010", "RJ000020", "RJ000030"}},
		{"desc", []string{"RJ000030", "RJ000020", "RJ000010", "RJ000001"}},
	} {
		t.Run(tc.order, func(t *testing.T) {
			w := doGet(t, h, "/api/works?sort=rj_number&order="+tc.order)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			var body struct {
				Items []struct {
					RJNumber *string `json:"rj_number"`
				} `json:"items"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, item := range body.Items {
				if item.RJNumber != nil {
					got = append(got, *item.RJNumber)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("rj_number order = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- GET /api/tags: 0件タグ除外・POST /api/tags/cleanup のテスト ---------------

func TestTagsExcludeZeroCount(t *testing.T) {
	h, database, workID := newTestServer(t)

	// タグを作成してリンク(1件)
	if _, err := database.Exec(`
		INSERT INTO tags (name, category) VALUES ('タグA', 'custom'), ('タグB', 'custom')
	`); err != nil {
		t.Fatal(err)
	}
	var tagAID int64
	if err := database.QueryRow("SELECT id FROM tags WHERE name='タグA'").Scan(&tagAID); err != nil {
		t.Fatal(err)
	}
	// タグA だけ作品にリンク(タグB は孤立)
	if _, err := database.Exec(
		"INSERT INTO work_tags (work_id, tag_id) VALUES (?, ?)", workID, tagAID,
	); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/tags")
	if w.Code != http.StatusOK {
		t.Fatalf("tags status = %d", w.Code)
	}
	var body struct {
		Items []struct {
			Name      string `json:"name"`
			WorkCount int    `json:"work_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, item := range body.Items {
		if item.Name == "タグB" {
			t.Error("work_count=0 のタグB が返ってきた")
		}
		if item.WorkCount == 0 {
			t.Errorf("work_count=0 のタグが含まれている: %q", item.Name)
		}
	}
}

func TestTagsCleanup(t *testing.T) {
	h, database, workID := newTestServer(t)

	// タグを挿入
	if _, err := database.Exec(`
		INSERT INTO tags (name, category) VALUES ('使用中', 'custom'), ('孤立A', 'custom'), ('孤立B', 'genre')
	`); err != nil {
		t.Fatal(err)
	}
	var usedID int64
	if err := database.QueryRow("SELECT id FROM tags WHERE name='使用中'").Scan(&usedID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		"INSERT INTO work_tags (work_id, tag_id) VALUES (?, ?)", workID, usedID,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tags/cleanup", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 2 {
		t.Errorf("deleted = %d, want 2", result.Deleted)
	}

	// 使用中タグが残っているか確認
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM tags").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("tags count after cleanup = %d, want 1", count)
	}
}

// ---- ファイル配信: other ファイルの Content-Disposition テスト ----------------

func TestFileServingOtherContentDisposition(t *testing.T) {
	h, _, id := newTestServer(t)

	// 台本.txt は media_kind="text" なので Content-Disposition なし
	w := doGet(t, h, urlf("/api/works/%d/file?path=", id)+url.QueryEscape("台本.txt"))
	if w.Code != http.StatusOK {
		t.Fatalf("txt status = %d, body = %s", w.Code, w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); cd != "" {
		t.Errorf("txt Content-Disposition = %q, want empty", cd)
	}
}

func TestFileServingOtherDispositionForUnknown(t *testing.T) {
	tmp := t.TempDir()

	database, err := db.Open(filepath.Join(tmp, "test.db"))
	if err != nil {
		t.Fatalf("DB オープン失敗: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	root := filepath.Join(tmp, "RJ999999_テスト")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// .zip は other
	if err := os.WriteFile(filepath.Join(root, "アーカイブ.zip"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := database.Exec(
		"INSERT INTO works (rj_number, title, root_path) VALUES ('RJ999999', 'テスト', ?)", root,
	)
	if err != nil {
		t.Fatal(err)
	}
	workID, _ := res.LastInsertId()

	cfg := &config.Config{LibraryRoots: []string{tmp}, DataDir: tmp, Addr: ":0"}
	h := New(database, cfg).Router()

	w := doGet(t, h, urlf("/api/works/%d/file?path=", workID)+url.QueryEscape("アーカイブ.zip"))
	if w.Code != http.StatusOK {
		t.Fatalf("zip status = %d, body = %s", w.Code, w.Body.String())
	}
	cd := w.Header().Get("Content-Disposition")
	if cd == "" {
		t.Error("zip ファイルに Content-Disposition が設定されていない")
	}
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want attachment", cd)
	}
	// RFC5987 エンコードされたファイル名を含む
	if !strings.Contains(cd, "UTF-8''") {
		t.Errorf("Content-Disposition = %q, want RFC5987 encoding", cd)
	}
}
