package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// historyItem は /api/history レスポンスのデコード用。
type historyItem struct {
	Work struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"work"`
	LastPlayedAt string `json:"last_played_at"`
	LastFilePath string `json:"last_file_path"`
	PlayCount    int    `json:"play_count"`
}

// decodeHistory は履歴レスポンスを items にデコードする。
func decodeHistory(t *testing.T, body []byte) []historyItem {
	t.Helper()
	var resp struct {
		Items []historyItem `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	return resp.Items
}

// TestHistoryAggregation は作品単位の集計(最終ファイル・再生回数・最終再生日時)を検証する。
// window 関数による last_file_path 取得が相関サブクエリと同じ結果になることを担保する。
func TestHistoryAggregation(t *testing.T) {
	h, database, _ := newTestServer(t)

	if _, err := database.Exec(`
		INSERT INTO works (id, rj_number, title) VALUES
		  (101, 'RJ500001', '猫の物語'),
		  (102, 'RJ500002', '犬の日記'),
		  (103, 'RJ500003', '鳥の歌');
		INSERT INTO play_history (work_id, file_path, played_at) VALUES
		  (101, 'a/01.mp3', '2026-01-01 10:00:00'),
		  (101, 'a/02.mp3', '2026-01-02 10:00:00'),
		  (101, 'a/03.mp3', '2026-01-03 10:00:00'),
		  (102, 'b/01.mp3', '2026-01-05 10:00:00'),
		  (103, 'c/01.mp3', '2025-12-31 10:00:00'),
		  (103, 'c/02.mp3', '2026-01-01 09:00:00');
	`); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/history")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	items := decodeHistory(t, w.Body.Bytes())
	if len(items) != 3 {
		t.Fatalf("items 数 = %d, want 3; %+v", len(items), items)
	}

	// デフォルトは最終再生日時の降順: B(01-05) > A(01-03) > C(01-01)
	if items[0].Work.ID != 102 || items[1].Work.ID != 101 || items[2].Work.ID != 103 {
		t.Errorf("並び順 = [%d %d %d], want [102 101 103]",
			items[0].Work.ID, items[1].Work.ID, items[2].Work.ID)
	}

	// A の集計: 最終ファイルは最新 played_at の a/03.mp3、再生回数 3
	a := items[1]
	if a.LastFilePath != "a/03.mp3" {
		t.Errorf("A last_file_path = %q, want a/03.mp3", a.LastFilePath)
	}
	if a.PlayCount != 3 {
		t.Errorf("A play_count = %d, want 3", a.PlayCount)
	}
	// C の最終ファイルは played_at が新しい c/02.mp3 (日付の大小で判定されること)
	if items[2].LastFilePath != "c/02.mp3" {
		t.Errorf("C last_file_path = %q, want c/02.mp3", items[2].LastFilePath)
	}
}

// TestHistoryKeywordFilter は q によるタイトル絞り込みを検証する。
func TestHistoryKeywordFilter(t *testing.T) {
	h, database, _ := newTestServer(t)
	if _, err := database.Exec(`
		INSERT INTO works (id, rj_number, title) VALUES
		  (201, 'RJ500101', '猫の物語'),
		  (202, 'RJ500102', '犬の日記'),
		  (203, 'RJ500103', '猫カフェ巡り');
		INSERT INTO play_history (work_id, file_path, played_at) VALUES
		  (201, 'a.mp3', '2026-01-01 10:00:00'),
		  (202, 'b.mp3', '2026-01-02 10:00:00'),
		  (203, 'c.mp3', '2026-01-03 10:00:00');
	`); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/history?q="+url.QueryEscape("猫"))
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	items := decodeHistory(t, w.Body.Bytes())
	if len(items) != 2 {
		t.Fatalf("items 数 = %d, want 2; %+v", len(items), items)
	}
	for _, it := range items {
		if it.Work.ID != 201 && it.Work.ID != 203 {
			t.Errorf("予期しない作品 %d がヒット", it.Work.ID)
		}
	}
}

// TestHistoryKeywordLikeSpecialChars は q に含まれる % がワイルドカード展開されず
// リテラル一致のみヒットすることを検証する(issue #50)。
func TestHistoryKeywordLikeSpecialChars(t *testing.T) {
	h, database, _ := newTestServer(t)
	if _, err := database.Exec(`
		INSERT INTO works (id, rj_number, title) VALUES
		  (211, 'RJ500111', '100%OFF セール作品'),
		  (212, 'RJ500112', '100XOFF 作品');
		INSERT INTO play_history (work_id, file_path, played_at) VALUES
		  (211, 'a.mp3', '2026-01-01 10:00:00'),
		  (212, 'b.mp3', '2026-01-02 10:00:00');
	`); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/history?q="+url.QueryEscape("100%OFF"))
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	items := decodeHistory(t, w.Body.Bytes())
	if len(items) != 1 || items[0].Work.ID != 211 {
		t.Errorf("items = %+v, want 1 件(work id=211)", items)
	}
}

// TestHistorySortByPlayCount は sort=play_count / order での並び替えを検証する。
func TestHistorySortByPlayCount(t *testing.T) {
	h, database, _ := newTestServer(t)
	if _, err := database.Exec(`
		INSERT INTO works (id, rj_number, title) VALUES
		  (301, 'RJ500201', '少'),
		  (302, 'RJ500202', '多'),
		  (303, 'RJ500203', '中');
		INSERT INTO play_history (work_id, file_path, played_at) VALUES
		  (301, 'a.mp3', '2026-01-10 10:00:00'),
		  (302, 'b1.mp3', '2026-01-01 10:00:00'),
		  (302, 'b2.mp3', '2026-01-02 10:00:00'),
		  (302, 'b3.mp3', '2026-01-03 10:00:00'),
		  (303, 'c1.mp3', '2026-01-04 10:00:00'),
		  (303, 'c2.mp3', '2026-01-05 10:00:00');
	`); err != nil {
		t.Fatal(err)
	}

	// 再生回数の降順: 多(3) > 中(2) > 少(1)
	w := doGet(t, h, "/api/history?sort=play_count&order=desc")
	items := decodeHistory(t, w.Body.Bytes())
	if len(items) != 3 {
		t.Fatalf("items 数 = %d, want 3", len(items))
	}
	if items[0].Work.ID != 302 || items[1].Work.ID != 303 || items[2].Work.ID != 301 {
		t.Errorf("play_count desc 並び = [%d %d %d], want [302 303 301]",
			items[0].Work.ID, items[1].Work.ID, items[2].Work.ID)
	}

	// 昇順
	w = doGet(t, h, "/api/history?sort=play_count&order=asc")
	items = decodeHistory(t, w.Body.Bytes())
	if items[0].Work.ID != 301 || items[2].Work.ID != 302 {
		t.Errorf("play_count asc 先頭/末尾 = %d/%d, want 301/302",
			items[0].Work.ID, items[2].Work.ID)
	}
}

// TestHistoryInvalidSortFallsBack は不正な sort/order をデフォルトにフォールバックすることを確認する。
func TestHistoryInvalidSortFallsBack(t *testing.T) {
	h, database, _ := newTestServer(t)
	if _, err := database.Exec(`
		INSERT INTO works (id, rj_number, title) VALUES (401, 'RJ500301', 'X');
		INSERT INTO play_history (work_id, file_path, played_at) VALUES
		  (401, 'a.mp3', '2026-01-01 10:00:00');
	`); err != nil {
		t.Fatal(err)
	}

	// SQL インジェクションを誘う値でも 500 にならず 200 で返ること
	w := doGet(t, h, "/api/history?sort="+url.QueryEscape("id; DROP TABLE works")+"&order="+url.QueryEscape("evil"))
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	items := decodeHistory(t, w.Body.Bytes())
	if len(items) != 1 {
		t.Errorf("items 数 = %d, want 1", len(items))
	}
}

// doDelete は DELETE リクエストを発行するテスト用ヘルパー。
func doDelete(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// decodeDeleted は {"deleted": n} レスポンスをデコードする。
func decodeDeleted(t *testing.T, body []byte) int64 {
	t.Helper()
	var resp struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	return resp.Deleted
}

// TestDeleteHistoryByWorkID は work_id 指定での作品単位削除を検証する。
// 該当作品の履歴行だけが消え、他作品の履歴は残ること。
func TestDeleteHistoryByWorkID(t *testing.T) {
	h, database, _ := newTestServer(t)
	if _, err := database.Exec(`
		INSERT INTO works (id, rj_number, title) VALUES
		  (501, 'RJ500401', '猫の物語'),
		  (502, 'RJ500402', '犬の日記');
		INSERT INTO play_history (work_id, file_path, played_at) VALUES
		  (501, 'a/01.mp3', '2026-01-01 10:00:00'),
		  (501, 'a/02.mp3', '2026-01-02 10:00:00'),
		  (502, 'b/01.mp3', '2026-01-03 10:00:00');
	`); err != nil {
		t.Fatal(err)
	}

	w := doDelete(t, h, "/api/history?work_id=501")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if deleted := decodeDeleted(t, w.Body.Bytes()); deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	w = doGet(t, h, "/api/history")
	items := decodeHistory(t, w.Body.Bytes())
	if len(items) != 1 || items[0].Work.ID != 502 {
		t.Errorf("削除後の items = %+v, want 作品 502 のみ", items)
	}

	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM play_history WHERE work_id=501").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("work_id=501 の play_history 残存数 = %d, want 0", count)
	}
}

// TestDeleteHistoryAll は work_id なしでの全件クリアを検証する。
func TestDeleteHistoryAll(t *testing.T) {
	h, database, _ := newTestServer(t)
	if _, err := database.Exec(`
		INSERT INTO works (id, rj_number, title) VALUES
		  (511, 'RJ500411', '猫の物語'),
		  (512, 'RJ500412', '犬の日記');
		INSERT INTO play_history (work_id, file_path, played_at) VALUES
		  (511, 'a/01.mp3', '2026-01-01 10:00:00'),
		  (512, 'b/01.mp3', '2026-01-02 10:00:00'),
		  (512, 'b/02.mp3', '2026-01-03 10:00:00');
	`); err != nil {
		t.Fatal(err)
	}

	w := doDelete(t, h, "/api/history")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if deleted := decodeDeleted(t, w.Body.Bytes()); deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}

	w = doGet(t, h, "/api/history")
	items := decodeHistory(t, w.Body.Bytes())
	if len(items) != 0 {
		t.Errorf("全件削除後の items = %+v, want 0 件", items)
	}
}

// TestDeleteHistoryInvalidWorkID は work_id が数値でない場合に 400 を返すことを検証する。
func TestDeleteHistoryInvalidWorkID(t *testing.T) {
	h, _, _ := newTestServer(t)

	w := doDelete(t, h, "/api/history?work_id=abc")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}
