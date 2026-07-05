package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koji-genba/stashpad/backend/internal/config"
	"github.com/koji-genba/stashpad/backend/internal/db"
)

// exportMetadataWork は GET /api/export の works 要素をテストで受け取るための型。
type exportMetadataWork struct {
	RJNumber       *string  `json:"rj_number"`
	RootPath       *string  `json:"root_path"`
	Title          string   `json:"title"`
	Circle         *string  `json:"circle"`
	ManuallyEdited bool     `json:"manually_edited"`
	Hidden         bool     `json:"hidden"`
	FavoritedAt    *string  `json:"favorited_at"`
	CustomTags     []string `json:"custom_tags"`
}

type exportMetadataResponse struct {
	Version    int                  `json:"version"`
	ExportedAt string               `json:"exported_at"`
	Works      []exportMetadataWork `json:"works"`
}

type importMetadataResponseBody struct {
	Matched   int      `json:"matched"`
	Skipped   int      `json:"skipped"`
	TagsAdded int      `json:"tags_added"`
	Errors    []string `json:"errors"`
}

// ---- GET /api/export ----------------------------------------------------------

// ユーザー付与メタデータ(カスタムタグ・お気に入り・非表示・手動編集)を1つ以上
// 持つ作品だけが出力され、CSV 由来タグ(genre 等)は custom_tags に含まれず、
// Content-Disposition も付与されること(issue #78)。
func TestExportMetadataOnlyIncludesWorksWithUserMetadata(t *testing.T) {
	h, database, _ := newTestServer(t)
	// newTestServer 既定の作品(RJ000001)はメタデータなしなので出力対象外のはず

	// カスタムタグを持つ作品(+ CSV 由来タグも混ぜて custom のみ出ることを確認)
	resTag, err := database.Exec(
		"INSERT INTO works (rj_number, title) VALUES ('RJ100001', 'カスタムタグ作品')")
	if err != nil {
		t.Fatal(err)
	}
	tagWorkID, _ := resTag.LastInsertId()
	if _, err := database.Exec(
		"INSERT INTO tags (name, category) VALUES ('睡眠用','custom'), ('R-15','genre')"); err != nil {
		t.Fatal(err)
	}
	var customTagID, genreTagID int64
	database.QueryRow("SELECT id FROM tags WHERE name='睡眠用'").Scan(&customTagID)
	database.QueryRow("SELECT id FROM tags WHERE name='R-15'").Scan(&genreTagID)
	if _, err := database.Exec(
		"INSERT INTO work_tags (work_id, tag_id) VALUES (?,?),(?,?)",
		tagWorkID, customTagID, tagWorkID, genreTagID); err != nil {
		t.Fatal(err)
	}

	// お気に入り作品
	if _, err := database.Exec(
		"INSERT INTO works (rj_number, title, favorited_at) VALUES ('RJ100002', 'お気に入り作品', '2026-07-01 12:00:00')"); err != nil {
		t.Fatal(err)
	}

	// 非表示作品
	if _, err := database.Exec(
		"INSERT INTO works (rj_number, title, hidden) VALUES ('RJ100003', '非表示作品', 1)"); err != nil {
		t.Fatal(err)
	}

	// 手動編集済み作品
	if _, err := database.Exec(
		"INSERT INTO works (rj_number, title, circle, manually_edited) VALUES ('RJ100004', '手動編集作品', 'サークルZ', 1)"); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/export")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	cd := w.Header().Get("Content-Disposition")
	wantPrefix := `attachment; filename="stashpad-metadata-` + time.Now().Format("20060102")
	if !strings.HasPrefix(cd, wantPrefix) {
		t.Errorf("Content-Disposition = %q, want prefix %q", cd, wantPrefix)
	}

	var body exportMetadataResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Version != 1 {
		t.Errorf("version = %d, want 1", body.Version)
	}
	if body.ExportedAt == "" {
		t.Error("exported_at が空")
	}

	byRJ := map[string]exportMetadataWork{}
	for _, item := range body.Works {
		if item.RJNumber != nil {
			byRJ[*item.RJNumber] = item
		}
	}
	if _, ok := byRJ["RJ000001"]; ok {
		t.Error("メタデータなしの作品(RJ000001)が出力された")
	}
	for _, rj := range []string{"RJ100001", "RJ100002", "RJ100003", "RJ100004"} {
		if _, ok := byRJ[rj]; !ok {
			t.Errorf("%s が出力対象に含まれない", rj)
		}
	}

	if item := byRJ["RJ100001"]; len(item.CustomTags) != 1 || item.CustomTags[0] != "睡眠用" {
		t.Errorf("custom_tags = %v, want [睡眠用]のみ(genre タグは含まれない)", item.CustomTags)
	}
	if item := byRJ["RJ100002"]; item.FavoritedAt == nil || *item.FavoritedAt != "2026-07-01 12:00:00" {
		t.Errorf("favorited_at = %v, want 2026-07-01 12:00:00", item.FavoritedAt)
	}
	if item := byRJ["RJ100003"]; !item.Hidden {
		t.Error("hidden = false, want true")
	}
	if item := byRJ["RJ100004"]; !item.ManuallyEdited || item.Circle == nil || *item.Circle != "サークルZ" {
		t.Errorf("manually_edited/circle 不一致: edited=%v circle=%v", item.ManuallyEdited, item.Circle)
	}
}

// ---- ラウンドトリップ: エクスポート → 新DB(スキャン相当)→ インポート -----------

func TestImportMetadataRoundTrip(t *testing.T) {
	h1, database1, _ := newTestServer(t)

	res, err := database1.Exec(
		`INSERT INTO works (rj_number, title, circle, root_path, hidden, favorited_at, manually_edited)
		 VALUES (?,?,?,?,?,?,?)`,
		"RJ200001", "手動タイトル", "元サークル", "/media/RJ200001_元フォルダ", 1, "2026-06-01 09:00:00", 1)
	if err != nil {
		t.Fatal(err)
	}
	workID, _ := res.LastInsertId()
	if _, err := database1.Exec(
		"INSERT INTO tags (name, category) VALUES ('よく聴く','custom')"); err != nil {
		t.Fatal(err)
	}
	var tagID int64
	database1.QueryRow("SELECT id FROM tags WHERE name='よく聴く'").Scan(&tagID)
	if _, err := database1.Exec(
		"INSERT INTO work_tags (work_id, tag_id) VALUES (?,?)", workID, tagID); err != nil {
		t.Fatal(err)
	}

	exportRec := doGet(t, h1, "/api/export")
	if exportRec.Code != http.StatusOK {
		t.Fatalf("export status = %d", exportRec.Code)
	}
	exportedBody := exportRec.Body.String()

	// 新しい DB(スキャン相当。works 行だけ再現し、メタデータは一切無い)
	tmp2 := t.TempDir()
	database2, err := db.Open(filepath.Join(tmp2, "test2.db"))
	if err != nil {
		t.Fatalf("DB オープン失敗: %v", err)
	}
	t.Cleanup(func() { database2.Close() })
	if _, err := database2.Exec(
		"INSERT INTO works (rj_number, title, root_path) VALUES (?,?,?)",
		"RJ200001", "スキャン直後タイトル", "/media/RJ200001_元フォルダ"); err != nil {
		t.Fatal(err)
	}

	cfg2 := &config.Config{LibraryRoots: []string{tmp2}, DataDir: tmp2, Addr: ":0"}
	h2 := New(database2, cfg2).Router()

	importReq := httptest.NewRequest(http.MethodPost, "/api/import/metadata", strings.NewReader(exportedBody))
	importReq.Header.Set("Content-Type", "application/json")
	importRec := httptest.NewRecorder()
	h2.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", importRec.Code, importRec.Body.String())
	}

	var result importMetadataResponseBody
	if err := json.Unmarshal(importRec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Skipped != 0 || result.TagsAdded != 1 || len(result.Errors) != 0 {
		t.Errorf("result = %+v", result)
	}

	var title, circle string
	var hidden, manuallyEdited int
	var favoritedAt string
	if err := database2.QueryRow(
		"SELECT title, circle, hidden, manually_edited, favorited_at FROM works WHERE rj_number='RJ200001'").
		Scan(&title, &circle, &hidden, &manuallyEdited, &favoritedAt); err != nil {
		t.Fatal(err)
	}
	if title != "手動タイトル" || circle != "元サークル" || hidden != 1 || manuallyEdited != 1 ||
		favoritedAt != "2026-06-01 09:00:00" {
		t.Errorf("復元後: title=%q circle=%q hidden=%d manually_edited=%d favorited_at=%q",
			title, circle, hidden, manuallyEdited, favoritedAt)
	}

	var tagCount int
	if err := database2.QueryRow(`
		SELECT COUNT(*) FROM work_tags wt
		JOIN tags t ON t.id=wt.tag_id
		JOIN works w ON w.id=wt.work_id
		WHERE w.rj_number='RJ200001' AND t.name='よく聴く' AND t.category='custom'`).Scan(&tagCount); err != nil {
		t.Fatal(err)
	}
	if tagCount != 1 {
		t.Errorf("custom タグが復元されていない: count=%d", tagCount)
	}
}

// ---- 照合: rj_number 一致 / RJ 無し root_path 一致 / どちらも不一致 ----------------

func TestImportMetadataMatching(t *testing.T) {
	h, database, _ := newTestServer(t)

	if _, err := database.Exec(
		"INSERT INTO works (rj_number, title) VALUES ('RJ300001', 'RJ有り作品')"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		"INSERT INTO works (rj_number, title, root_path) VALUES (NULL, 'RJ無し作品', '/media/rjnone_フォルダ')"); err != nil {
		t.Fatal(err)
	}

	body := `{
		"version": 1,
		"exported_at": "2026-07-05T00:00:00Z",
		"works": [
			{"rj_number": "RJ300001", "root_path": null, "title": "x", "circle": null,
			 "manually_edited": false, "hidden": true, "favorited_at": null, "custom_tags": []},
			{"rj_number": null, "root_path": "/media/rjnone_フォルダ", "title": "y", "circle": null,
			 "manually_edited": false, "hidden": true, "favorited_at": null, "custom_tags": []},
			{"rj_number": "RJ999999", "root_path": null, "title": "z", "circle": null,
			 "manually_edited": false, "hidden": true, "favorited_at": null, "custom_tags": []}
		]
	}`

	rec := doJSON(t, h, http.MethodPost, "/api/import/metadata", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result importMetadataResponseBody
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Matched != 2 || result.Skipped != 1 {
		t.Errorf("matched=%d skipped=%d, want 2/1", result.Matched, result.Skipped)
	}

	var hidden1, hidden2 int
	if err := database.QueryRow("SELECT hidden FROM works WHERE rj_number='RJ300001'").Scan(&hidden1); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("SELECT hidden FROM works WHERE root_path='/media/rjnone_フォルダ'").Scan(&hidden2); err != nil {
		t.Fatal(err)
	}
	if hidden1 != 1 || hidden2 != 1 {
		t.Errorf("hidden1=%d hidden2=%d, want both 1", hidden1, hidden2)
	}
}

// ---- 加算のみ: hidden=false のエクスポートを hidden=1 の作品に食わせても 1 のまま ---

func TestImportMetadataAdditiveOnlyHiddenNeverClears(t *testing.T) {
	h, database, id := newTestServer(t)
	if _, err := database.Exec("UPDATE works SET hidden=1 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}

	// newTestServer の既定作品は RJ000001
	body := `{"version":1,"exported_at":"2026-07-05T00:00:00Z","works":[
		{"rj_number":"RJ000001","root_path":null,"title":"テスト作品","circle":null,
		 "manually_edited":false,"hidden":false,"favorited_at":null,"custom_tags":[]}
	]}`
	rec := doJSON(t, h, http.MethodPost, "/api/import/metadata", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var hidden int
	if err := database.QueryRow("SELECT hidden FROM works WHERE id=?", id).Scan(&hidden); err != nil {
		t.Fatal(err)
	}
	if hidden != 1 {
		t.Errorf("hidden = %d, want 1(hidden=false のインポートで解除されてはいけない)", hidden)
	}
}

// favorited_at はエクスポート値が非 null のときだけ SET され、既存が非 NULL でも上書きされる
// (エクスポート時刻の方が「登録順」として正しいとする設計判断)。
func TestImportMetadataFavoritedAtOverwritesExisting(t *testing.T) {
	h, database, id := newTestServer(t)
	if _, err := database.Exec("UPDATE works SET favorited_at='2020-01-01 00:00:00' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}

	body := `{"version":1,"exported_at":"2026-07-05T00:00:00Z","works":[
		{"rj_number":"RJ000001","root_path":null,"title":"テスト作品","circle":null,
		 "manually_edited":false,"hidden":false,"favorited_at":"2026-07-01 09:00:00","custom_tags":[]}
	]}`
	rec := doJSON(t, h, http.MethodPost, "/api/import/metadata", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var favoritedAt string
	if err := database.QueryRow("SELECT favorited_at FROM works WHERE id=?", id).Scan(&favoritedAt); err != nil {
		t.Fatal(err)
	}
	if favoritedAt != "2026-07-01 09:00:00" {
		t.Errorf("favorited_at = %q, want overwritten value", favoritedAt)
	}
}

// 同一リクエスト内で同じタグ名が重複しても tags_added は 1 件のみ加算される。
func TestImportMetadataTagsAddedIdempotent(t *testing.T) {
	h, _, _ := newTestServer(t)
	body := `{"version":1,"exported_at":"2026-07-05T00:00:00Z","works":[
		{"rj_number":"RJ000001","root_path":null,"title":"テスト作品","circle":null,
		 "manually_edited":false,"hidden":false,"favorited_at":null,"custom_tags":["睡眠用","睡眠用"]}
	]}`
	rec := doJSON(t, h, http.MethodPost, "/api/import/metadata", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result importMetadataResponseBody
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TagsAdded != 1 {
		t.Errorf("tags_added = %d, want 1(重複タグ名は1件のみ加算)", result.TagsAdded)
	}
}

// ---- version 不正 / JSON 不正 → 400 -------------------------------------------

func TestImportMetadataInvalidVersion(t *testing.T) {
	h, _, _ := newTestServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/import/metadata", `{"version":2,"works":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("version 不正 status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestImportMetadataInvalidJSON(t *testing.T) {
	h, _, _ := newTestServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/import/metadata", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("不正 JSON status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// TestImportMetadataRejectsTrailingData は、正しい JSON オブジェクトの後に
// 余剰データが連結されたボディ(例: `{...}{...}`)を 400 で拒否することをテスト
// (PR #89 レビュー指摘)。json.Decoder は Decode 後に先頭のオブジェクトだけを
// 読んで成功したように見えるため、Token() で io.EOF を確認する必要がある。
func TestImportMetadataRejectsTrailingData(t *testing.T) {
	h, _, _ := newTestServer(t)
	body := `{"version":1,"exported_at":"2026-07-05T00:00:00Z","works":[]}{"version":1,"exported_at":"2026-07-05T00:00:00Z","works":[]}`
	rec := doJSON(t, h, http.MethodPost, "/api/import/metadata", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("連結 JSON status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}
