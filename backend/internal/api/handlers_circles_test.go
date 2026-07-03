package api

import (
	"encoding/json"
	"net/url"
	"testing"
)

// ---- GET /api/circles ----------------------------------------------------------

// TestListCircles はサークル一覧の件数集計・NULL/空文字除外・q 絞り込みを検証する。
func TestListCircles(t *testing.T) {
	h, database, _ := newTestServer(t)
	// newTestServer は circle=NULL の作品を1件作成している

	// サークルを持つ作品を追加
	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle) VALUES
		  ('RJ400001', '作品A1', 'サークルA'),
		  ('RJ400002', '作品A2', 'サークルA'),
		  ('RJ400003', '作品B1', 'サークルB'),
		  ('RJ400004', 'circle NULL', NULL),
		  ('RJ400005', 'circle 空文字', '')
	`); err != nil {
		t.Fatal(err)
	}

	t.Run("件数集計", func(t *testing.T) {
		w := doGet(t, h, "/api/circles")
		if w.Code != 200 {
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
		// サークルA:2, サークルB:1 の計2件(NULL と空文字は除外)
		if len(body.Items) != 2 {
			t.Fatalf("items 数 = %d, want 2; items = %+v", len(body.Items), body.Items)
		}
		// work_count 降順 → サークルA が先頭
		if body.Items[0].Name != "サークルA" || body.Items[0].WorkCount != 2 {
			t.Errorf("先頭 = %+v, want {サークルA, 2}", body.Items[0])
		}
		if body.Items[1].Name != "サークルB" || body.Items[1].WorkCount != 1 {
			t.Errorf("2番目 = %+v, want {サークルB, 1}", body.Items[1])
		}
	})

	t.Run("NULL と空文字のサークルは除外される", func(t *testing.T) {
		w := doGet(t, h, "/api/circles")
		var body struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		for _, item := range body.Items {
			if item.Name == "" {
				t.Error("空文字サークルが含まれている")
			}
		}
	})

	t.Run("q 絞り込み", func(t *testing.T) {
		w := doGet(t, h, "/api/circles?q="+url.QueryEscape("サークルA"))
		if w.Code != 200 {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
		var body struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Items) != 1 || body.Items[0].Name != "サークルA" {
			t.Errorf("q=サークルA items = %+v, want [サークルA]", body.Items)
		}
	})

	t.Run("q 絞り込みでヒットしない場合は空配列", func(t *testing.T) {
		w := doGet(t, h, "/api/circles?q="+url.QueryEscape("存在しないサークル"))
		var body struct {
			Items []struct{} `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		if body.Items == nil {
			t.Error("items が null(空配列 [] であるべき)")
		}
		if len(body.Items) != 0 {
			t.Errorf("items 数 = %d, want 0", len(body.Items))
		}
	})
}

// TestListCirclesLikeSpecialChars は q に含まれる % がワイルドカード展開されず
// リテラル一致のみヒットすることを検証する(issue #50)。
func TestListCirclesLikeSpecialChars(t *testing.T) {
	h, database, _ := newTestServer(t)

	if _, err := database.Exec(`
		INSERT INTO works (rj_number, title, circle) VALUES
		  ('RJ410001', '作品1', '100%OFFサークル'),
		  ('RJ410002', '作品2', '100XOFFサークル')
	`); err != nil {
		t.Fatal(err)
	}

	w := doGet(t, h, "/api/circles?q="+url.QueryEscape("100%OFF"))
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Items) != 1 || body.Items[0].Name != "100%OFFサークル" {
		t.Errorf("q=100%%OFF items = %+v, want [100%%OFFサークル]", body.Items)
	}
}

// TestListCirclesEmpty は作品が全て circle=NULL の場合に items が空配列であることを検証する。
func TestListCirclesEmpty(t *testing.T) {
	h, _, _ := newTestServer(t)
	// newTestServer の作品は circle=NULL

	w := doGet(t, h, "/api/circles")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []struct{} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Items == nil {
		t.Error("0 件時に items が null(空配列 [] であるべき)")
	}
	if len(body.Items) != 0 {
		t.Errorf("items 数 = %d, want 0", len(body.Items))
	}
}

// TestListCirclesDBError は DB クローズ時に 500 を返すことを確認する。
func TestListCirclesDBError(t *testing.T) {
	h, database, _ := newTestServer(t)
	database.Close()

	w := doGet(t, h, "/api/circles")
	if w.Code != 500 {
		t.Errorf("DB エラー status = %d, want 500", w.Code)
	}
}
