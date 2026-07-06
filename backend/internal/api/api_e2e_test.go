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
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/koji-genba/stashpad/backend/internal/config"
	"github.com/koji-genba/stashpad/backend/internal/db"
)

// TestE2EFullFlow は実ユースを模した一気通貫テスト。
//
//	scan → import/csv(RJ 番号突合で linked + メタデータ付与)→ works 検索
//	→ entries 閲覧 → file の Range 配信(206)→ plays 記録 → history 確認
func TestE2EFullFlow(t *testing.T) {
	tmp := t.TempDir()

	database, err := db.Open(filepath.Join(tmp, "e2e.db"))
	if err != nil {
		t.Fatalf("DB オープン失敗: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	libRoot := filepath.Join(tmp, "library")
	if err := os.MkdirAll(libRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// DataDir/thumbs は事前に作らない(サムネイル生成側が自動作成することの検証を兼ねる)

	// RJ 形式フォルダ: 音声ファイル + 表紙画像
	rjDir := filepath.Join(libRoot, "RJ123456_素敵な作品")
	if err := os.MkdirAll(filepath.Join(rjDir, "audio"), 0o755); err != nil {
		t.Fatal(err)
	}
	audioContent := []byte("ABCDEFGHIJKLMNOPQRST") // 20 bytes(Range 検証用)
	if err := os.WriteFile(filepath.Join(rjDir, "audio", "track01.mp3"), audioContent, 0o644); err != nil {
		t.Fatal(err)
	}
	writeE2EPNG(t, filepath.Join(rjDir, "cover.png"), 120, 120)

	// RJ なしフォルダ
	noRJDir := filepath.Join(libRoot, "雑多なフォルダ")
	if err := os.MkdirAll(noRJDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noRJDir, "memo.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{LibraryRoots: []string{libRoot}, DataDir: tmp, Addr: ":0"}
	h := New(database, cfg).Router()

	// --- ステップ1: scan ---
	rec := postE2E(t, h, "/api/scan")
	if rec.Code != http.StatusOK {
		t.Fatalf("scan status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var scanRes struct {
		WorksFound      int `json:"works_found"`
		NewlyRegistered int `json:"newly_registered"`
	}
	json.Unmarshal(rec.Body.Bytes(), &scanRes)
	if scanRes.WorksFound != 2 {
		t.Errorf("works_found = %d, want 2(RJ + RJなし)", scanRes.WorksFound)
	}
	if scanRes.NewlyRegistered != 2 {
		t.Errorf("newly_registered = %d, want 2", scanRes.NewlyRegistered)
	}

	// scan 直後の RJ 作品 ID を取得し、root_path が付いていること・サムネイル生成を確認
	var workID int64
	var thumbPath sql.NullString
	if err := database.QueryRow(
		"SELECT id, thumbnail_path FROM works WHERE rj_number='RJ123456'").
		Scan(&workID, &thumbPath); err != nil {
		t.Fatalf("scan 後の作品取得失敗: %v", err)
	}
	if !thumbPath.Valid {
		t.Error("scan で表紙画像からサムネイルが生成されていない")
	}

	// --- ステップ2: import/csv(scan 済み RJ 番号を含む CSV で linked を確認) ---
	csvContent := "rj_number,title,series_name,circle,purchase_date,genres,voice_actor\n" +
		"RJ123456,CSVタイトル,シリーズZ,サークルQ,2026/03/03 12:00,\"R18, ボイス・ASMR\",声優X\n"
	req := multipartCSVReq(t, csvContent)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var importRes struct {
		Created int `json:"created"`
		Updated int `json:"updated"`
		Linked  int `json:"linked"`
	}
	json.Unmarshal(rec.Body.Bytes(), &importRes)
	// scan 済みの RJ 行に CSV がマージされる → updated=1, linked=1, created=0
	if importRes.Linked != 1 {
		t.Errorf("linked = %d, want 1(RJ 番号突合)", importRes.Linked)
	}
	if importRes.Created != 0 {
		t.Errorf("created = %d, want 0(既存 RJ にマージのはず)", importRes.Created)
	}

	// メタデータが付与され、RJ 番号での突合が機能していること
	var title, circle, series string
	if err := database.QueryRow(
		"SELECT title, circle, series_name FROM works WHERE rj_number='RJ123456'").
		Scan(&title, &circle, &series); err != nil {
		t.Fatal(err)
	}
	if title != "CSVタイトル" || circle != "サークルQ" || series != "シリーズZ" {
		t.Errorf("CSV メタデータ未反映: title=%q circle=%q series=%q", title, circle, series)
	}

	// --- ステップ3: GET /api/works で検索 ---
	rec = getE2E(t, h, "/api/works?q="+url.QueryEscape("CSVタイトル"))
	var listRes struct {
		Total int `json:"total"`
		Items []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	json.Unmarshal(rec.Body.Bytes(), &listRes)
	if listRes.Total != 1 || len(listRes.Items) != 1 || listRes.Items[0].ID != workID {
		t.Errorf("検索結果 = %+v, want workID=%d", listRes, workID)
	}

	// --- ステップ4: entries で閲覧 ---
	rec = getE2E(t, h, urlf("/api/works/%d/entries?path=", workID))
	var entRes struct {
		Entries []struct {
			Name  string `json:"name"`
			IsDir bool   `json:"is_dir"`
		} `json:"entries"`
	}
	json.Unmarshal(rec.Body.Bytes(), &entRes)
	var foundAudioDir bool
	for _, e := range entRes.Entries {
		if e.Name == "audio" && e.IsDir {
			foundAudioDir = true
		}
	}
	if !foundAudioDir {
		t.Errorf("entries に audio ディレクトリがない: %+v", entRes.Entries)
	}

	// --- ステップ5: file の Range 配信(206) ---
	fileURL := urlf("/api/works/%d/file?path=", workID) + url.QueryEscape("audio/track01.mp3")
	req = httptest.NewRequest(http.MethodGet, fileURL, nil)
	req.Header.Set("Range", "bytes=0-3")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("Range status = %d, want 206", rec.Code)
	}
	if rec.Body.String() != "ABCD" {
		t.Errorf("Range body = %q, want ABCD", rec.Body.String())
	}

	// --- ステップ6: plays 記録 ---
	rec = postJSONE2E(t, h, urlf("/api/works/%d/plays", workID), `{"path":"audio/track01.mp3"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("plays status = %d", rec.Code)
	}

	// --- ステップ7: history 確認 ---
	rec = getE2E(t, h, "/api/history")
	var histRes struct {
		Items []struct {
			Work struct {
				ID int64 `json:"id"`
			} `json:"work"`
			LastFilePath string `json:"last_file_path"`
			PlayCount    int    `json:"play_count"`
		} `json:"items"`
	}
	json.Unmarshal(rec.Body.Bytes(), &histRes)
	if len(histRes.Items) != 1 ||
		histRes.Items[0].Work.ID != workID ||
		histRes.Items[0].LastFilePath != "audio/track01.mp3" ||
		histRes.Items[0].PlayCount != 1 {
		t.Errorf("history = %+v", histRes.Items)
	}
}

// ---- E2E 用ヘルパー ----------------------------------------------------------

func writeE2EPNG(t *testing.T, path string, w, h int) {
	t.Helper()
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

func postE2E(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func getE2E(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func postJSONE2E(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func multipartCSVReq(t *testing.T, csvContent string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "import.csv")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(csvContent))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/import/csv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}
