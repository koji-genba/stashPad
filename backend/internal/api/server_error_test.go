package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRespondInternalErrorHidesDetails は respondInternalError がクライアントには
// 固定メッセージのみを返し、err.Error() の内部詳細を漏らさないことをテスト(issue #70)。
func TestRespondInternalErrorHidesDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	respondInternalError(rec, "一覧取得失敗", errors.New("secret internal detail: SELECT * FROM works"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "一覧取得失敗") {
		t.Errorf("body = %q, want 固定メッセージ「一覧取得失敗」を含む", body)
	}
	if strings.Contains(body, "secret internal detail") {
		t.Errorf("body = %q に err.Error() の内部詳細が漏れている", body)
	}
}

// TestClassifyMultipartParseError は ParseMultipartForm のエラー分類をテスト。
// http.MaxBytesReader 由来のサイズ超過は 413、それ以外は 400 に分類される(issue #70)。
func TestClassifyMultipartParseError(t *testing.T) {
	t.Run("MaxBytesErrorは413", func(t *testing.T) {
		err := &http.MaxBytesError{Limit: maxCSVUploadBytes}
		status, msg := classifyMultipartParseError(err)
		if status != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want %d", status, http.StatusRequestEntityTooLarge)
		}
		if !strings.Contains(msg, "大きすぎます") {
			t.Errorf("msg = %q, want サイズ超過メッセージ", msg)
		}
	})

	t.Run("ラップされたMaxBytesErrorも413", func(t *testing.T) {
		err := fmtWrap(&http.MaxBytesError{Limit: maxCSVUploadBytes})
		status, _ := classifyMultipartParseError(err)
		if status != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want %d", status, http.StatusRequestEntityTooLarge)
		}
	})

	t.Run("その他のパースエラーは400", func(t *testing.T) {
		status, msg := classifyMultipartParseError(errors.New("no multipart boundary param in Content-Type"))
		if status != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
		}
		if !strings.Contains(msg, "multipart パース失敗") {
			t.Errorf("msg = %q, want パース失敗メッセージ", msg)
		}
	})
}

// fmtWrap はエラーを 1 段ラップする(errors.As の分類が unwrap 越しにも効くことの確認用)。
func fmtWrap(err error) error {
	return &wrappedErr{inner: err}
}

type wrappedErr struct{ inner error }

func (w *wrappedErr) Error() string { return "wrapped: " + w.inner.Error() }
func (w *wrappedErr) Unwrap() error { return w.inner }
