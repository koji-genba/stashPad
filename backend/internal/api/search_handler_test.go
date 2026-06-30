package api

import (
	"encoding/json"
	"net/url"
	"testing"
)

// ---- 機能1: キーワード検索のターム分割と NOT 検索 --------------------------------

// TestListWorksMultiTermKeyword は q を空白で分割した AND 検索を検証する。
func TestListWorksMultiTermKeyword(t *testing.T) {
	h, database, _ := newTestServer(t)

	// テスト用作品を追加
	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle) VALUES
		  ('RJ100001', 'ASMR 耳かき', 'ふわふわサークル'),
		  ('RJ100002', 'ASMR 睡眠', 'ふわふわサークル'),
		  ('RJ100003', 'BGM 作業用', 'ほのぼのサークル')
	`); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		q     string
		wantN int
	}{
		// 単一ターム(後方互換)
		{"単一ターム ASMR", "ASMR", 2},
		// 半角スペース AND: ASMR かつ 耳かき
		{"AND 検索 ASMR 耳かき", "ASMR 耳かき", 1},
		// 全角スペース AND
		{"全角スペース AND", "ASMR　睡眠", 1},
		// 存在しないタームの AND は 0 件
		{"AND 検索ヒットなし", "ASMR 存在しない", 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := doGet(t, h, "/api/works?q="+url.QueryEscape(tc.q))
			if w.Code != 200 {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			var body struct {
				Total int `json:"total"`
			}
			json.Unmarshal(w.Body.Bytes(), &body)
			if body.Total != tc.wantN {
				t.Errorf("q=%q: total = %d, want %d", tc.q, body.Total, tc.wantN)
			}
		})
	}
}

// TestListWorksNotKeyword は - で始まるタームによる除外検索を検証する。
func TestListWorksNotKeyword(t *testing.T) {
	h, database, _ := newTestServer(t)

	// テスト用作品を追加
	// newTestServer は RJ000001 を既に持つ
	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle) VALUES
		  ('RJ200001', 'ASMR 耳かき', 'サークルA'),
		  ('RJ200002', 'ASMR 睡眠', 'サークルA'),
		  ('RJ200003', 'BGM 作業用', 'サークルB')
	`); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		q     string
		wantN int
	}{
		// ASMR を除外 → RJ000001(テスト作品) と RJ200003(BGM) の2件
		{"NOT 除外", "-ASMR", 2},
		// ASMR で絞り込んで 耳かき を除外 → 睡眠の1件
		{"AND + NOT", "ASMR -耳かき", 1},
		// - 単体は無視 → 全件
		{"-単体は無視", "-", 4},
		// 複数 NOT(全角スペース区切り): ASMR と BGM を両方除外 → テスト作品のみ残る
		{"複数 NOT", "-ASMR　-BGM", 1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := doGet(t, h, "/api/works?q="+url.QueryEscape(tc.q))
			if w.Code != 200 {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			var body struct {
				Total int `json:"total"`
			}
			json.Unmarshal(w.Body.Bytes(), &body)
			if body.Total != tc.wantN {
				t.Errorf("q=%q: total = %d, want %d", tc.q, body.Total, tc.wantN)
			}
		})
	}
}

// ---- 機能2: exclude_tags パラメータ --------------------------------------------

// TestListWorksExcludeTags は exclude_tags による AND NOT EXISTS 除外を検証する。
func TestListWorksExcludeTags(t *testing.T) {
	h, database, firstID := newTestServer(t)

	// 作品2件追加
	r2, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ300001', '作品2')")
	id2, _ := r2.LastInsertId()
	r3, _ := database.Exec("INSERT INTO works (rj_number, title) VALUES ('RJ300002', '作品3')")
	id3, _ := r3.LastInsertId()

	// タグ2件
	database.Exec("INSERT INTO tags (name, category) VALUES ('除外タグ','custom'),('通常タグ','custom')")
	var excTagID, normalTagID int64
	database.QueryRow("SELECT id FROM tags WHERE name='除外タグ'").Scan(&excTagID)
	database.QueryRow("SELECT id FROM tags WHERE name='通常タグ'").Scan(&normalTagID)

	// firstID: 除外タグ + 通常タグ
	// id2: 除外タグのみ
	// id3: タグなし
	database.Exec("INSERT INTO work_tags (work_id, tag_id) VALUES (?,?),(?,?),(?,?)",
		firstID, excTagID, firstID, normalTagID, id2, excTagID)
	_ = id3

	t.Run("除外タグ単体", func(t *testing.T) {
		w := doGet(t, h, urlf("/api/works?exclude_tags=%d", excTagID))
		var body struct {
			Total int `json:"total"`
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		// firstID と id2 が除外され id3 のみ残るはず
		if body.Total != 1 {
			t.Errorf("除外後 total = %d, want 1", body.Total)
		}
		if len(body.Items) != 1 || body.Items[0].ID != id3 {
			t.Errorf("残存作品 = %+v, want id3=%d", body.Items, id3)
		}
	})

	t.Run("include タグと exclude タグの併用", func(t *testing.T) {
		// tags=normalTag かつ exclude_tags=excTag
		// → 通常タグを持ち 除外タグを持たない作品
		// firstID は両方持つのでヒットしない
		// id2 は除外タグのみ → ヒットしない
		// id3 はどちらも持たない → normalTag でフィルタされてヒットしない
		// → 結果 0 件
		w := doGet(t, h, urlf("/api/works?tags=%d&exclude_tags=%d", normalTagID, excTagID))
		var body struct {
			Total int `json:"total"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		if body.Total != 0 {
			t.Errorf("include+exclude: total = %d, want 0", body.Total)
		}
	})

	t.Run("複数 exclude_tags", func(t *testing.T) {
		// 除外タグと通常タグを両方除外 → id3 のみ
		w := doGet(t, h, urlf("/api/works?exclude_tags=%d,%d", excTagID, normalTagID))
		var body struct {
			Total int `json:"total"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		if body.Total != 1 {
			t.Errorf("複数 exclude: total = %d, want 1", body.Total)
		}
	})

	t.Run("非数値は無視", func(t *testing.T) {
		// 非数値が混ざっても無視され、有効な数値のみ適用
		w := doGet(t, h, urlf("/api/works?exclude_tags=%d,abc", excTagID))
		var body struct {
			Total int `json:"total"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		if body.Total != 1 {
			t.Errorf("非数値混在 exclude: total = %d, want 1", body.Total)
		}
	})
}
