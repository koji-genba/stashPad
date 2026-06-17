package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// ---- 作品非表示機能(issue #10)のテスト ----------------------------------------
//
// t-wada 流 TDD: 先にテストを書き、最小実装で通し、リファクタする。
// 非表示作品は「存在しない扱い」: 検索・一覧・タグ集計・サークル集計・履歴に出ない。

// TestHiddenWorkExcludedFromList は非表示作品がデフォルトの作品一覧から除外されることを検証する。
func TestHiddenWorkExcludedFromList(t *testing.T) {
	h, database, id := newTestServer(t)

	// id を非表示にする
	if _, err := database.Exec("UPDATE works SET hidden=1 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/works")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Total int              `json:"total"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// 非表示作品は含まれないので 0 件
	if body.Total != 0 {
		t.Errorf("非表示作品が一覧に含まれている: total = %d, want 0", body.Total)
	}
}

// TestHiddenParamOne は ?hidden=1 で非表示作品のみ返すことを検証する(設定画面用)。
func TestHiddenParamOne(t *testing.T) {
	h, database, id := newTestServer(t)

	// 可視作品を1件追加、テスト作品を非表示にする
	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, root_path) VALUES ('RJ000099', '可視作品', '/tmp/fake99')
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("UPDATE works SET hidden=1 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}

	// ?hidden=1 → 非表示作品のみ返る
	w := doGet(t, h, "/api/works?hidden=1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Total int `json:"total"`
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 {
		t.Errorf("?hidden=1: total = %d, want 1", body.Total)
	}
	if len(body.Items) != 1 || body.Items[0].ID != id {
		t.Errorf("?hidden=1: items = %+v, want id=%d", body.Items, id)
	}

	// ?hidden=0(または未指定)→ 可視作品のみ
	w = doGet(t, h, "/api/works?hidden=0")
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 {
		t.Errorf("?hidden=0: total = %d, want 1(可視のみ)", body.Total)
	}
}

// TestPatchWorkHidden は PATCH {hidden:true} で非表示化・{hidden:false} で解除を検証する。
func TestPatchWorkHidden(t *testing.T) {
	h, _, id := newTestServer(t)

	// --- 非表示化 ---
	w := doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"hidden":true}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("hidden=true PATCH: status = %d, body = %s", w.Code, w.Body.String())
	}

	// 一覧から消えること
	w = doGet(t, h, "/api/works")
	var list struct {
		Total int `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if list.Total != 0 {
		t.Errorf("非表示化後 total = %d, want 0", list.Total)
	}

	// 詳細の hidden が true になること
	w = doGet(t, h, urlf("/api/works/%d", id))
	if w.Code != http.StatusOK {
		t.Fatalf("GET after hidden: status = %d", w.Code)
	}
	var detail struct {
		Hidden bool `json:"hidden"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.Hidden {
		t.Error("詳細の hidden が true でない")
	}

	// --- 解除 ---
	w = doJSON(t, h, http.MethodPatch, urlf("/api/works/%d", id), `{"hidden":false}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("hidden=false PATCH: status = %d, body = %s", w.Code, w.Body.String())
	}

	// 一覧に戻ること
	w = doGet(t, h, "/api/works")
	json.Unmarshal(w.Body.Bytes(), &list)
	if list.Total != 1 {
		t.Errorf("解除後 total = %d, want 1", list.Total)
	}

	// 詳細の hidden が false になること
	w = doGet(t, h, urlf("/api/works/%d", id))
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Hidden {
		t.Error("解除後の hidden が false でない")
	}
}

// TestGetWorkHiddenField は GET /api/works/{id} のレスポンスに hidden フィールドが含まれることを検証する。
func TestGetWorkHiddenField(t *testing.T) {
	h, _, id := newTestServer(t)

	w := doGet(t, h, urlf("/api/works/%d", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	// hidden フィールドが常に存在すること(デフォルト false)
	if _, ok := raw["hidden"]; !ok {
		t.Error("hidden フィールドが存在しない")
	}
	// デフォルトは false
	if raw["hidden"] != false {
		t.Errorf("デフォルト hidden = %v, want false", raw["hidden"])
	}
}

// TestTagsHiddenWorkExcluded はタグ集計で非表示作品の分が除外されることを検証する。
// ケース1: 可視作品+非表示作品が同じタグを持つ → work_count は可視分のみ
// ケース2: 非表示作品にしか付いていないタグ → 一覧から消える
func TestTagsHiddenWorkExcluded(t *testing.T) {
	h, database, visibleID := newTestServer(t)

	// 非表示作品を追加
	res, err := database.Exec(`
		INSERT INTO works (rj_number, title, hidden) VALUES ('RJ800001', '非表示作品', 1)
	`)
	if err != nil {
		t.Fatal(err)
	}
	hiddenID, _ := res.LastInsertId()

	// タグ2件: 共有タグ(可視+非表示)、非表示専用タグ
	if _, err := database.Exec(`
		INSERT INTO tags (name, category) VALUES ('共有タグ', 'custom'), ('非表示専用タグ', 'custom')
	`); err != nil {
		t.Fatal(err)
	}
	var sharedTagID, hiddenOnlyTagID int64
	database.QueryRow("SELECT id FROM tags WHERE name='共有タグ'").Scan(&sharedTagID)
	database.QueryRow("SELECT id FROM tags WHERE name='非表示専用タグ'").Scan(&hiddenOnlyTagID)

	// 可視作品と非表示作品に共有タグを付ける
	// 非表示作品だけに非表示専用タグを付ける
	database.Exec("INSERT INTO work_tags (work_id, tag_id) VALUES (?,?),(?,?),(?,?)",
		visibleID, sharedTagID,
		hiddenID, sharedTagID,
		hiddenID, hiddenOnlyTagID)

	w := doGet(t, h, "/api/tags")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
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

	// 非表示専用タグは一覧に出ないこと
	for _, item := range body.Items {
		if item.Name == "非表示専用タグ" {
			t.Error("非表示専用タグが一覧に出た(HAVING で除外されるべき)")
		}
	}

	// 共有タグの work_count は可視分のみ(1件)
	for _, item := range body.Items {
		if item.Name == "共有タグ" {
			if item.WorkCount != 1 {
				t.Errorf("共有タグの work_count = %d, want 1(可視分のみ)", item.WorkCount)
			}
			return
		}
	}
	t.Error("共有タグが一覧に存在しない")
}

// TestCirclesHiddenWorkExcluded は非表示作品しかいないサークルが /api/circles に出ないことを検証する。
func TestCirclesHiddenWorkExcluded(t *testing.T) {
	h, database, _ := newTestServer(t)

	// 可視サークルの作品と非表示サークルの作品を追加
	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle, hidden) VALUES
		  ('RJ900001', '可視作品', '可視サークル', 0),
		  ('RJ900002', '非表示作品', '非表示専用サークル', 1)
	`); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/circles")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	// 非表示専用サークルは出ないこと
	for _, item := range body.Items {
		if item.Name == "非表示専用サークル" {
			t.Error("非表示作品しかいないサークルが /api/circles に出た")
		}
	}

	// 可視サークルは出ること
	found := false
	for _, item := range body.Items {
		if item.Name == "可視サークル" {
			found = true
			break
		}
	}
	if !found {
		t.Error("可視サークルが /api/circles に出ない")
	}
}

// TestHistoryHiddenWorkExcluded は非表示作品の履歴が /api/history に出ないことを検証する。
// play_history に行があっても、対応する作品が hidden=1 なら結果に含まれないこと。
func TestHistoryHiddenWorkExcluded(t *testing.T) {
	h, database, id := newTestServer(t)

	// テスト作品を非表示にして再生履歴を残す
	if _, err := database.Exec("UPDATE works SET hidden=1 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		"INSERT INTO play_history (work_id, file_path) VALUES (?, 'a.mp3')", id,
	); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/history")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 0 {
		t.Errorf("非表示作品の履歴が出た: items = %+v", body.Items)
	}
}

// TestCirclesHiddenParamNotAffectCircleEndpoint はサークル集計が hidden フィルタ適用後も
// クエリパラメータ q で絞り込めることを確認する(既存機能の組み合わせ確認)。
func TestCirclesHiddenWorkCountDecremented(t *testing.T) {
	h, database, _ := newTestServer(t)

	// サークルX に可視作品2件・非表示作品1件
	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle, hidden) VALUES
		  ('RJ950001', '可視1', 'サークルX', 0),
		  ('RJ950002', '可視2', 'サークルX', 0),
		  ('RJ950003', '非表示', 'サークルX', 1)
	`); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/circles?q="+url.QueryEscape("サークルX"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
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
	if len(body.Items) != 1 {
		t.Fatalf("items 数 = %d, want 1", len(body.Items))
	}
	// work_count は可視分の2件のみ
	if body.Items[0].WorkCount != 2 {
		t.Errorf("サークルX work_count = %d, want 2(可視分のみ)", body.Items[0].WorkCount)
	}
}
