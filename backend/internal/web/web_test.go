package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// PWA (issue #73) 用の manifest.webmanifest が正しい Content-Type
// (application/manifest+json) で配信されることを確認する。
// Go の mime.TypeByExtension は .webmanifest を認識せず、かつ distroless
// 環境には /etc/mime.types も無いため、明示的な補完が必要。
func TestManifestWebmanifestContentType(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: []byte("<html></html>")},
		"manifest.webmanifest": &fstest.MapFile{Data: []byte(`{"name":"stashPad"}`)},
	}
	handler := newHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	const want = "application/manifest+json"
	if got := rec.Header().Get("Content-Type"); got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

// 既存の拡張子(明示マップに無いもの)は http.FileServer のデフォルト判定に
// 委ねる従来動作が壊れていないことを確認する。
func TestOtherStaticFilesKeepDefaultContentType(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
		"app.js":     &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	handler := newHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want it to contain %q", ct, "javascript")
	}
}

// dist が未ビルド(index.html が無い)状態では 503 を返す従来動作の確認。
func TestServesServiceUnavailableWhenNotBuilt(t *testing.T) {
	fsys := fstest.MapFS{}
	handler := newHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
