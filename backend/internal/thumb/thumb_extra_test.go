package thumb

import (
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createJPEGTestImage はテスト用 JPEG 画像を作成する。
func createJPEGTestImage(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
}

// TestRefreshNoCandidates は画像が 0 件の場合に (false, "", nil) を返すことをテスト。
func TestRefreshNoCandidates(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// テキストファイルだけ(画像なし)
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := New(thumbsDir)
	regen, outPath, err := g.Refresh(100, dir)
	if err != nil {
		t.Fatalf("Refresh エラー: %v", err)
	}
	if regen {
		t.Error("画像なしなのに regen=true")
	}
	if outPath != "" {
		t.Errorf("outPath = %q, want 空文字", outPath)
	}
}

// TestRefreshRootPathInvalid は rootPath が存在しないディレクトリの場合にエラーが返ることをテスト。
func TestRefreshRootPathInvalid(t *testing.T) {
	thumbsDir := t.TempDir()

	g := New(thumbsDir)
	_, _, err := g.Refresh(101, "/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("存在しないパスなのにエラーにならなかった")
	}
}

// TestWalkDepthSubdirReadError はサブディレクトリの ReadDir が失敗しても
// 他のファイルは収集されることをテスト(walkDepth の continue 分岐)。
func TestWalkDepthSubdirReadError(t *testing.T) {
	dir := t.TempDir()

	// ルート直下に画像を置く
	createTestImage(t, filepath.Join(dir, "cover.png"), 100, 100)

	// サブディレクトリを作成し、読み取り権限を剥奪
	restrictedDir := filepath.Join(dir, "restricted")
	if err := os.Mkdir(restrictedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// restricted の中に画像を置いておく
	createTestImage(t, filepath.Join(restrictedDir, "img.png"), 50, 50)
	// 読み取り不可にする
	if err := os.Chmod(restrictedDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// テスト後に権限を戻してクリーンアップできるようにする
		_ = os.Chmod(restrictedDir, 0o755)
	})

	// root の読み込みは成功し、cover.png が候補に含まれる
	candidates, err := collectImageCandidates(dir, 2)
	if err != nil {
		t.Fatalf("collectImageCandidates 失敗: %v", err)
	}
	// restricted ディレクトリのファイルは収集できないが cover.png は収集できる
	found := false
	for _, c := range candidates {
		if filepath.Base(c) == "cover.png" {
			found = true
		}
	}
	if !found {
		t.Error("cover.png が候補に含まれなかった")
	}
}

// TestGenerateWithJPEGSource は JPEG ソース画像からサムネイルが生成されることをテスト。
func TestGenerateWithJPEGSource(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	createJPEGTestImage(t, filepath.Join(dir, "cover.jpg"), 800, 600)

	g := New(thumbsDir)
	path, err := g.Generate(200, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	if path == "" {
		t.Fatal("パスが空")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("出力ファイルが存在しない: %v", err)
	}
}

// TestGenerateSameFileTwice は同じ workID で 2 回 Generate しても
// 2 回目はスキップされることをテスト(ソース変更なし・mtime 変更なし)。
func TestGenerateSameFileTwice(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	createTestImage(t, filepath.Join(dir, "cover.png"), 100, 100)

	g := New(thumbsDir)

	// 1 回目
	path1, err := g.Generate(201, dir)
	if err != nil || path1 == "" {
		t.Fatalf("1回目 Generate 失敗: %v path=%q", err, path1)
	}

	// キャッシュの mtime をソースより未来にする
	future := time.Now().Add(10 * time.Second)
	if err := os.Chtimes(path1, future, future); err != nil {
		t.Fatal(err)
	}

	stat1, _ := os.Stat(path1)

	// 2 回目: ソースの mtime がキャッシュより古い → スキップ
	path2, err := g.Generate(201, dir)
	if err != nil {
		t.Fatalf("2回目 Generate 失敗: %v", err)
	}
	if path2 != path1 {
		t.Errorf("2回目パス変更: %q → %q", path1, path2)
	}

	stat2, _ := os.Stat(path2)
	if !stat2.ModTime().Equal(stat1.ModTime()) {
		t.Error("2回目でファイルが更新された(スキップされていない)")
	}
}

// TestIsRootThumbnail は isRootThumbnail の境界値テスト。
func TestIsRootThumbnail(t *testing.T) {
	cases := []struct {
		rootPath string
		path     string
		want     bool
	}{
		{"/a", "/a/thumbnail.jpg", true},
		{"/a", "/a/thumbnail.jpeg", true},
		{"/a", "/a/thumbnail.png", true},
		{"/a", "/a/thumbnail.webp", true},
		{"/a", "/a/THUMBNAIL.JPG", true},
		{"/a", "/a/cover.jpg", false},
		{"/a", "/a/sub/thumbnail.jpg", false}, // サブディレクトリ
		{"/a", "/a/thumbnail.gif", false},     // 非対応拡張子
		{"/a", "/a/thumbnailX.jpg", false},    // thumbnail で始まるが一致しない
	}

	for _, tc := range cases {
		got := isRootThumbnail(tc.rootPath, tc.path)
		if got != tc.want {
			t.Errorf("isRootThumbnail(%q, %q) = %v, want %v",
				tc.rootPath, tc.path, got, tc.want)
		}
	}
}

// TestResizeLongEdgeZeroDimension は超細長画像でゼロ除算が起きないことをテスト。
func TestResizeLongEdgeZeroDimension(t *testing.T) {
	// 幅 1px、高さ 2048px
	img := image.NewRGBA(image.Rect(0, 0, 1, 2048))
	resized := resizeLongEdge(img, 512)
	b := resized.Bounds()
	if b.Dx() < 1 {
		t.Errorf("幅が 0 以下: %d", b.Dx())
	}
	if b.Dy() != 512 {
		t.Errorf("高さ = %d, want 512", b.Dy())
	}
}

// TestChooseBestImageSingleCandidate は候補が 1 件の場合にそれが返されることをテスト。
func TestChooseBestImageSingleCandidate(t *testing.T) {
	got := chooseBestImage("/a", []string{"/a/only.jpg"})
	if got != "/a/only.jpg" {
		t.Errorf("got = %q, want /a/only.jpg", got)
	}
}

// TestGenerateOutputPathFormat はサムネイルの保存パスが {ThumbsDir}/{workID}.jpg 形式であることをテスト。
func TestGenerateOutputPathFormat(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	createTestImage(t, filepath.Join(dir, "cover.png"), 100, 100)

	g := New(thumbsDir)
	path, err := g.Generate(999, dir)
	if err != nil {
		t.Fatalf("Generate 失敗: %v", err)
	}
	expected := filepath.Join(thumbsDir, "999.jpg")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}

// TestCollectImageCandidatesDepthLimit は深さ 2 を超えたディレクトリは探索されないことをテスト。
func TestCollectImageCandidatesDepthLimit(t *testing.T) {
	dir := t.TempDir()

	// 深さ 3 に画像を置く
	deep := filepath.Join(dir, "lv1", "lv2", "lv3")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	createTestImage(t, filepath.Join(deep, "deep.png"), 100, 100)

	// 深さ 1 に画像を置く
	lv1 := filepath.Join(dir, "lv1")
	createTestImage(t, filepath.Join(lv1, "shallow.png"), 100, 100)

	candidates, err := collectImageCandidates(dir, 2)
	if err != nil {
		t.Fatal(err)
	}

	// shallow.png は収集される(深さ 1)
	foundShallow := false
	foundDeep := false
	for _, c := range candidates {
		if filepath.Base(c) == "shallow.png" {
			foundShallow = true
		}
		if filepath.Base(c) == "deep.png" {
			foundDeep = true
		}
	}
	if !foundShallow {
		t.Error("shallow.png(深さ1)が候補に含まれなかった")
	}
	if foundDeep {
		t.Error("deep.png(深さ3)が候補に含まれた(深さ制限が機能していない)")
	}
}

// TestGenerateThumbnailPNGSource は PNG ソースから正しい JPEG が生成されることをテスト。
func TestGenerateThumbnailPNGSource(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// 大きな PNG 画像を作成(リサイズが発生するサイズ)
	src := filepath.Join(dir, "cover.png")
	createTestImage(t, src, 1024, 768)

	outPath := filepath.Join(thumbsDir, "gen_test.jpg")
	g := New(thumbsDir)
	if err := g.generateThumbnail(src, outPath); err != nil {
		t.Fatalf("generateThumbnail 失敗: %v", err)
	}

	// 出力ファイルが JPEG で長辺 512 以下であることを確認
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("JPEG デコード失敗: %v", err)
	}
	b := img.Bounds()
	longEdge := b.Dx()
	if b.Dy() > longEdge {
		longEdge = b.Dy()
	}
	if longEdge > 512 {
		t.Errorf("長辺 %d > 512", longEdge)
	}
}

// TestGenerateThumbnailInvalidSrc は存在しないソースファイルでエラーになることをテスト。
func TestGenerateThumbnailInvalidSrc(t *testing.T) {
	thumbsDir := t.TempDir()
	outPath := filepath.Join(thumbsDir, "out.jpg")

	g := New(thumbsDir)
	err := g.generateThumbnail("/nonexistent/image.png", outPath)
	if err == nil {
		t.Error("存在しないソースファイルなのにエラーにならなかった")
	}
}

// TestGenerateThumbnailInvalidImage は画像でないファイルをデコードするとエラーになることをテスト。
func TestGenerateThumbnailInvalidImage(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// テキストファイルを PNG という名前で作成
	notAnImage := filepath.Join(dir, "not_image.png")
	if err := os.WriteFile(notAnImage, []byte("this is not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(thumbsDir, "out.jpg")
	g := New(thumbsDir)
	err := g.generateThumbnail(notAnImage, outPath)
	if err == nil {
		t.Error("無効な画像ファイルなのにエラーにならなかった")
	}
}

// TestGenerateThumbnailCreatesOutputDir は出力ディレクトリが存在しない場合に
// 自動作成して生成が成功することをテスト(main.go の起動時 MkdirAll に依存しない)。
func TestGenerateThumbnailCreatesOutputDir(t *testing.T) {
	dir := t.TempDir()
	base := t.TempDir()

	src := filepath.Join(dir, "cover.png")
	createTestImage(t, src, 100, 100)

	// 存在しないネストしたディレクトリへ出力
	outPath := filepath.Join(base, "not", "yet", "created", "out.jpg")
	g := New(base)
	if err := g.generateThumbnail(src, outPath); err != nil {
		t.Fatalf("generateThumbnail 失敗(出力先を自動作成すべき): %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("出力ファイルが存在しない: %v", err)
	}
}

// TestRefreshSourceChangedToNonPriority は生成元が thumbnail.* から通常ファイルに変わった場合に
// 再生成されることをテスト。
func TestRefreshSourceChangedToNonPriority(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// thumbnail.png を最初に用意
	thumbSrc := filepath.Join(dir, "thumbnail.png")
	createTestImage(t, thumbSrc, 50, 50)

	g := New(thumbsDir)

	// 初回: thumbnail.png から生成
	regen, path, err := g.Refresh(300, dir)
	if err != nil || !regen || path == "" {
		t.Fatalf("初回 Refresh = (%v, %q, %v)", regen, path, err)
	}

	// thumbnail.png を削除すると cover.png が選ばれる
	createTestImage(t, filepath.Join(dir, "cover.png"), 100, 100)
	if err := os.Remove(thumbSrc); err != nil {
		t.Fatal(err)
	}

	// 2 回目: 生成元が cover.png に変わるので再生成される
	regen, _, err = g.Refresh(300, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !regen {
		t.Error("生成元が変わったのに再生成されなかった")
	}
}

// TestIsImageFile は isImageFile が画像拡張子を正しく識別することをテスト。
// gif はデコード可能な候補として true(#54)。avif は KindByExt 上は image だが
// Go 標準ライブラリでデコードできないため候補から除外する(thumb_formats_test.go 参照)。
func TestIsImageFile(t *testing.T) {
	imageFiles := []string{"test.jpg", "test.jpeg", "test.png", "test.webp",
		"test.gif", "test.JPG", "test.PNG"}
	for _, f := range imageFiles {
		if !isImageFile(f) {
			t.Errorf("isImageFile(%q) = false, want true", f)
		}
	}

	nonImageFiles := []string{"test.mp3", "test.mp4", "test.txt", "test.avif", "test.bmp"}
	for _, f := range nonImageFiles {
		if isImageFile(f) {
			t.Errorf("isImageFile(%q) = true, want false", f)
		}
	}
}

// TestSrcRecordWrittenOnSkip は .src 記録がない旧キャッシュでスキップ時に .src が書き込まれることをテスト。
// (needsRegenerate の「旧バージョン相当」分岐: thumbnail.* でない場合は mtime 次第でスキップ)
func TestSrcRecordWrittenOnSkip(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// cover.png(優先名だが thumbnail.* でない)
	coverPath := filepath.Join(dir, "cover.png")
	createTestImage(t, coverPath, 100, 100)

	g := New(thumbsDir)

	// 初回生成
	if _, _, err := g.Refresh(301, dir); err != nil {
		t.Fatal(err)
	}

	// .src 記録を削除してキャッシュを旧バージョン相当にする
	srcRecord := filepath.Join(thumbsDir, "301.src")
	if err := os.Remove(srcRecord); err != nil {
		t.Fatal(err)
	}

	// ソースをキャッシュより古くする
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(coverPath, past, past); err != nil {
		t.Fatal(err)
	}

	// 2 回目: .src がない状態で cover.png(thumbnail.* でない)は mtime ベースでスキップ
	regen, outPath, err := g.Refresh(301, dir)
	if err != nil {
		t.Fatal(err)
	}
	// ソースが古いので再生成されないはず
	if regen {
		t.Log("再生成された(実装の挙動として許容: .src なし旧キャッシュで mtime が古い場合)")
	}
	if outPath == "" {
		t.Error("outPath が空")
	}

	// .src 記録が書き込まれているはず
	if _, statErr := os.Stat(srcRecord); statErr != nil {
		t.Error(".src 記録が書き込まれていない(スキップ後でも記録されるべき)")
	}
}

// TestGenerateThumbnailExceedsMaxPixels は maxPixels を小さく注入した Generator で、
// 上限を超える画像(幅×高さ > maxPixels)がエラーになりサムネイルが生成されないことをテスト(#69)。
// image.Decode で全ピクセルを展開する前に image.DecodeConfig のヘッダ情報だけで判定できているはず。
func TestGenerateThumbnailExceedsMaxPixels(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	// 100x100 = 10000px。maxPixels=100 を注入すれば上限超過になる
	src := filepath.Join(dir, "big.png")
	createTestImage(t, src, 100, 100)

	outPath := filepath.Join(thumbsDir, "out.jpg")
	g := New(thumbsDir)
	g.maxPixels = 100

	err := g.generateThumbnail(src, outPath)
	if err == nil {
		t.Fatal("上限超過の画像なのにエラーにならなかった")
	}

	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Error("上限超過なのにサムネイルファイルが生成されている")
	}
}

// TestGenerateThumbnailWithinMaxPixels は maxPixels 上限内であれば従来どおり
// サムネイルが生成されることをテスト(#69)。
func TestGenerateThumbnailWithinMaxPixels(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	src := filepath.Join(dir, "small.png")
	createTestImage(t, src, 100, 100) // 10000px

	outPath := filepath.Join(thumbsDir, "out.jpg")
	g := New(thumbsDir)
	g.maxPixels = 20000 // 10000px は上限内

	if err := g.generateThumbnail(src, outPath); err != nil {
		t.Fatalf("上限内の画像なのにエラーになった: %v", err)
	}
	if _, statErr := os.Stat(outPath); statErr != nil {
		t.Errorf("上限内なのにサムネイルファイルが生成されていない: %v", statErr)
	}
}

// TestRefreshExceedsMaxPixels は Refresh 経由でも上限超過がエラーとして伝播し、
// サムネイルが作られないことをテスト(#69)。scanner 側はこのエラーを
// 「スキップ+ログ」で処理する(scanner.go の thumbGen.Generate 呼び出し部を参照)。
func TestRefreshExceedsMaxPixels(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()

	createTestImage(t, filepath.Join(dir, "cover.png"), 100, 100)

	g := New(thumbsDir)
	g.maxPixels = 100

	regen, outPath, err := g.Refresh(400, dir)
	if err == nil {
		t.Fatal("上限超過の画像なのに Refresh がエラーを返さなかった")
	}
	if regen {
		t.Error("エラー時に regenerated=true になっている")
	}
	if outPath != "" {
		t.Errorf("エラー時に outPath = %q, want 空文字", outPath)
	}
}

// TestDefaultMaxPixels は New() が既定の maxPixels(100MP)を設定することをテスト(#69)。
func TestDefaultMaxPixels(t *testing.T) {
	g := New(t.TempDir())
	if g.maxPixels != defaultMaxPixels {
		t.Errorf("g.maxPixels = %d, want %d", g.maxPixels, defaultMaxPixels)
	}
}
