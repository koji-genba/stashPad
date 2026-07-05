package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/koji-genba/stashpad/backend/internal/config"
	"github.com/koji-genba/stashpad/backend/internal/db"
)

// makeWorkDir はライブラリルート配下に作品フォルダを作り、files で指定された
// 相対パス→内容のファイルを書き込み、works テーブルに登録して root_path を返す。
//
// LibraryRoots[0] を作品フォルダの親として使うため、newTestServer の tmp 構造に
// 依存する。ここでは database から既存作品の root_path の親を辿る代わりに、
// 専用のルートを掘る形にはせず、テスト DB に直接 INSERT する。
func makeWorkDir(t *testing.T, database *sql.DB, rjNumber string, files map[string]string) string {
	t.Helper()
	// 既存作品の root_path からライブラリルート(親ディレクトリ)を推定する
	var existing string
	if err := database.QueryRow(
		"SELECT root_path FROM works WHERE root_path IS NOT NULL LIMIT 1").Scan(&existing); err != nil {
		t.Fatalf("既存 root_path 取得失敗: %v", err)
	}
	libRoot := filepath.Dir(existing)

	root := filepath.Join(libRoot, rjNumber+"_テスト")
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(
		"INSERT INTO works (rj_number, title, root_path) VALUES (?, ?, ?)",
		rjNumber, rjNumber, root); err != nil {
		t.Fatal(err)
	}
	return root
}

// writePNG は path に実際にデコード可能な PNG 画像を書き込む。
func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// ---- GET /api/works/{id}/thumbnail -------------------------------------------

// thumbnail_path NULL → 404
func TestWorkThumbnailNull(t *testing.T) {
	h, _, id := newTestServer(t)
	w := doGet(t, h, urlf("/api/works/%d/thumbnail", id))
	if w.Code != http.StatusNotFound {
		t.Errorf("thumbnail_path NULL status = %d, want 404", w.Code)
	}
}

// 実ファイルあり → 200 + Content-Type image/jpeg
func TestWorkThumbnailServed(t *testing.T) {
	h, database, id := newTestServer(t)
	tmp := t.TempDir()
	thumbFile := filepath.Join(tmp, "thumb.jpg")
	// 中身は何でもよい(ServeContent が配信する)
	if err := os.WriteFile(thumbFile, []byte("jpegdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		"UPDATE works SET thumbnail_path=? WHERE id=?", thumbFile, id); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, urlf("/api/works/%d/thumbnail", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	if w.Body.String() != "jpegdata" {
		t.Errorf("body = %q", w.Body.String())
	}
}

// サムネイルは Last-Modified による 304 だけでなく Cache-Control でブラウザキャッシュも効かせる。
func TestWorkThumbnailCacheControl(t *testing.T) {
	h, database, id := newTestServer(t)
	tmp := t.TempDir()
	thumbFile := filepath.Join(tmp, "thumb.jpg")
	if err := os.WriteFile(thumbFile, []byte("jpegdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		"UPDATE works SET thumbnail_path=? WHERE id=?", thumbFile, id); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, urlf("/api/works/%d/thumbnail", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	const want = "private, max-age=3600"
	if cc := w.Header().Get("Cache-Control"); cc != want {
		t.Errorf("Cache-Control = %q, want %q", cc, want)
	}

	// If-Modified-Since で 304 になるケースでも Cache-Control が落ちないことを確認する
	// (http.ServeContent の挙動が変わったり、Set 順序を変えるリファクタで気付けるように)。
	lastMod := w.Header().Get("Last-Modified")
	if lastMod == "" {
		t.Fatal("Last-Modified が返っていない")
	}
	req := httptest.NewRequest(http.MethodGet, urlf("/api/works/%d/thumbnail", id), nil)
	req.Header.Set("If-Modified-Since", lastMod)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req)
	if w2.Code != http.StatusNotModified {
		t.Fatalf("304 期待: status = %d", w2.Code)
	}
	if cc := w2.Header().Get("Cache-Control"); cc != want {
		t.Errorf("304 時の Cache-Control = %q, want %q", cc, want)
	}
}

// DB にパスはあるがファイルが消失 → 404
func TestWorkThumbnailFileMissing(t *testing.T) {
	h, database, id := newTestServer(t)
	if _, err := database.Exec(
		"UPDATE works SET thumbnail_path='/nonexistent/path/x.jpg' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	w := doGet(t, h, urlf("/api/works/%d/thumbnail", id))
	if w.Code != http.StatusNotFound {
		t.Errorf("ファイル消失 status = %d, want 404", w.Code)
	}
}

// 存在しない作品 → 404
func TestWorkThumbnailWorkNotFound(t *testing.T) {
	h, _, _ := newTestServer(t)
	w := doGet(t, h, "/api/works/99999/thumbnail")
	if w.Code != http.StatusNotFound {
		t.Errorf("存在しない作品 status = %d, want 404", w.Code)
	}
}

// ---- POST /api/works/{id}/thumbnail/refresh ----------------------------------

// root_path NULL → 200 {"refreshed": false}
func TestRefreshThumbnailRootNull(t *testing.T) {
	h, database, _ := newTestServer(t)
	res, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ810000', 'なし')")
	id, _ := res.LastInsertId()

	req := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Refreshed bool `json:"refreshed"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Refreshed {
		t.Error("root_path NULL なのに refreshed=true")
	}
}

// 表紙画像のある作品 → refreshed true + thumbnail_path 更新
func TestRefreshThumbnailWithCover(t *testing.T) {
	h, database, _ := newTestServer(t)
	root := makeWorkDir(t, database, "RJ820000", nil)
	writePNG(t, filepath.Join(root, "cover.png"), 100, 100)
	var id int64
	database.QueryRow("SELECT id FROM works WHERE rj_number='RJ820000'").Scan(&id)

	req := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Refreshed bool `json:"refreshed"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Refreshed {
		t.Error("表紙画像があるのに refreshed=false")
	}

	// thumbnail_path が更新されている
	var thumbPath sql.NullString
	database.QueryRow("SELECT thumbnail_path FROM works WHERE id=?", id).Scan(&thumbPath)
	if !thumbPath.Valid {
		t.Error("thumbnail_path が更新されていない")
	}
	if _, err := os.Stat(thumbPath.String); err != nil {
		t.Errorf("生成されたサムネイルファイルが存在しない: %v", err)
	}
}

// root_path はあるが画像がない作品 → 200 {"refreshed": false}
func TestRefreshThumbnailNoImage(t *testing.T) {
	h, database, _ := newTestServer(t)
	makeWorkDir(t, database, "RJ825000", map[string]string{"readme.txt": "x"})
	var id int64
	database.QueryRow("SELECT id FROM works WHERE rj_number='RJ825000'").Scan(&id)

	req := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Refreshed bool `json:"refreshed"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Refreshed {
		t.Error("画像がないのに refreshed=true")
	}
}

// 画像が消えた作品の単発 refresh は古い thumbnail_path と実ファイルを消す。
func TestRefreshThumbnailNoImageClearsStaleThumbnail(t *testing.T) {
	h, database, _ := newTestServer(t)
	root := makeWorkDir(t, database, "RJ825001", map[string]string{"readme.txt": "x"})
	var id int64
	database.QueryRow("SELECT id FROM works WHERE rj_number='RJ825001'").Scan(&id)

	thumbsDir := filepath.Join(filepath.Dir(root), "thumbs")
	oldThumb := filepath.Join(thumbsDir, fmt.Sprintf("%d.jpg", id))
	oldSrc := filepath.Join(thumbsDir, fmt.Sprintf("%d.src", id))
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, oldThumb, 40, 40)
	if err := os.WriteFile(oldSrc, []byte(filepath.Join(root, "deleted-cover.png")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("UPDATE works SET thumbnail_path=? WHERE id=?", oldThumb, id); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var thumbPath sql.NullString
	database.QueryRow("SELECT thumbnail_path FROM works WHERE id=?", id).Scan(&thumbPath)
	if thumbPath.Valid {
		t.Errorf("thumbnail_path = %q, want NULL", thumbPath.String)
	}
	if _, err := os.Stat(oldThumb); !os.IsNotExist(err) {
		t.Errorf("古いサムネイルが削除されていない: %v", err)
	}
	if _, err := os.Stat(oldSrc); !os.IsNotExist(err) {
		t.Errorf("古い .src が削除されていない: %v", err)
	}

	w := doGet(t, h, urlf("/api/works/%d/thumbnail", id))
	if w.Code != http.StatusNotFound {
		t.Errorf("thumbnail GET status = %d, want 404", w.Code)
	}
}

// 存在しない作品 → 404
func TestRefreshThumbnailWorkNotFound(t *testing.T) {
	h, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/works/99999/thumbnail/refresh", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ---- POST /api/thumbnails/rebuild(非同期。issue #55) --------------------------
//
// POST は 202 Accepted を即座に返すだけで、実際の再生成は goroutine で非同期に
// 進む。最終結果は GET /api/thumbnails/rebuild/status をポーリングして確認する
// (ポーリングの契約自体は rebuild_async_test.go でカバーする)。

func TestRebuildThumbnails(t *testing.T) {
	h, database, baseID := newTestServer(t)

	// 表紙ありの作品
	rootWith := makeWorkDir(t, database, "RJ830001", nil)
	writePNG(t, filepath.Join(rootWith, "cover.png"), 100, 100)

	// 表紙なしの作品(テキストのみ)
	makeWorkDir(t, database, "RJ830002", map[string]string{"readme.txt": "x"})

	// newTestServer の baseID も root_path あり(表紙.jpg は JPG だが中身が画像でないので生成失敗)
	_ = baseID

	req := httptest.NewRequest(http.MethodPost, "/api/thumbnails/rebuild", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", rec.Code, rec.Body.String())
	}
	var accepted struct {
		Running bool `json:"running"`
		Total   int  `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("JSON デコード失敗: %v", err)
	}
	if !accepted.Running {
		t.Error("202 応答で running=true になっていない")
	}
	// root_path がある作品: baseID(表紙.jpg中身非画像) + RJ830001(cover有) + RJ830002(なし) = 3
	if accepted.Total != 3 {
		t.Errorf("total = %d, want 3", accepted.Total)
	}

	body := waitForRebuildDone(t, h)

	if body.Checked != 3 {
		t.Errorf("checked = %d, want 3", body.Checked)
	}
	// 実際に表紙画像として有効なのは RJ830001 のみ
	if body.Regenerated != 1 {
		t.Errorf("regenerated = %d, want 1", body.Regenerated)
	}

	// RJ830001 の thumbnail_path が更新されている
	var thumbPath sql.NullString
	database.QueryRow("SELECT thumbnail_path FROM works WHERE rj_number='RJ830001'").Scan(&thumbPath)
	if !thumbPath.Valid {
		t.Error("RJ830001 の thumbnail_path が更新されていない")
	}
}

// 一括 rebuild は画像が消えた作品の古い thumbnail_path と実ファイルを消す。
func TestRebuildThumbnailsClearsStaleThumbnailWhenNoImage(t *testing.T) {
	h, database, _ := newTestServer(t)
	root := makeWorkDir(t, database, "RJ830003", map[string]string{"readme.txt": "x"})
	var id int64
	database.QueryRow("SELECT id FROM works WHERE rj_number='RJ830003'").Scan(&id)

	thumbsDir := filepath.Join(filepath.Dir(root), "thumbs")
	oldThumb := filepath.Join(thumbsDir, fmt.Sprintf("%d.jpg", id))
	oldSrc := filepath.Join(thumbsDir, fmt.Sprintf("%d.src", id))
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, oldThumb, 40, 40)
	if err := os.WriteFile(oldSrc, []byte(filepath.Join(root, "deleted-cover.png")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("UPDATE works SET thumbnail_path=? WHERE id=?", oldThumb, id); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/thumbnails/rebuild", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", rec.Code, rec.Body.String())
	}
	waitForRebuildDone(t, h)

	var thumbPath sql.NullString
	database.QueryRow("SELECT thumbnail_path FROM works WHERE id=?", id).Scan(&thumbPath)
	if thumbPath.Valid {
		t.Errorf("thumbnail_path = %q, want NULL", thumbPath.String)
	}
	if _, err := os.Stat(oldThumb); !os.IsNotExist(err) {
		t.Errorf("古いサムネイルが削除されていない: %v", err)
	}
	if _, err := os.Stat(oldSrc); !os.IsNotExist(err) {
		t.Errorf("古い .src が削除されていない: %v", err)
	}
}

// ---- POST /api/works/{id}/thumbnail/refresh スロットル -------------------

// TestRefreshThumbnailUpdatesCheckedAt: 初回 refresh で thumb_checked_at が設定される。
func TestRefreshThumbnailUpdatesCheckedAt(t *testing.T) {
	h, database, _ := newTestServer(t)
	root := makeWorkDir(t, database, "RJ840001", nil)
	writePNG(t, filepath.Join(root, "cover.png"), 100, 100)
	var id int64
	database.QueryRow("SELECT id FROM works WHERE rj_number='RJ840001'").Scan(&id)

	req := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// thumb_checked_at が NULL でなくなっていることを確認
	var checkedAt sql.NullString
	database.QueryRow("SELECT thumb_checked_at FROM works WHERE id=?", id).Scan(&checkedAt)
	if !checkedAt.Valid {
		t.Error("refresh 後に thumb_checked_at が NULL のまま")
	}
}

// TestRefreshThumbnailThrottledWithin24h: 24h 以内に checked_at がある場合、
// thumb.Refresh は呼ばれずに refreshed=false が返る。
func TestRefreshThumbnailThrottledWithin24h(t *testing.T) {
	h, database, _ := newTestServer(t)
	root := makeWorkDir(t, database, "RJ840002", nil)
	writePNG(t, filepath.Join(root, "cover.png"), 100, 100)
	var id int64
	database.QueryRow("SELECT id FROM works WHERE rj_number='RJ840002'").Scan(&id)

	// thumb_checked_at を現在時刻にセット(24h 以内 → スロットル対象)
	if _, err := database.Exec(
		"UPDATE works SET thumb_checked_at=datetime('now') WHERE id=?", id,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Refreshed bool `json:"refreshed"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Refreshed {
		t.Error("スロットル中なのに refreshed=true が返った")
	}

	// thumb_checked_at の値が変わっていないこと(再 UPDATE されない)
	var checkedAt1 sql.NullString
	database.QueryRow("SELECT thumb_checked_at FROM works WHERE id=?", id).Scan(&checkedAt1)

	req2 := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	var checkedAt2 sql.NullString
	database.QueryRow("SELECT thumb_checked_at FROM works WHERE id=?", id).Scan(&checkedAt2)
	if checkedAt1.String != checkedAt2.String {
		t.Errorf("スロットル中に thumb_checked_at が変わった: %q → %q", checkedAt1.String, checkedAt2.String)
	}

	// walk が走っていない証拠: cover.png を削除してもスロットル中は 200 で返る
	if err := os.Remove(filepath.Join(root, "cover.png")); err != nil {
		t.Fatal(err)
	}
	req3 := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("cover 削除後スロットル中: status = %d, want 200", rec3.Code)
	}
}

// TestRefreshThumbnailReExpiresAfter24h: 25 時間前に checked_at が設定されている場合、
// スロットルを通過して thumb.Refresh が実行され、checked_at が更新される。
func TestRefreshThumbnailReExpiresAfter24h(t *testing.T) {
	h, database, _ := newTestServer(t)
	root := makeWorkDir(t, database, "RJ840003", nil)
	writePNG(t, filepath.Join(root, "cover.png"), 100, 100)
	var id int64
	database.QueryRow("SELECT id FROM works WHERE rj_number='RJ840003'").Scan(&id)

	// thumb_checked_at を 25 時間前にセット(有効期限切れ → スロットル対象外)
	if _, err := database.Exec(
		"UPDATE works SET thumb_checked_at=datetime('now', '-25 hours') WHERE id=?", id,
	); err != nil {
		t.Fatal(err)
	}
	var oldCheckedAt sql.NullString
	database.QueryRow("SELECT thumb_checked_at FROM works WHERE id=?", id).Scan(&oldCheckedAt)

	req := httptest.NewRequest(http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// thumb_checked_at が新しい値に更新されていること
	var newCheckedAt sql.NullString
	database.QueryRow("SELECT thumb_checked_at FROM works WHERE id=?", id).Scan(&newCheckedAt)
	if !newCheckedAt.Valid {
		t.Error("refresh 後に thumb_checked_at が NULL")
	}
	if oldCheckedAt.String == newCheckedAt.String {
		t.Errorf("thumb_checked_at が更新されていない: %q", newCheckedAt.String)
	}
}

// ---- POST /api/import/csv ----------------------------------------------------

// multipartCSV は file フィールドに CSV 内容を持つ multipart リクエストを組み立てる。
func multipartCSV(t *testing.T, csvContent string) (*http.Request, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "works.csv")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(csvContent))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/import/csv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req, mw.FormDataContentType()
}

func TestImportCSV(t *testing.T) {
	h, database, _ := newTestServer(t)

	csvContent := "rj_number,title,circle,genres,voice_actor\n" +
		"RJ900001,新作タイトル,新作サークル,\"R-18, ボイス・ASMR\",声優A/声優B\n" +
		"RJ900002,別作品,別サークル,癒し,声優C\n"

	req, _ := multipartCSV(t, csvContent)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Created int `json:"created"`
		Updated int `json:"updated"`
		Linked  int `json:"linked"`
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Created != 2 {
		t.Errorf("created = %d, want 2", res.Created)
	}

	// DB 反映確認
	var title, circle string
	if err := database.QueryRow(
		"SELECT title, circle FROM works WHERE rj_number='RJ900001'").
		Scan(&title, &circle); err != nil {
		t.Fatal(err)
	}
	if title != "新作タイトル" || circle != "新作サークル" {
		t.Errorf("title=%q circle=%q", title, circle)
	}
	// タグも展開されている
	var tagCount int
	database.QueryRow(`
		SELECT COUNT(*) FROM work_tags wt
		JOIN works w ON w.id=wt.work_id WHERE w.rj_number='RJ900001'`).Scan(&tagCount)
	if tagCount == 0 {
		t.Error("CSV タグが展開されていない")
	}
}

// file フィールドなし → 400
func TestImportCSVNoFile(t *testing.T) {
	h, _, _ := newTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("other", "value")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/import/csv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("file なし status = %d, want 400", rec.Code)
	}
}

// ---- POST /api/scan ----------------------------------------------------------

func TestScan(t *testing.T) {
	h, database, _ := newTestServer(t)

	// LibraryRoots 配下(= 既存 root_path の親)に新しい RJ フォルダを作る
	var existing string
	database.QueryRow("SELECT root_path FROM works WHERE root_path IS NOT NULL LIMIT 1").Scan(&existing)
	libRoot := filepath.Dir(existing)

	newDir := filepath.Join(libRoot, "RJ950001_スキャン対象")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "audio.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res struct {
		WorksFound      int `json:"works_found"`
		NewlyRegistered int `json:"newly_registered"`
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if res.WorksFound < 1 {
		t.Errorf("works_found = %d, want >= 1", res.WorksFound)
	}
	if res.NewlyRegistered < 1 {
		t.Errorf("newly_registered = %d, want >= 1", res.NewlyRegistered)
	}

	// DB に登録される
	var cnt int
	database.QueryRow("SELECT COUNT(*) FROM works WHERE rj_number='RJ950001'").Scan(&cnt)
	if cnt != 1 {
		t.Errorf("RJ950001 の登録数 = %d, want 1", cnt)
	}
}

// 全ライブラリルートが読めない場合(NAS 未マウント等)は、固定の「スキャン失敗」に
// 丸めず、対処を促すメッセージ付きの 503 を返すこと(issue #48 / #70)
func TestScanAllRootsUnreadableReturns503(t *testing.T) {
	tmp := t.TempDir()
	database, err := db.Open(filepath.Join(tmp, "test.db"))
	if err != nil {
		t.Fatalf("DB オープン失敗: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		LibraryRoots: []string{filepath.Join(tmp, "not-mounted")},
		DataDir:      tmp,
		Addr:         ":0",
	}
	h := New(database, cfg).Router()

	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("マウント")) {
		t.Errorf("body = %s, want マウント状態の確認を促すメッセージ", rec.Body.String())
	}
}
