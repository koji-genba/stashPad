package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// doJSON は JSON ボディ付きのリクエストを送るヘルパー。
func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---- DB クローズ時の 500 応答(各ハンドラのエラー分岐網羅) -------------------

// DB を閉じた状態で各読み取りハンドラが 500 を返すことを確認する。
// これにより respondError(500) のエラー分岐をまとめてカバーする。
func TestHandlersReturn500OnDBError(t *testing.T) {
	h, database, id := newTestServer(t)
	// 先にタグを1件付けておく(クローズ後は SELECT で失敗する)
	database.Exec("INSERT INTO tags (name, category) VALUES ('x','custom')")
	database.Exec("INSERT INTO work_tags (work_id, tag_id) SELECT ?, id FROM tags", id)
	database.Close()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"list", http.MethodGet, "/api/works", ""},
		{"get", http.MethodGet, urlf("/api/works/%d", id), ""},
		{"history", http.MethodGet, "/api/history", ""},
		{"tags", http.MethodGet, "/api/tags", ""},
		{"cleanup", http.MethodPost, "/api/tags/cleanup", ""},
		{"deltag", http.MethodDelete, urlf("/api/works/%d/tags/1", id), ""},
		{"thumb", http.MethodGet, urlf("/api/works/%d/thumbnail", id), ""},
		{"refresh", http.MethodPost, urlf("/api/works/%d/thumbnail/refresh", id), ""},
		{"rebuild", http.MethodPost, "/api/thumbnails/rebuild", ""},
		{"entries", http.MethodGet, urlf("/api/works/%d/entries?path=", id), ""},
		{"file", http.MethodGet, urlf("/api/works/%d/file?path=a.mp3", id), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, h, tc.method, tc.path, tc.body)
			if w.Code != http.StatusInternalServerError {
				t.Errorf("%s: status = %d, want 500", tc.name, w.Code)
			}
		})
	}
}

// EXISTS チェックで DB エラーが起きるハンドラは 500 を返すこと
// (不存在の 404 と区別される)。
func TestExistsCheckErrorReturns500(t *testing.T) {
	h, database, id := newTestServer(t)
	database.Close()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"patch", http.MethodPatch, urlf("/api/works/%d", id), `{"title":"x"}`},
		{"addtag", http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":"y"}`},
		{"play", http.MethodPost, urlf("/api/works/%d/plays", id), `{"path":"a.mp3"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, h, tc.method, tc.path, tc.body)
			if w.Code != http.StatusInternalServerError {
				t.Errorf("%s: status = %d, want 500", tc.name, w.Code)
			}
		})
	}
}

// ---- GET /api/healthz --------------------------------------------------------

func TestHealthz(t *testing.T) {
	h, _, _ := newTestServer(t)
	w := doGet(t, h, "/api/healthz")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

// ---- GET /api/works/{id} -----------------------------------------------------

// 全メタデータ + タグ付きの作品が全フィールドを返し、tags がカテゴリ→名前順であること
func TestGetWorkFullMetadata(t *testing.T) {
	h, database, id := newTestServer(t)

	// メタデータを埋める
	if _, err := database.Exec(`
		UPDATE works SET
			circle='サークルX', series_name='シリーズY', purchase_date='2026/01/01 00:00',
			work_type='ボイス・ASMR', age_rating='R-18', file_format='WAV/MP3',
			file_size_text='1.2GB'
		WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}

	// タグを2件作成・リンク(category と name で順序検証)
	// 期待順: (genre, あ) < (genre, ん) < (voice_actor, 太郎)
	if _, err := database.Exec(`
		INSERT INTO tags (name, category) VALUES
			('ん', 'genre'), ('あ', 'genre'), ('太郎', 'voice_actor')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO work_tags (work_id, tag_id)
		SELECT ?, id FROM tags`, id); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, urlf("/api/works/%d", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		ID           int64  `json:"id"`
		Title        string `json:"title"`
		Circle       string `json:"circle"`
		SeriesName   string `json:"series_name"`
		PurchaseDate string `json:"purchase_date"`
		WorkType     string `json:"work_type"`
		AgeRating    string `json:"age_rating"`
		FileFormat   string `json:"file_format"`
		FileSizeText string `json:"file_size_text"`
		HasFolder    bool   `json:"has_folder"`
		ThumbnailURL string `json:"thumbnail_url"`
		Tags         []struct {
			Name     string `json:"name"`
			Category string `json:"category"`
		} `json:"tags"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Circle != "サークルX" || body.SeriesName != "シリーズY" ||
		body.PurchaseDate != "2026/01/01 00:00" || body.WorkType != "ボイス・ASMR" ||
		body.AgeRating != "R-18" || body.FileFormat != "WAV/MP3" ||
		body.FileSizeText != "1.2GB" {
		t.Errorf("メタデータ不一致: %+v", body)
	}
	if !body.HasFolder {
		t.Error("has_folder が false")
	}
	// thumbnail_path 未設定なので thumbnail_url は付かない
	if body.ThumbnailURL != "" {
		t.Errorf("thumbnail_url = %q, want empty", body.ThumbnailURL)
	}
	// tags はカテゴリ→名前順
	var got []string
	for _, tag := range body.Tags {
		got = append(got, tag.Category+":"+tag.Name)
	}
	want := []string{"genre:あ", "genre:ん", "voice_actor:太郎"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tags 順序 = %v, want %v", got, want)
	}
}

// NULL フィールドはキー自体が省略されること(setIfValid の挙動)
func TestGetWorkOmitsNullFields(t *testing.T) {
	h, _, id := newTestServer(t)
	// newTestServer は circle/series 等を設定していない
	w := doGet(t, h, urlf("/api/works/%d", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"circle", "series_name", "purchase_date",
		"work_type", "age_rating", "file_format", "file_size_text", "thumbnail_url"} {
		if _, ok := raw[key]; ok {
			t.Errorf("NULL フィールド %q がキーとして存在する", key)
		}
	}
	// rj_number は設定済みなので存在する
	if _, ok := raw["rj_number"]; !ok {
		t.Error("rj_number が省略された")
	}
}

// thumbnail_path があれば thumbnail_url が付くこと
func TestGetWorkThumbnailURL(t *testing.T) {
	h, database, id := newTestServer(t)
	if _, err := database.Exec(
		"UPDATE works SET thumbnail_path='/some/path/1.jpg' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	w := doGet(t, h, urlf("/api/works/%d", id))
	var body struct {
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ThumbnailURL != urlf("/api/works/%d/thumbnail", id) {
		t.Errorf("thumbnail_url = %q", body.ThumbnailURL)
	}
}

func TestGetWorkNotFoundAndBadID(t *testing.T) {
	h, _, _ := newTestServer(t)

	w := doGet(t, h, "/api/works/99999")
	if w.Code != http.StatusNotFound {
		t.Errorf("存在しない ID status = %d, want 404", w.Code)
	}

	w = doGet(t, h, "/api/works/abc")
	if w.Code != http.StatusBadRequest {
		t.Errorf("非数値 ID status = %d, want 400", w.Code)
	}
}

// ---- PATCH /api/works/{id} ---------------------------------------------------

func TestPatchWorkTitleOnly(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"title":"新タイトル"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var title string
	var circle interface{}
	if err := database.QueryRow("SELECT title, circle FROM works WHERE id=?", id).
		Scan(&title, &circle); err != nil {
		t.Fatal(err)
	}
	if title != "新タイトル" {
		t.Errorf("title = %q", title)
	}
	if circle != nil {
		t.Errorf("circle が変わった: %v", circle)
	}
}

func TestPatchWorkCircleOnly(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"circle":"新サークル"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	var title, circle string
	if err := database.QueryRow("SELECT title, circle FROM works WHERE id=?", id).
		Scan(&title, &circle); err != nil {
		t.Fatal(err)
	}
	if circle != "新サークル" {
		t.Errorf("circle = %q", circle)
	}
	if title != "テスト作品" {
		t.Errorf("title が変わった: %q", title)
	}
}

func TestPatchWorkBoth(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id),
		`{"title":"T2","circle":"C2"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	var title, circle string
	if err := database.QueryRow("SELECT title, circle FROM works WHERE id=?", id).
		Scan(&title, &circle); err != nil {
		t.Fatal(err)
	}
	if title != "T2" || circle != "C2" {
		t.Errorf("title=%q circle=%q", title, circle)
	}
}

func TestPatchWorkEmptyBody(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	var title string
	if err := database.QueryRow("SELECT title FROM works WHERE id=?", id).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "テスト作品" {
		t.Errorf("空ボディで title が変わった: %q", title)
	}
}

func TestPatchWorkBadJSONAndNotFound(t *testing.T) {
	h, _, id := newTestServer(t)

	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{not json`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("不正 JSON status = %d, want 400", w.Code)
	}

	w = doJSON(t, h, http.MethodPatch, "/api/works/99999", `{"title":"x"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("存在しない ID status = %d, want 404", w.Code)
	}
}

// ---- POST /api/works/{id}/tags & DELETE --------------------------------------

func TestAddTagCreateAndUpsert(t *testing.T) {
	h, database, id := newTestServer(t)

	// 新規タグ作成 + リンク
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":"お気に入り"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var first struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Name != "お気に入り" || first.Category != "custom" {
		t.Errorf("レスポンス = %+v", first)
	}

	// 同名タグを再追加 → 同じ tag ID(upsert)
	w = doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":"お気に入り"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("再追加 status = %d", w.Code)
	}
	var second struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Errorf("再追加で tag ID が変わった: %d → %d", first.ID, second.ID)
	}

	// tags 行は1件だけ
	var tagCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM tags WHERE name='お気に入り'").
		Scan(&tagCount); err != nil {
		t.Fatal(err)
	}
	if tagCount != 1 {
		t.Errorf("tags 行数 = %d, want 1", tagCount)
	}
	// work_tags も1件(冪等)
	var linkCount int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM work_tags WHERE work_id=? AND tag_id=?", id, first.ID).
		Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 1 {
		t.Errorf("work_tags 行数 = %d, want 1", linkCount)
	}
}

// 別作品に同名タグを追加しても tags 行は増えないこと
func TestAddTagSharedAcrossWorks(t *testing.T) {
	h, database, id := newTestServer(t)
	res, err := database.Exec(
		"INSERT INTO works (rj_number, title) VALUES ('RJ000050', '別作品')")
	if err != nil {
		t.Fatal(err)
	}
	otherID, _ := res.LastInsertId()

	doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":"共有"}`)
	doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", otherID), `{"name":"共有"}`)

	var tagCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM tags WHERE name='共有'").
		Scan(&tagCount); err != nil {
		t.Fatal(err)
	}
	if tagCount != 1 {
		t.Errorf("tags 行数 = %d, want 1", tagCount)
	}
	var linkCount int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM work_tags wt JOIN tags t ON t.id=wt.tag_id WHERE t.name='共有'").
		Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 2 {
		t.Errorf("work_tags リンク数 = %d, want 2", linkCount)
	}
}

func TestAddTagErrors(t *testing.T) {
	h, _, id := newTestServer(t)

	// name 空
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":""}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("name 空 status = %d, want 400", w.Code)
	}

	// 不正 JSON
	w = doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{broken`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("不正 JSON status = %d, want 400", w.Code)
	}

	// 存在しない作品
	w = doJSON(t, h, http.MethodPost, "/api/works/99999/tags", `{"name":"x"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("存在しない作品 status = %d, want 404", w.Code)
	}
}

func TestDeleteTag(t *testing.T) {
	h, database, id := newTestServer(t)

	// タグを追加
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":"消す"}`)
	var added struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &added)

	// リンク削除 → 204
	req := httptest.NewRequest(http.MethodDelete,
		urlf("/api/works/%d/tags/%d", id, added.ID), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("削除 status = %d", rec.Code)
	}
	var cnt int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM work_tags WHERE work_id=? AND tag_id=?", id, added.ID).
		Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Errorf("work_tags から消えていない: %d", cnt)
	}

	// 存在しないリンクの削除も 204(冪等)
	req = httptest.NewRequest(http.MethodDelete,
		urlf("/api/works/%d/tags/%d", id, added.ID), nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("冪等削除 status = %d, want 204", rec.Code)
	}
}

func TestDeleteTagBadID(t *testing.T) {
	h, _, id := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete,
		urlf("/api/works/%d/tags/abc", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非数値 tag_id status = %d, want 400", rec.Code)
	}
}

// ---- GET /api/works 未カバー分岐 ---------------------------------------------

// q キーワード検索が title / circle / rj_number それぞれにヒットすること
func TestListWorksKeyword(t *testing.T) {
	h, database, _ := newTestServer(t)
	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle) VALUES
			('RJ888001', 'キーワード対象タイトル', 'ふつうサークル'),
			('RJ888002', 'ふつうタイトル', 'キーワード対象サークル'),
			('RJ888003', 'ふつう2', 'ふつう2')`); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		q    string
		want int
	}{
		{"キーワード対象タイトル", 1}, // title ヒット
		{"キーワード対象サークル", 1}, // circle ヒット
		{"RJ888003", 1},    // rj_number ヒット
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.q, func(t *testing.T) {
			w := doGet(t, h, "/api/works?q="+url.QueryEscape(tc.q))
			if w.Code != http.StatusOK {
				t.Fatalf("q=%q status = %d", tc.q, w.Code)
			}
			var body struct {
				Total int `json:"total"`
			}
			json.Unmarshal(w.Body.Bytes(), &body)
			if body.Total != tc.want {
				t.Errorf("total = %d, want %d", body.Total, tc.want)
			}
		})
	}
}

// tags=1,2 の AND フィルタ + 非数値の無視
func TestListWorksTagsAndFilter(t *testing.T) {
	h, database, firstID := newTestServer(t)
	// 2つの作品を追加
	r2, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ700001', 'W2')")
	id2, _ := r2.LastInsertId()

	// タグ2件
	database.Exec("INSERT INTO tags (name, category) VALUES ('T1','custom'),('T2','custom')")
	var t1, t2 int64
	database.QueryRow("SELECT id FROM tags WHERE name='T1'").Scan(&t1)
	database.QueryRow("SELECT id FROM tags WHERE name='T2'").Scan(&t2)

	// firstID は T1, T2 両方。id2 は T1 のみ
	database.Exec("INSERT INTO work_tags (work_id, tag_id) VALUES (?,?),(?,?),(?,?)",
		firstID, t1, firstID, t2, id2, t1)

	// tags=t1,t2 の AND → firstID のみ
	w := doGet(t, h, urlf("/api/works?tags=%d,%d", t1, t2))
	var body struct {
		Total int `json:"total"`
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Total != 1 || len(body.Items) != 1 || body.Items[0].ID != firstID {
		t.Errorf("AND フィルタ: total=%d items=%+v, want firstID=%d", body.Total, body.Items, firstID)
	}

	// 非数値が混ざっても無視される(tags=t1,abc → t1 のみのフィルタ = firstID,id2 の2件)
	w = doGet(t, h, urlf("/api/works?tags=%d,abc", t1))
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Total != 2 {
		t.Errorf("非数値混在 total = %d, want 2", body.Total)
	}
}

// page/limit ページネーションと limit/sort のフォールバック
func TestListWorksPagination(t *testing.T) {
	h, database, _ := newTestServer(t)
	// newTestServer で1件あるので、さらに数件足す(purchase_date でソート確認)
	for i := 0; i < 5; i++ {
		database.Exec("INSERT INTO works (rj_number, title) VALUES (?, ?)",
			urlf("RJ60000%d", i), urlf("作品%d", i))
	}
	// 合計6件

	// limit=2, page=1
	w := doGet(t, h, "/api/works?limit=2&page=1")
	var body struct {
		Total int `json:"total"`
		Page  int `json:"page"`
		Limit int `json:"limit"`
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Total != 6 || len(body.Items) != 2 || body.Page != 1 || body.Limit != 2 {
		t.Errorf("page1: total=%d items=%d page=%d limit=%d", body.Total, len(body.Items), body.Page, body.Limit)
	}
	firstPageIDs := map[int64]bool{}
	for _, it := range body.Items {
		firstPageIDs[it.ID] = true
	}

	// page=2 は別の内容
	w = doGet(t, h, "/api/works?limit=2&page=2")
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Page != 2 || len(body.Items) != 2 {
		t.Errorf("page2: page=%d items=%d", body.Page, len(body.Items))
	}
	for _, it := range body.Items {
		if firstPageIDs[it.ID] {
			t.Errorf("page2 に page1 と同じ作品 ID=%d が含まれる", it.ID)
		}
	}

	// limit>200 はデフォルト 40 に落ちる
	w = doGet(t, h, "/api/works?limit=999")
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Limit != 40 {
		t.Errorf("limit>200: limit = %d, want 40", body.Limit)
	}

	// 不正 sort はデフォルト purchase_date に落ちる(エラーにならず 200)
	w = doGet(t, h, "/api/works?sort=evil")
	if w.Code != http.StatusOK {
		t.Errorf("不正 sort status = %d, want 200", w.Code)
	}
}

// ---- entries 未カバー分岐 ----------------------------------------------------

// path がファイルを指す → 400
func TestEntriesPathIsFile(t *testing.T) {
	h, _, id := newTestServer(t)
	w := doGet(t, h, urlf("/api/works/%d/entries?path=", id)+url.QueryEscape("表紙.jpg"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("ファイル指定 status = %d, want 400", w.Code)
	}
}

// root_path NULL の作品 → 404
func TestEntriesRootPathNull(t *testing.T) {
	h, database, _ := newTestServer(t)
	res, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ710000', 'フォルダなし')")
	id, _ := res.LastInsertId()
	w := doGet(t, h, urlf("/api/works/%d/entries?path=", id))
	if w.Code != http.StatusNotFound {
		t.Errorf("root_path NULL status = %d, want 404", w.Code)
	}
}

// 存在しないサブパス → 404
func TestEntriesMissingSubpath(t *testing.T) {
	h, _, id := newTestServer(t)
	w := doGet(t, h, urlf("/api/works/%d/entries?path=", id)+url.QueryEscape("nonexistent"))
	if w.Code != http.StatusNotFound {
		t.Errorf("存在しないサブパス status = %d, want 404", w.Code)
	}
}

// 存在しない作品 → 404
func TestEntriesWorkNotFound(t *testing.T) {
	h, _, _ := newTestServer(t)
	w := doGet(t, h, "/api/works/99999/entries?path=")
	if w.Code != http.StatusNotFound {
		t.Errorf("存在しない作品 status = %d, want 404", w.Code)
	}
}

// parent フィールドの計算(ネスト時)
func TestEntriesParentField(t *testing.T) {
	h, _, id := newTestServer(t)

	// ルート: parent は空
	w := doGet(t, h, urlf("/api/works/%d/entries?path=", id))
	var body struct {
		Path   string `json:"path"`
		Parent string `json:"parent"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Parent != "" {
		t.Errorf("ルートの parent = %q, want empty", body.Parent)
	}

	// mp3 配下: parent は空(1階層なので filepath.Dir("mp3")="." → "")
	w = doGet(t, h, urlf("/api/works/%d/entries?path=mp3", id))
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Parent != "" {
		t.Errorf("mp3 の parent = %q, want empty", body.Parent)
	}
	if body.Path != "mp3" {
		t.Errorf("path = %q, want mp3", body.Path)
	}
}

// ネスト時の parent 計算(深い階層)
func TestEntriesNestedParent(t *testing.T) {
	h, database, _ := newTestServer(t)
	root := makeWorkDir(t, database, "RJ720000", map[string]string{
		"a/b/c/leaf.txt": "x",
	})
	_ = root
	var id int64
	database.QueryRow("SELECT id FROM works WHERE rj_number='RJ720000'").Scan(&id)

	w := doGet(t, h, urlf("/api/works/%d/entries?path=a/b/c", id))
	var body struct {
		Parent string `json:"parent"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Parent != "a/b" {
		t.Errorf("a/b/c の parent = %q, want a/b", body.Parent)
	}
}

// ---- file 未カバー分岐 -------------------------------------------------------

// path 空 → 400
func TestFileEmptyPath(t *testing.T) {
	h, _, id := newTestServer(t)
	w := doGet(t, h, urlf("/api/works/%d/file?path=", id))
	if w.Code != http.StatusBadRequest {
		t.Errorf("path 空 status = %d, want 400", w.Code)
	}
}

// path がディレクトリ → 400
func TestFilePathIsDir(t *testing.T) {
	h, _, id := newTestServer(t)
	w := doGet(t, h, urlf("/api/works/%d/file?path=mp3", id))
	if w.Code != http.StatusBadRequest {
		t.Errorf("ディレクトリ指定 status = %d, want 400", w.Code)
	}
}

// root_path NULL → 404
func TestFileRootPathNull(t *testing.T) {
	h, database, _ := newTestServer(t)
	res, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ730000', 'なし')")
	id, _ := res.LastInsertId()
	w := doGet(t, h, urlf("/api/works/%d/file?path=", id)+url.QueryEscape("x.mp3"))
	if w.Code != http.StatusNotFound {
		t.Errorf("root_path NULL status = %d, want 404", w.Code)
	}
}

// GET の Content-Type 検証(mp3 → audio/mpeg)
func TestFileContentType(t *testing.T) {
	h, _, id := newTestServer(t)
	w := doGet(t, h, urlf("/api/works/%d/file?path=", id)+url.QueryEscape("mp3/01_intro.mp3"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "audio/mpeg") {
		t.Errorf("Content-Type = %q, want audio/mpeg", ct)
	}
}

// HEAD リクエストで Content-Length 付き・ボディなしの応答が返ること
// (http.ServeContent が HEAD を処理する。プレイヤーの事前メタデータ取得に対応)
func TestFileHead(t *testing.T) {
	h, _, id := newTestServer(t)
	req := httptest.NewRequest(http.MethodHead,
		urlf("/api/works/%d/file?path=", id)+url.QueryEscape("mp3/01_intro.mp3"), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", rec.Code)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "10" {
		t.Errorf("Content-Length = %q, want 10", cl)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD のボディが空でない: %q", rec.Body.String())
	}
}

// HEAD はサムネイルエンドポイントでも受け付けること
func TestThumbnailHead(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodHead, urlf("/api/works/%d/thumbnail", id), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD thumbnail status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD のボディが空でない: %q", rec.Body.String())
	}
}

// ---- plays 未カバー分岐 ------------------------------------------------------

func TestRecordPlayErrors(t *testing.T) {
	h, _, id := newTestServer(t)

	// path 空
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/plays", id), `{"path":""}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("path 空 status = %d, want 400", w.Code)
	}

	// 不正 JSON
	w = doJSON(t, h, http.MethodPost, urlf("/api/works/%d/plays", id), `{broken`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("不正 JSON status = %d, want 400", w.Code)
	}

	// 存在しない作品
	w = doJSON(t, h, http.MethodPost, "/api/works/99999/plays", `{"path":"x.mp3"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("存在しない作品 status = %d, want 404", w.Code)
	}
}

// ---- history 集計・順序・ページパラメータ ------------------------------------

func TestHistoryAggregateAndOrder(t *testing.T) {
	h, database, id := newTestServer(t)
	// 2作品目
	res, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ740000', '作品B')")
	id2, _ := res.LastInsertId()

	// id を複数回再生(play_count 集計)、id2 を後から再生(last_played_at が新しい)
	database.Exec("INSERT INTO play_history (work_id, file_path, played_at) VALUES (?, 'a.mp3', '2026-01-01 00:00:00')", id)
	database.Exec("INSERT INTO play_history (work_id, file_path, played_at) VALUES (?, 'b.mp3', '2026-01-02 00:00:00')", id)
	database.Exec("INSERT INTO play_history (work_id, file_path, played_at) VALUES (?, 'c.mp3', '2026-01-03 00:00:00')", id2)

	w := doGet(t, h, "/api/history")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Page  int `json:"page"`
		Items []struct {
			Work struct {
				ID int64 `json:"id"`
			} `json:"work"`
			PlayCount int `json:"play_count"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 2 {
		t.Fatalf("items 数 = %d, want 2", len(body.Items))
	}
	// last_played_at 降順 → id2 が先頭
	if body.Items[0].Work.ID != id2 {
		t.Errorf("先頭 = %d, want %d(last_played_at 降順)", body.Items[0].Work.ID, id2)
	}
	// id の play_count は2
	for _, it := range body.Items {
		if it.Work.ID == id && it.PlayCount != 2 {
			t.Errorf("id=%d play_count = %d, want 2", id, it.PlayCount)
		}
	}
}

// history の work に thumbnail_path があれば thumbnail_url が付くこと
func TestHistoryThumbnailURL(t *testing.T) {
	h, database, id := newTestServer(t)
	database.Exec("UPDATE works SET thumbnail_path='/x/1.jpg' WHERE id=?", id)
	database.Exec("INSERT INTO play_history (work_id, file_path) VALUES (?, 'a.mp3')", id)

	w := doGet(t, h, "/api/history")
	var body struct {
		Items []struct {
			Work struct {
				ThumbnailURL string `json:"thumbnail_url"`
			} `json:"work"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 1 || body.Items[0].Work.ThumbnailURL != urlf("/api/works/%d/thumbnail", id) {
		t.Errorf("history thumbnail_url = %+v", body.Items)
	}
}

// ---- GET /api/tags の category / q フィルタ -----------------------------------

func TestListTagsFilters(t *testing.T) {
	h, database, id := newTestServer(t)
	database.Exec(`INSERT INTO tags (name, category) VALUES
		('癒し','genre'), ('耳かき','detail_genre'), ('声優太郎','voice_actor')`)
	// 全タグを作品にリンク(work_count > 0 にする)
	database.Exec("INSERT INTO work_tags (work_id, tag_id) SELECT ?, id FROM tags", id)

	// category=genre で1件
	w := doGet(t, h, "/api/tags?category=genre")
	var body struct {
		Items []struct {
			Name     string `json:"name"`
			Category string `json:"category"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 1 || body.Items[0].Name != "癒し" {
		t.Errorf("category=genre = %+v, want [癒し]", body.Items)
	}

	// q=声優 で部分一致1件
	w = doGet(t, h, "/api/tags?q=声優")
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 1 || body.Items[0].Name != "声優太郎" {
		t.Errorf("q=声優 = %+v", body.Items)
	}
}

// q に含まれる % がワイルドカード展開されずリテラル一致のみヒットすることを検証する(issue #50)。
func TestListTagsLikeSpecialChars(t *testing.T) {
	h, database, id := newTestServer(t)
	database.Exec(`INSERT INTO tags (name, category) VALUES
		('100%OFFタグ','custom'), ('100XOFFタグ','custom')`)
	database.Exec("INSERT INTO work_tags (work_id, tag_id) SELECT ?, id FROM tags", id)

	w := doGet(t, h, "/api/tags?q="+url.QueryEscape("100%OFF"))
	var body struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 1 || body.Items[0].Name != "100%OFFタグ" {
		t.Errorf("q=100%%OFF items = %+v, want [100%%OFFタグ]", body.Items)
	}
}

// last_file_path が最新の再生履歴のファイルパスを返すことを確認する。
// 同一作品に古いファイルと新しいファイルを記録し、last_file_path が新しい方であることを検証。
func TestHistoryLastFilePathIsLatest(t *testing.T) {
	h, database, id := newTestServer(t)

	// 古い再生履歴を挿入
	if _, err := database.Exec(
		"INSERT INTO play_history (work_id, file_path, played_at) VALUES (?, 'a.mp3', '2026-01-01 00:00:00')", id,
	); err != nil {
		t.Fatal(err)
	}
	// 新しい再生履歴を挿入
	if _, err := database.Exec(
		"INSERT INTO play_history (work_id, file_path, played_at) VALUES (?, 'b.mp3', '2026-01-02 00:00:00')", id,
	); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/history")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var body struct {
		Items []struct {
			LastFilePath string `json:"last_file_path"`
			Work         struct {
				ID int64 `json:"id"`
			} `json:"work"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items 数 = %d, want 1", len(body.Items))
	}
	// last_file_path は最新の再生履歴のファイルパス(b.mp3)であること
	if body.Items[0].LastFilePath != "b.mp3" {
		t.Errorf("last_file_path = %q, want b.mp3", body.Items[0].LastFilePath)
	}
}

// history の page パラメータ(不正値はデフォルト 1)
func TestHistoryPageParam(t *testing.T) {
	h, _, _ := newTestServer(t)

	w := doGet(t, h, "/api/history?page=abc")
	var body struct {
		Page int `json:"page"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Page != 1 {
		t.Errorf("不正 page = %d, want 1", body.Page)
	}

	w = doGet(t, h, "/api/history?page=3")
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Page != 3 {
		t.Errorf("page=3 = %d, want 3", body.Page)
	}
}
