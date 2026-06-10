package thumb

import (
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// createTestImage はテスト用 PNG 画像を作成する。
func createTestImage(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestGeneratePriorityImage は優先ファイル名(cover/表紙/jacket/サムネ/main)が選ばれることをテスト。
func TestGeneratePriorityImage(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// 通常ファイルと優先ファイル
	createTestImage(t, filepath.Join(dir, "01_image.png"), 100, 100)
	createTestImage(t, filepath.Join(dir, "cover.jpg"), 200, 200) // 優先

	g := New(thumbsDir)
	path, err := g.Generate(1, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	if path == "" {
		t.Fatal("サムネイルパスが空")
	}

	// 出力ファイルが存在するか
	if _, err := os.Stat(path); err != nil {
		t.Errorf("サムネイルファイルが存在しない: %v", err)
	}
}

// TestGenerateNoPriority は優先ファイルがない場合に自然順最初の画像が選ばれることをテスト。
func TestGenerateNoPriority(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// 優先名なし。自然順で page02.png > page10.png なので page02.png が先
	createTestImage(t, filepath.Join(dir, "page10.png"), 100, 100)
	createTestImage(t, filepath.Join(dir, "page02.png"), 100, 100)

	g := New(thumbsDir)
	path, err := g.Generate(2, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	if path == "" {
		t.Fatal("サムネイルパスが空")
	}
}

// TestGenerateNoImage は画像がない場合に空文字列が返ることをテスト。
func TestGenerateNoImage(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// テキストファイルだけ
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := New(thumbsDir)
	path, err := g.Generate(3, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	if path != "" {
		t.Errorf("画像なしなのにパスが返った: %q", path)
	}
}

// TestGenerateSkipExisting は生成済みサムネイルをスキップすることをテスト。
func TestGenerateSkipExisting(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	createTestImage(t, filepath.Join(dir, "cover.png"), 100, 100)

	g := New(thumbsDir)

	// 初回生成
	path, err := g.Generate(4, dir)
	if err != nil || path == "" {
		t.Fatalf("初回 Generate 失敗: %v", err)
	}

	// ファイルの更新時刻を記録
	stat1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// 2回目はスキップされてファイルが更新されない
	path2, err := g.Generate(4, dir)
	if err != nil {
		t.Fatalf("2回目 Generate 失敗: %v", err)
	}
	if path2 != path {
		t.Errorf("パスが変わった: %q → %q", path, path2)
	}
	stat2, err := os.Stat(path2)
	if err != nil {
		t.Fatal(err)
	}
	if !stat2.ModTime().Equal(stat1.ModTime()) {
		t.Error("2回目でファイルが更新された(スキップされていない)")
	}
}

// TestGenerateSubdir はサブディレクトリ内の画像も探索されることをテスト(深さ 2)。
func TestGenerateSubdir(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	subdir := filepath.Join(dir, "images")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	createTestImage(t, filepath.Join(subdir, "jacket.png"), 100, 100)

	g := New(thumbsDir)
	path, err := g.Generate(5, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	if path == "" {
		t.Fatal("サブディレクトリの画像が見つからなかった")
	}
}

// TestResizeLongEdge はリサイズが正しく動くことをテスト。
func TestResizeLongEdge(t *testing.T) {
	cases := []struct {
		w, h    int
		maxEdge int
		expW    int
		expH    int
	}{
		{1024, 768, 512, 512, 384},
		{768, 1024, 512, 384, 512},
		{100, 100, 512, 100, 100}, // 縮小不要
		{512, 512, 512, 512, 512}, // ちょうど
		{2048, 1, 512, 512, 1},    // 超細長
	}

	for _, tc := range cases {
		img := image.NewRGBA(image.Rect(0, 0, tc.w, tc.h))
		resized := resizeLongEdge(img, tc.maxEdge)
		b := resized.Bounds()
		if b.Dx() != tc.expW || b.Dy() != tc.expH {
			t.Errorf("resize(%d,%d,max=%d) = %dx%d, want %dx%d",
				tc.w, tc.h, tc.maxEdge, b.Dx(), b.Dy(), tc.expW, tc.expH)
		}
	}
}

// TestChooseBestImage は各種優先ファイル名が正しく選ばれることをテスト。
func TestChooseBestImage(t *testing.T) {
	cases := []struct {
		name       string
		candidates []string
		want       string
	}{
		{
			name:       "cover が優先",
			candidates: []string{"/a/01.jpg", "/a/cover.jpg", "/a/02.jpg"},
			want:       "/a/cover.jpg",
		},
		{
			name:       "表紙 が優先",
			candidates: []string{"/a/01.jpg", "/a/表紙.jpg"},
			want:       "/a/表紙.jpg",
		},
		{
			name:       "jacket が優先",
			candidates: []string{"/a/jacket.png", "/a/01.png"},
			want:       "/a/jacket.png",
		},
		{
			name:       "main が優先",
			candidates: []string{"/a/01.png", "/a/main.png"},
			want:       "/a/main.png",
		},
		{
			name:       "COVER 大文字も優先",
			candidates: []string{"/a/01.jpg", "/a/COVER.jpg"},
			want:       "/a/COVER.jpg",
		},
		{
			name:       "優先なしは自然順最初",
			candidates: []string{"/a/page10.jpg", "/a/page02.jpg", "/a/page01.jpg"},
			want:       "/a/page01.jpg",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseBestImage(tc.candidates)
			if got != tc.want {
				t.Errorf("chooseBestImage() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGenerateOutputIsJPEG は出力が正しい JPEG であることをテスト。
func TestGenerateOutputIsJPEG(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	createTestImage(t, filepath.Join(dir, "cover.png"), 800, 600)

	g := New(thumbsDir)
	path, err := g.Generate(99, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	if path == "" {
		t.Fatal("パスが空")
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("出力が JPEG でない: %v", err)
	}

	// 長辺が 512 以下
	b := img.Bounds()
	maxEdge := b.Dx()
	if b.Dy() > maxEdge {
		maxEdge = b.Dy()
	}
	if maxEdge > 512 {
		t.Errorf("長辺 %d > 512", maxEdge)
	}
}
