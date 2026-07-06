package api

import (
	"database/sql"
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

// NULL フィールドはキーを省略せず明示的に null で返ること(issue #57)。
// 以前は setIfValid でキー自体を省略していたが、フロントの `string | null` 型定義との
// 契約を一致させるため typed struct + 明示 null に変更した。
func TestGetWorkNullFieldsAreExplicitNull(t *testing.T) {
	h, _, id := newTestServer(t)
	// newTestServer は circle/series 等を設定していない
	w := doGet(t, h, urlf("/api/works/%d", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	// 生 JSON 文字列としても明示 null になっていること(キー省略の再発を検出しやすくする)。
	for _, needle := range []string{
		`"circle":null`, `"series_name":null`, `"purchase_date":null`,
		`"work_type":null`, `"age_rating":null`, `"file_format":null`,
		`"file_size_text":null`, `"thumbnail_url":null`,
	} {
		if !strings.Contains(w.Body.String(), needle) {
			t.Errorf("レスポンスに %q が含まれない: %s", needle, w.Body.String())
		}
	}

	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"circle", "series_name", "purchase_date",
		"work_type", "age_rating", "file_format", "file_size_text", "thumbnail_url"} {
		v, ok := raw[key]
		if !ok {
			t.Errorf("NULL フィールド %q のキー自体が省略された", key)
		}
		if v != nil {
			t.Errorf("NULL フィールド %q = %v, want null", key, v)
		}
	}
	// rj_number は設定済みなので値ありで存在する
	if v, ok := raw["rj_number"]; !ok || v == nil {
		t.Error("rj_number が省略/null になった")
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

// PATCH title を TrimSpace 後に空文字になる場合は 400(作品名を空にできる
// 現状バグの修正、issue #63)。
func TestPatchWorkTitleEmptyRejected(t *testing.T) {
	h, database, id := newTestServer(t)

	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"title":"   "}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空白のみ title status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
	w = doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"title":""}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空文字 title status = %d, want 400, body = %s", w.Code, w.Body.String())
	}

	// title が変更されていないこと
	var title string
	if err := database.QueryRow("SELECT title FROM works WHERE id=?", id).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "テスト作品" {
		t.Errorf("拒否されたはずの title 更新が反映された: %q", title)
	}
}

// PATCH title の前後の空白が TrimSpace されて保存されること。
func TestPatchWorkTitleTrimmed(t *testing.T) {
	h, database, id := newTestServer(t)

	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"title":"  新タイトル  "}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var title string
	if err := database.QueryRow("SELECT title FROM works WHERE id=?", id).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "新タイトル" {
		t.Errorf("title = %q, want trim 済みの \"新タイトル\"", title)
	}
}

// PATCH title が 200 rune を超える場合は 400。
func TestPatchWorkTitleTooLong(t *testing.T) {
	h, _, id := newTestServer(t)
	long := strings.Repeat("あ", 201)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"title":"`+long+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("201文字 title status = %d, want 400", w.Code)
	}
}

// PATCH circle に空文字を指定すると「サークル情報の削除」として NULL になること
// (判断: circle は元々 NULL 許容のフィールドで、CSV 側も空文字を nullIfEmpty で
// NULL 化している。フロントに既存の編集 UI はまだ無いため、既存のドメイン表現
// 〈「サークル無し」= NULL〉に揃えるのが自然と判断した)。
// 削除であっても Circle キー自体は非 nil で PATCH されているので manually_edited は立つ
// (CSV 再インポートで復元されないようにする、issue #64 の意図を維持)。
func TestPatchWorkCircleEmptyClears(t *testing.T) {
	h, database, id := newTestServer(t)
	if _, err := database.Exec("UPDATE works SET circle='元サークル' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}

	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"circle":""}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var circle sql.NullString
	var manuallyEdited int
	if err := database.QueryRow("SELECT circle, manually_edited FROM works WHERE id=?", id).
		Scan(&circle, &manuallyEdited); err != nil {
		t.Fatal(err)
	}
	if circle.Valid {
		t.Errorf("circle が NULL になっていない: %q", circle.String)
	}
	if manuallyEdited != 1 {
		t.Errorf("manually_edited = %d, want 1(circle 削除も手動編集扱い)", manuallyEdited)
	}
}

// PATCH circle が 200 rune を超える場合は 400。
func TestPatchWorkCircleTooLong(t *testing.T) {
	h, _, id := newTestServer(t)
	long := strings.Repeat("あ", 201)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"circle":"`+long+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("201文字 circle status = %d, want 400", w.Code)
	}
}

// PATCH title で manually_edited が立つこと(issue #64 案 A)。
func TestPatchWorkTitleSetsManuallyEdited(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"title":"新タイトル"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var manuallyEdited int
	if err := database.QueryRow("SELECT manually_edited FROM works WHERE id=?", id).
		Scan(&manuallyEdited); err != nil {
		t.Fatal(err)
	}
	if manuallyEdited != 1 {
		t.Errorf("manually_edited = %d, want 1(title PATCH で立つべき)", manuallyEdited)
	}
}

// PATCH circle でも manually_edited が立つこと。
func TestPatchWorkCircleSetsManuallyEdited(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"circle":"新サークル"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	var manuallyEdited int
	if err := database.QueryRow("SELECT manually_edited FROM works WHERE id=?", id).
		Scan(&manuallyEdited); err != nil {
		t.Fatal(err)
	}
	if manuallyEdited != 1 {
		t.Errorf("manually_edited = %d, want 1(circle PATCH で立つべき)", manuallyEdited)
	}
}

// PATCH hidden のみでは manually_edited が立たないこと(title/circle 以外の
// PATCH は「手動編集」として扱わない)。
func TestPatchWorkHiddenOnlyDoesNotSetManuallyEdited(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"hidden":true}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var manuallyEdited int
	if err := database.QueryRow("SELECT manually_edited FROM works WHERE id=?", id).
		Scan(&manuallyEdited); err != nil {
		t.Fatal(err)
	}
	if manuallyEdited != 0 {
		t.Errorf("manually_edited = %d, want 0(hidden PATCH のみでは立たないべき)", manuallyEdited)
	}
}

// PATCH favorite のみでは manually_edited が立たないこと。
func TestPatchWorkFavoriteOnlyDoesNotSetManuallyEdited(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"favorite":true}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var manuallyEdited int
	if err := database.QueryRow("SELECT manually_edited FROM works WHERE id=?", id).
		Scan(&manuallyEdited); err != nil {
		t.Fatal(err)
	}
	if manuallyEdited != 0 {
		t.Errorf("manually_edited = %d, want 0(favorite PATCH のみでは立たないべき)", manuallyEdited)
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

// name が空白のみの場合も TrimSpace 後に空 → 400(issue #63)。
func TestAddTagWhitespaceOnlyRejected(t *testing.T) {
	h, _, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":"   "}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("空白のみ name status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

// name の前後の空白が TrimSpace されて登録されること。
func TestAddTagNameTrimmed(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":"  タグ  "}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Name != "タグ" {
		t.Errorf("レスポンスの name = %q, want trim 済みの \"タグ\"", resp.Name)
	}
	var cnt int
	if err := database.QueryRow("SELECT COUNT(*) FROM tags WHERE name='タグ'").Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Errorf("trim 済み名でタグが登録されていない: count = %d", cnt)
	}
}

// name が 101 文字(rune 数)を超える場合は 400。
func TestAddTagNameTooLong(t *testing.T) {
	h, _, id := newTestServer(t)
	long := strings.Repeat("あ", 101)
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/tags", id), `{"name":"`+long+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("101文字 name status = %d, want 400", w.Code)
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

// GET /api/works の items も NULL フィールド(circle/age_rating/thumbnail_url)を
// 明示的な null で返すこと(キー省略ではない。issue #57)。
// newTestServer が作る作品は circle/age_rating が NULL、thumbnail_path 未設定。
func TestListWorksNullFieldsAreExplicitNull(t *testing.T) {
	h, _, _ := newTestServer(t)

	w := doGet(t, h, "/api/works")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	for _, needle := range []string{`"circle":null`, `"age_rating":null`, `"thumbnail_url":null`} {
		if !strings.Contains(w.Body.String(), needle) {
			t.Errorf("レスポンスに %q が含まれない: %s", needle, w.Body.String())
		}
	}

	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items 数 = %d, want 1", len(body.Items))
	}
	for _, key := range []string{"circle", "age_rating", "thumbnail_url"} {
		v, ok := body.Items[0][key]
		if !ok {
			t.Errorf("NULL フィールド %q のキー自体が省略された", key)
		}
		if v != nil {
			t.Errorf("NULL フィールド %q = %v, want null", key, v)
		}
	}
	if v, ok := body.Items[0]["rj_number"]; !ok || v == nil {
		t.Error("rj_number が省略/null になった")
	}
}

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

// path がパストラバーサル(../)の場合、media.ResolvePath の ErrForbidden を
// 400 にマップして記録を拒否すること(issue #63。ブラウズ系の 403 とは異なり、
// 再生記録 API では「不正な入力」として 400 で返す設計とした)。
func TestRecordPlayTraversalPathRejected(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/plays", id), `{"path":"../../etc/passwd"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("traversal path status = %d, want 400, body = %s", w.Code, w.Body.String())
	}

	// 履歴に記録されていないこと
	var cnt int
	if err := database.QueryRow("SELECT COUNT(*) FROM play_history WHERE work_id=?", id).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Errorf("拒否されたはずの再生が記録された: count = %d", cnt)
	}
}

// path が作品フォルダ内に存在しない場合、media.ResolvePath の ErrNotFound を
// 404 にマップして記録を拒否すること。
func TestRecordPlayNonexistentPathRejected(t *testing.T) {
	h, database, id := newTestServer(t)
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/plays", id), `{"path":"mp3/nonexistent.mp3"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("存在しない path status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
	var cnt int
	if err := database.QueryRow("SELECT COUNT(*) FROM play_history WHERE work_id=?", id).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Errorf("拒否されたはずの再生が記録された: count = %d", cnt)
	}
}

// path が長すぎる場合(1000 byte 超)は 400。
func TestRecordPlayPathTooLong(t *testing.T) {
	h, _, id := newTestServer(t)
	long := strings.Repeat("a", 1001)
	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/plays", id), `{"path":"`+long+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("長すぎる path status = %d, want 400", w.Code)
	}
}

// root_path が NULL(フォルダ未登録)の作品への再生記録は 404 で拒否されること
// (パスの実在を検証できないため)。
func TestRecordPlayNoRootFolderRejected(t *testing.T) {
	h, database, _ := newTestServer(t)
	res, err := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ690000', 'フォルダなし')")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	w := doJSON(t, h, http.MethodPost, urlf("/api/works/%d/plays", id), `{"path":"a.mp3"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("root_path NULL status = %d, want 404, body = %s", w.Code, w.Body.String())
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

// history の work にサムネが無い場合、thumbnail_url はキー省略ではなく明示的な
// null で返ること(issue #57)。
func TestHistoryThumbnailURLExplicitNullWhenMissing(t *testing.T) {
	h, database, id := newTestServer(t)
	// newTestServer の作品は thumbnail_path 未設定
	database.Exec("INSERT INTO play_history (work_id, file_path) VALUES (?, 'a.mp3')", id)

	w := doGet(t, h, "/api/history")
	if !strings.Contains(w.Body.String(), `"thumbnail_url":null`) {
		t.Errorf("thumbnail_url が明示 null になっていない: %s", w.Body.String())
	}

	var body struct {
		Items []struct {
			Work map[string]any `json:"work"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items 数 = %d, want 1", len(body.Items))
	}
	v, ok := body.Items[0].Work["thumbnail_url"]
	if !ok {
		t.Error("thumbnail_url のキー自体が省略された")
	}
	if v != nil {
		t.Errorf("thumbnail_url = %v, want null", v)
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

// ?limit= で件数が絞られ、未指定なら全件返ること(issue #38-3)。
func TestListTagsLimit(t *testing.T) {
	h, database, id := newTestServer(t)
	database.Exec(`INSERT INTO tags (name, category) VALUES
		('あ','custom'), ('い','custom'), ('う','custom')`)
	database.Exec("INSERT INTO work_tags (work_id, tag_id) SELECT ?, id FROM tags", id)

	// 未指定 → 全件(3件)
	w := doGet(t, h, "/api/tags")
	var body struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 3 {
		t.Fatalf("未指定 items 数 = %d, want 3", len(body.Items))
	}

	// limit=1 → 1件のみ
	w = doGet(t, h, "/api/tags?limit=1")
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 1 {
		t.Errorf("limit=1 items 数 = %d, want 1", len(body.Items))
	}

	// limit=0 は 1 にクランプされる
	w = doGet(t, h, "/api/tags?limit=0")
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 1 {
		t.Errorf("limit=0 items 数 = %d, want 1(下限クランプ)", len(body.Items))
	}

	// limit=9999 は 1000 にクランプされる(が母数が3件なので3件のまま)
	w = doGet(t, h, "/api/tags?limit=9999")
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 3 {
		t.Errorf("limit=9999 items 数 = %d, want 3(上限クランプでも母数までしか出ない)", len(body.Items))
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

// ---- お気に入り機能(issue #72) ------------------------------------------------

// PATCH favorite=true でお気に入り登録(favorited_at が入る)、
// favorite=false で解除(NULL に戻る)こと。hidden と同じ流儀。
func TestPatchWorkFavorite(t *testing.T) {
	h, database, id := newTestServer(t)

	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"favorite":true}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("favorite=true status = %d, body = %s", w.Code, w.Body.String())
	}
	var favoritedAt sql.NullString
	if err := database.QueryRow("SELECT favorited_at FROM works WHERE id=?", id).Scan(&favoritedAt); err != nil {
		t.Fatal(err)
	}
	if !favoritedAt.Valid || favoritedAt.String == "" {
		t.Errorf("favorited_at がセットされていない: %+v", favoritedAt)
	}

	// GET でも favorited: true が返る
	getResp := doGet(t, h, urlf("/api/works/%d", id))
	var getBody struct {
		Favorited bool `json:"favorited"`
	}
	json.Unmarshal(getResp.Body.Bytes(), &getBody)
	if !getBody.Favorited {
		t.Error("GET /api/works/{id} の favorited が true にならない")
	}

	// 解除
	w = doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"favorite":false}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("favorite=false status = %d", w.Code)
	}
	favoritedAt = sql.NullString{}
	if err := database.QueryRow("SELECT favorited_at FROM works WHERE id=?", id).Scan(&favoritedAt); err != nil {
		t.Fatal(err)
	}
	if favoritedAt.Valid {
		t.Errorf("favorite=false で favorited_at が NULL に戻っていない: %v", favoritedAt.String)
	}
}

// ---- 評価機能(issue #95) -----------------------------------------------------

func TestPatchWorkRating(t *testing.T) {
	h, database, id := newTestServer(t)

	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"rating":5}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("rating=5 status = %d, body = %s", w.Code, w.Body.String())
	}
	var rating sql.NullInt64
	if err := database.QueryRow("SELECT rating FROM works WHERE id=?", id).Scan(&rating); err != nil {
		t.Fatal(err)
	}
	if !rating.Valid || rating.Int64 != 5 {
		t.Fatalf("rating = %+v, want 5", rating)
	}

	getResp := doGet(t, h, urlf("/api/works/%d", id))
	var getBody struct {
		Rating *int `json:"rating"`
	}
	if err := json.Unmarshal(getResp.Body.Bytes(), &getBody); err != nil {
		t.Fatal(err)
	}
	if getBody.Rating == nil || *getBody.Rating != 5 {
		t.Fatalf("GET /api/works/{id} rating = %v, want 5", getBody.Rating)
	}

	w = doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"rating":null}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("rating=null status = %d, body = %s", w.Code, w.Body.String())
	}
	rating = sql.NullInt64{}
	if err := database.QueryRow("SELECT rating FROM works WHERE id=?", id).Scan(&rating); err != nil {
		t.Fatal(err)
	}
	if rating.Valid {
		t.Fatalf("rating=null で NULL に戻っていない: %+v", rating)
	}
}

func TestPatchWorkRatingRejectsOutOfRange(t *testing.T) {
	h, database, id := newTestServer(t)

	for _, body := range []string{`{"rating":0}`, `{"rating":6}`, `{"rating":-1}`} {
		w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", body, w.Code)
		}
	}

	var rating sql.NullInt64
	if err := database.QueryRow("SELECT rating FROM works WHERE id=?", id).Scan(&rating); err != nil {
		t.Fatal(err)
	}
	if rating.Valid {
		t.Fatalf("不正 rating PATCH で値が入った: %+v", rating)
	}
}

func TestListWorksRatingFieldAndSort(t *testing.T) {
	h, database, id := newTestServer(t)
	res, err := database.Exec("INSERT INTO works (rj_number, title, rating) VALUES ('RJ950001', '星3', 3)")
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := res.LastInsertId()
	res, err = database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ950002', '未評価')")
	if err != nil {
		t.Fatal(err)
	}
	id3, _ := res.LastInsertId()
	if _, err := database.Exec("UPDATE works SET rating=5 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/works?sort=rating&order=desc")
	var body struct {
		Items []struct {
			ID     int64 `json:"id"`
			Rating *int  `json:"rating"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 3 {
		t.Fatalf("items 数 = %d, want 3", len(body.Items))
	}
	if body.Items[0].ID != id || body.Items[1].ID != id2 || body.Items[2].ID != id3 {
		t.Fatalf("rating desc order = %+v, want [%d,%d,%d]", body.Items, id, id2, id3)
	}
	if body.Items[0].Rating == nil || *body.Items[0].Rating != 5 {
		t.Fatalf("先頭 rating = %v, want 5", body.Items[0].Rating)
	}
	if body.Items[2].Rating != nil {
		t.Fatalf("未評価作品 rating = %v, want null", body.Items[2].Rating)
	}
}

// favorite キーを含まない PATCH では favorited_at が変化しないこと
func TestPatchWorkFavoriteUntouchedWhenOmitted(t *testing.T) {
	h, database, id := newTestServer(t)
	doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"favorite":true}`)

	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"title":"新タイトル"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	var favoritedAt sql.NullString
	if err := database.QueryRow("SELECT favorited_at FROM works WHERE id=?", id).Scan(&favoritedAt); err != nil {
		t.Fatal(err)
	}
	if !favoritedAt.Valid {
		t.Error("favorite を指定しない PATCH で favorited_at が消えた")
	}
}

// GET /api/works?favorite=1 でお気に入りのみが返ること。
// 一覧・詳細双方のレスポンスに favorited フィールドが付くことも合わせて確認する。
func TestListWorksFavoriteFilterAndField(t *testing.T) {
	h, database, id := newTestServer(t)
	res, err := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ750001', '非お気に入り')")
	if err != nil {
		t.Fatal(err)
	}
	otherID, _ := res.LastInsertId()

	doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"favorite":true}`)

	// favorite=1 → お気に入りのみ
	w := doGet(t, h, "/api/works?favorite=1")
	var body struct {
		Total int `json:"total"`
		Items []struct {
			ID        int64 `json:"id"`
			Favorited bool  `json:"favorited"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || len(body.Items) != 1 || body.Items[0].ID != id {
		t.Errorf("favorite=1: total=%d items=%+v, want id=%d のみ", body.Total, body.Items, id)
	}
	if !body.Items[0].Favorited {
		t.Error("一覧の favorited が true にならない")
	}

	// 未指定 → 両方(2件)返り、favorited フィールドがそれぞれ正しい
	w = doGet(t, h, "/api/works")
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Total != 2 {
		t.Errorf("未指定 total = %d, want 2", body.Total)
	}
	for _, item := range body.Items {
		want := item.ID == id
		if item.Favorited != want {
			t.Errorf("id=%d favorited=%v, want %v", item.ID, item.Favorited, want)
		}
	}
	_ = otherID
}

// sort=favorited_at: お気に入り登録順(新しい順)で並び、非お気に入りは末尾に来ること
func TestListWorksSortFavoritedAt(t *testing.T) {
	h, database, id := newTestServer(t)
	res, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ760001', '後で登録')")
	id2, _ := res.LastInsertId()
	res, _ = database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ760002', '未登録')")
	id3, _ := res.LastInsertId()

	// id を先に、id2 を後にお気に入り登録(favorited_at の値を明示的にずらす)
	database.Exec("UPDATE works SET favorited_at='2026-01-01 00:00:00' WHERE id=?", id)
	database.Exec("UPDATE works SET favorited_at='2026-01-02 00:00:00' WHERE id=?", id2)

	w := doGet(t, h, "/api/works?sort=favorited_at&order=desc")
	var body struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 3 {
		t.Fatalf("items 数 = %d, want 3", len(body.Items))
	}
	if body.Items[0].ID != id2 || body.Items[1].ID != id {
		t.Errorf("お気に入り登録順(新しい順) = %v, want [%d, %d, ...]", body.Items, id2, id)
	}
	// 非お気に入り(id3)は末尾
	if body.Items[2].ID != id3 {
		t.Errorf("非お気に入りが末尾に来ていない: %v, want 末尾 %d", body.Items, id3)
	}
}

// sort=last_played: 最近再生した順で並び、未再生は末尾に来ること(order 指定に関わらず)
func TestListWorksSortLastPlayed(t *testing.T) {
	h, database, id := newTestServer(t)
	res, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ770001', '再生済み')")
	id2, _ := res.LastInsertId()
	res, _ = database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ770002', '未再生')")
	id3, _ := res.LastInsertId()

	database.Exec("INSERT INTO play_history (work_id, file_path, played_at) VALUES (?, 'a.mp3', '2026-01-01 00:00:00')", id)
	database.Exec("INSERT INTO play_history (work_id, file_path, played_at) VALUES (?, 'b.mp3', '2026-01-05 00:00:00')", id2)

	for _, order := range []string{"desc", "asc"} {
		w := doGet(t, h, "/api/works?sort=last_played&order="+order)
		var body struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Items) != 3 {
			t.Fatalf("order=%s items 数 = %d, want 3", order, len(body.Items))
		}
		// 未再生(id3)は asc/desc どちらでも末尾
		if body.Items[2].ID != id3 {
			t.Errorf("order=%s: 未再生が末尾に来ていない: %v, want 末尾 %d", order, body.Items, id3)
		}
	}

	// desc では直近再生(id2)が先頭
	w := doGet(t, h, "/api/works?sort=last_played&order=desc")
	var body struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Items[0].ID != id2 {
		t.Errorf("last_played desc 先頭 = %d, want %d", body.Items[0].ID, id2)
	}
}

// sort=play_count: 再生回数順で並び、未再生(0回)は末尾に来ること(order 指定に関わらず)
func TestListWorksSortPlayCount(t *testing.T) {
	h, database, id := newTestServer(t)
	res, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ780001', 'よく聴く')")
	id2, _ := res.LastInsertId()
	res, _ = database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ780002', '未再生')")
	id3, _ := res.LastInsertId()

	// id: 1回, id2: 3回
	database.Exec("INSERT INTO play_history (work_id, file_path) VALUES (?, 'a.mp3')", id)
	database.Exec("INSERT INTO play_history (work_id, file_path) VALUES (?, 'a.mp3'), (?, 'a.mp3'), (?, 'a.mp3')", id2, id2, id2)

	for _, order := range []string{"desc", "asc"} {
		w := doGet(t, h, "/api/works?sort=play_count&order="+order)
		var body struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Items) != 3 {
			t.Fatalf("order=%s items 数 = %d, want 3", order, len(body.Items))
		}
		// 未再生(id3)は asc/desc どちらでも末尾
		if body.Items[2].ID != id3 {
			t.Errorf("order=%s: 未再生が末尾に来ていない: %v, want 末尾 %d", order, body.Items, id3)
		}
	}

	// desc では再生回数最多(id2)が先頭
	w := doGet(t, h, "/api/works?sort=play_count&order=desc")
	var body struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Items[0].ID != id2 {
		t.Errorf("play_count desc 先頭 = %d, want %d", body.Items[0].ID, id2)
	}
}
