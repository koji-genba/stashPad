package thumb

import (
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// TestGenerateSkipExisting はソース画像の mtime がキャッシュより古い場合にスキップすることをテスト。
func TestGenerateSkipExisting(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	srcPath := filepath.Join(dir, "cover.png")
	createTestImage(t, srcPath, 100, 100)

	g := New(thumbsDir)

	// 初回生成
	path, err := g.Generate(4, dir)
	if err != nil || path == "" {
		t.Fatalf("初回 Generate 失敗: %v", err)
	}

	// キャッシュの mtime をソース画像より新しくする
	future := time.Now().Add(10 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
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
		rootPath   string
		candidates []string
		want       string
	}{
		{
			name:       "cover が優先",
			rootPath:   "/a",
			candidates: []string{"/a/01.jpg", "/a/cover.jpg", "/a/02.jpg"},
			want:       "/a/cover.jpg",
		},
		{
			name:       "表紙 が優先",
			rootPath:   "/a",
			candidates: []string{"/a/01.jpg", "/a/表紙.jpg"},
			want:       "/a/表紙.jpg",
		},
		{
			name:       "jacket が優先",
			rootPath:   "/a",
			candidates: []string{"/a/jacket.png", "/a/01.png"},
			want:       "/a/jacket.png",
		},
		{
			name:       "main が優先",
			rootPath:   "/a",
			candidates: []string{"/a/01.png", "/a/main.png"},
			want:       "/a/main.png",
		},
		{
			name:       "COVER 大文字も優先",
			rootPath:   "/a",
			candidates: []string{"/a/01.jpg", "/a/COVER.jpg"},
			want:       "/a/COVER.jpg",
		},
		{
			name:       "優先なしは自然順最初",
			rootPath:   "/a",
			candidates: []string{"/a/page10.jpg", "/a/page02.jpg", "/a/page01.jpg"},
			want:       "/a/page01.jpg",
		},
		{
			name:       "thumbnail.jpg が最優先(cover より上)",
			rootPath:   "/a",
			candidates: []string{"/a/cover.jpg", "/a/thumbnail.jpg", "/a/01.jpg"},
			want:       "/a/thumbnail.jpg",
		},
		{
			name:       "THUMBNAIL.PNG 大文字も最優先",
			rootPath:   "/a",
			candidates: []string{"/a/cover.jpg", "/a/THUMBNAIL.PNG"},
			want:       "/a/THUMBNAIL.PNG",
		},
		{
			name:       "サブディレクトリの thumbnail は最優先対象外",
			rootPath:   "/a",
			candidates: []string{"/a/sub/thumbnail.jpg", "/a/cover.jpg"},
			want:       "/a/cover.jpg",
		},
		{
			name:       "thumbnail.webp も最優先",
			rootPath:   "/a",
			candidates: []string{"/a/01.jpg", "/a/thumbnail.webp"},
			want:       "/a/thumbnail.webp",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseBestImage(tc.rootPath, tc.candidates)
			if got != tc.want {
				t.Errorf("chooseBestImage() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGenerateThumbnailPriority は作品ルート直下の thumbnail.* が最優先で選ばれることをテスト。
func TestGenerateThumbnailPriority(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// cover.jpg と thumbnail.png を共存させる
	createTestImage(t, filepath.Join(dir, "cover.jpg"), 100, 100)
	createTestImage(t, filepath.Join(dir, "thumbnail.png"), 200, 200)

	g := New(thumbsDir)
	path, err := g.Generate(10, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	if path == "" {
		t.Fatal("サムネイルパスが空")
	}
	// 生成されること自体を確認(内部選択の検証は chooseBestImage のテストで担保)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("サムネイルファイルが存在しない: %v", err)
	}
}

// TestGenerateMtimeRegenerate はソース画像が更新されたらキャッシュを再生成することをテスト。
func TestGenerateMtimeRegenerate(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	srcPath := filepath.Join(dir, "cover.png")
	createTestImage(t, srcPath, 100, 100)

	g := New(thumbsDir)

	// 初回生成
	path, err := g.Generate(11, dir)
	if err != nil || path == "" {
		t.Fatalf("初回 Generate 失敗: %v", err)
	}
	stat1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// キャッシュの mtime をソース画像より古く設定してからソース画像の mtime を更新
	past := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	// ソース画像の mtime をキャッシュより新しくする
	now := time.Now()
	if err := os.Chtimes(srcPath, now, now); err != nil {
		t.Fatal(err)
	}

	// 2回目: ソース画像の mtime がキャッシュより新しいので再生成される
	path2, err := g.Generate(11, dir)
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
	if stat2.ModTime().Equal(stat1.ModTime()) || stat2.ModTime().Before(stat1.ModTime()) {
		// キャッシュが古い mtime に設定されていたので、再生成後は current time になるはず
		// mtime が past より後になっていればよい
		if !stat2.ModTime().After(past) {
			t.Error("2回目でファイルが更新されなかった(再生成されていない)")
		}
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

// TestRefreshSourceChanged は mtime に関係なく、生成元が変わったら再生成されることをテスト。
// 実機フィードバック: コピー等で mtime が古いまま thumbnail.* を置いても差し替わること。
func TestRefreshSourceChanged(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	createTestImage(t, filepath.Join(dir, "cover.png"), 100, 100)

	g := New(thumbsDir)

	// 初回: cover.png から生成
	regen, path, _, err := g.Refresh(21, dir)
	if err != nil || !regen || path == "" {
		t.Fatalf("初回 Refresh = (%v, %q, %v)", regen, path, err)
	}

	// thumbnail.png を「キャッシュより古い mtime」で設置
	thumbSrc := filepath.Join(dir, "thumbnail.png")
	createTestImage(t, thumbSrc, 50, 50)
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(thumbSrc, past, past); err != nil {
		t.Fatal(err)
	}

	// 2回目: mtime が古くても生成元が thumbnail.png に変わったので再生成される
	regen, _, _, err = g.Refresh(21, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !regen {
		t.Error("生成元が thumbnail.png に変わったのに再生成されなかった")
	}

	// 3回目: 生成元・mtime とも変化なし → スキップ
	regen, _, _, err = g.Refresh(21, dir)
	if err != nil {
		t.Fatal(err)
	}
	if regen {
		t.Error("変化がないのに再生成された")
	}

	// thumbnail.png を消すと cover.png に戻る(これも生成元変更として再生成)
	if err := os.Remove(thumbSrc); err != nil {
		t.Fatal(err)
	}
	regen, _, _, err = g.Refresh(21, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !regen {
		t.Error("thumbnail.png 削除後に再生成されなかった")
	}
}

// TestRefreshLegacyCacheWithRootThumbnail は .src 記録が無い旧キャッシュでも、
// ルート直下の thumbnail.* が選ばれる場合は mtime に関係なく再生成されることをテスト。
func TestRefreshLegacyCacheWithRootThumbnail(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	thumbSrc := filepath.Join(dir, "thumbnail.png")
	createTestImage(t, thumbSrc, 50, 50)

	g := New(thumbsDir)

	// 初回生成後、旧バージョン相当にするため .src 記録を削除し、
	// ソースをキャッシュより古くする
	if _, _, _, err := g.Refresh(22, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(thumbsDir, "22.src")); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(thumbSrc, past, past); err != nil {
		t.Fatal(err)
	}

	regen, _, _, err := g.Refresh(22, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !regen {
		t.Error("記録なしキャッシュ + thumbnail.* で再生成されなかった")
	}
}
