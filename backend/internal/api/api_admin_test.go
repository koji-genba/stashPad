package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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

// ensureThumbsDir は本番の main.go と同様に DataDir/thumbs を作成する。
// newTestServer は DataDir=tmp(= 作品フォルダの親)を使うため、既存作品の
// root_path の親から DataDir を割り出して thumbs ディレクトリを掘る。
func ensureThumbsDir(t *testing.T, database *sql.DB) {
	t.Helper()
	var existing string
	if err := database.QueryRow(
		"SELECT root_path FROM works WHERE root_path IS NOT NULL LIMIT 1").Scan(&existing); err != nil {
		t.Fatalf("既存 root_path 取得失敗: %v", err)
	}
	dataDir := filepath.Dir(existing)
	if err := os.MkdirAll(filepath.Join(dataDir, "thumbs"), 0o755); err != nil {
		t.Fatal(err)
	}
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
	ensureThumbsDir(t, database)
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
	ensureThumbsDir(t, database)
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

// ---- POST /api/thumbnails/rebuild --------------------------------------------

func TestRebuildThumbnails(t *testing.T) {
	h, database, baseID := newTestServer(t)
	ensureThumbsDir(t, database)

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
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Checked     int `json:"checked"`
		Regenerated int `json:"regenerated"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)

	// root_path がある作品: baseID(表紙.jpg中身非画像) + RJ830001(cover有) + RJ830002(なし) = 3
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
	ensureThumbsDir(t, database)

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
