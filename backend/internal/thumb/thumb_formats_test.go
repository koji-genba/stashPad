package thumb

import (
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// createGIFTestImage はテスト用 GIF 画像を作成する。
func createGIFTestImage(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{
		color.Black, color.White,
	})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := gif.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}

// TestIsImageFileIncludesGifExcludesAvif は isImageFile(サムネイル候補判定)が
// gif をデコード可能な候補に含め、avif を除外することをテストする。
func TestIsImageFileIncludesGifExcludesAvif(t *testing.T) {
	if !isImageFile("cover.gif") {
		t.Error(`isImageFile("cover.gif") = false, want true`)
	}
	if isImageFile("cover.avif") {
		t.Error(`isImageFile("cover.avif") = true, want false`)
	}
}

// TestCollectImageCandidatesIncludesGif は GIF ファイルがサムネイル候補として
// 収集されることをテストする。
func TestCollectImageCandidatesIncludesGif(t *testing.T) {
	dir := t.TempDir()
	createGIFTestImage(t, filepath.Join(dir, "cover.gif"), 100, 100)

	candidates, _, err := collectImageCandidates(dir, 2)
	if err != nil {
		t.Fatalf("collectImageCandidates 失敗: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %v, want 1 件(cover.gif)", candidates)
	}
	if filepath.Base(candidates[0]) != "cover.gif" {
		t.Errorf("candidates[0] = %q, want cover.gif", candidates[0])
	}
}

// TestCollectImageCandidatesExcludesAvif は avif ファイルがサムネイル候補から
// 除外されることをテストする(Go 標準ライブラリは avif をデコードできないため)。
// avif はダミーバイト列で十分(候補選定はデコードせず拡張子で判定するため)。
func TestCollectImageCandidatesExcludesAvif(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cover.avif"), []byte("dummy avif bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates, _, err := collectImageCandidates(dir, 2)
	if err != nil {
		t.Fatalf("collectImageCandidates 失敗: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("candidates = %v, want 0 件(avif は候補から除外)", candidates)
	}
}

// TestGenerateWithGIFSource は GIF ソース画像から JPEG サムネイルが生成されることをテストする。
func TestGenerateWithGIFSource(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	createGIFTestImage(t, filepath.Join(dir, "cover.gif"), 200, 150)

	g := New(thumbsDir)
	path, err := g.Generate(400, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	if path == "" {
		t.Fatal("パスが空(GIF から生成できなかった)")
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := jpeg.Decode(f); err != nil {
		t.Errorf("生成物が JPEG としてデコードできない: %v", err)
	}
}

// TestGenerateSkipsAvifOnlyFolder は avif しか無いフォルダでは
// サムネイルが生成されない(候補なし扱い)ことをテストする。
func TestGenerateSkipsAvifOnlyFolder(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cover.avif"), []byte("dummy avif bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := New(thumbsDir)
	path, err := g.Generate(401, dir)
	if err != nil {
		t.Fatalf("Generate がエラーを返した: %v", err)
	}
	if path != "" {
		t.Errorf("path = %q, want 空文字(avif のみでは生成不可)", path)
	}
}
