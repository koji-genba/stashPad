// Package thumb はサムネイル生成を担当する。
// 作品フォルダから深さ 2 までを探索し、優先ファイル名ルールに従って画像を選び、
// 長辺 512px / jpeg q85 で {DATA_DIR}/thumbs/{work_id}.jpg に保存する。
package thumb

import (
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // PNG デコード登録
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/koji-genba/stashpad/backend/internal/media"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // WebP デコード登録
)

// priorityPattern はサムネイル優先ファイル名のパターン(大文字小文字無視)。
var priorityPattern = regexp.MustCompile(`(?i)(表紙|cover|jacket|サムネ|main)`)

// Generator はサムネイル生成器。
type Generator struct {
	ThumbsDir string // {DATA_DIR}/thumbs
}

// New は Generator を生成する。
func New(thumbsDir string) *Generator {
	return &Generator{ThumbsDir: thumbsDir}
}

// Generate は workID の作品フォルダ rootPath からサムネイルを生成し、
// {ThumbsDir}/{workID}.jpg へ保存する。既存ならスキップ。
// 保存したパス(または既存パス)を返す。画像が見つからない場合は空文字列を返す。
func (g *Generator) Generate(workID int64, rootPath string) (string, error) {
	outPath := filepath.Join(g.ThumbsDir, fmt.Sprintf("%d.jpg", workID))

	// 生成済みならスキップ
	if _, err := os.Stat(outPath); err == nil {
		return outPath, nil
	}

	// 画像候補を収集(深さ 2 まで)
	candidates, err := collectImageCandidates(rootPath, 2)
	if err != nil {
		return "", fmt.Errorf("画像候補収集失敗: %w", err)
	}
	if len(candidates) == 0 {
		return "", nil
	}

	// 優先ファイル名ルール
	chosen := chooseBestImage(candidates)

	// 画像を読み込んでリサイズ
	if err := generateThumbnail(chosen, outPath); err != nil {
		return "", fmt.Errorf("サムネイル生成失敗 %q: %w", chosen, err)
	}
	return outPath, nil
}

// collectImageCandidates は rootPath から maxDepth の深さまで画像ファイルを収集する。
// 自然順ソートは呼び出し元(chooseBestImage)で行う。
func collectImageCandidates(root string, maxDepth int) ([]string, error) {
	var candidates []string
	err := walkDepth(root, 0, maxDepth, func(path string) {
		if isImageFile(path) {
			candidates = append(candidates, path)
		}
	})
	return candidates, err
}

// walkDepth は最大 maxDepth の深さまでディレクトリを再帰探索し、
// ファイルに callback を呼び出す。
func walkDepth(dir string, depth, maxDepth int, callback func(string)) error {
	if depth > maxDepth {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if depth < maxDepth {
				if err := walkDepth(path, depth+1, maxDepth, callback); err != nil {
					continue // サブディレクトリ読み込み失敗は無視して続行
				}
			}
		} else {
			callback(path)
		}
	}
	return nil
}

// isImageFile は拡張子が画像かどうかを判定する。
func isImageFile(name string) bool {
	return media.KindByExt(name) == "image"
}

// chooseBestImage は候補から最適な画像を選ぶ。
// 優先: 名前が priorityPattern にマッチするもの。なければ自然順で最初の画像。
func chooseBestImage(candidates []string) string {
	// 優先ファイル名チェック(ディレクトリ名は除いてファイル名部分だけを確認)
	for _, c := range candidates {
		base := filepath.Base(c)
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		if priorityPattern.MatchString(noExt) {
			return c
		}
	}

	// なければ自然順で最初の画像
	best := candidates[0]
	for _, c := range candidates[1:] {
		if media.NaturalLess(filepath.Base(c), filepath.Base(best)) {
			best = c
		}
	}
	return best
}

// generateThumbnail は src 画像を読み込み、長辺 512px・jpeg q85 で dst に保存する。
func generateThumbnail(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("ソース画像オープン失敗: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("画像デコード失敗: %w", err)
	}

	resized := resizeLongEdge(img, 512)

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("出力ファイル作成失敗: %w", err)
	}
	defer out.Close()

	if err := jpeg.Encode(out, resized, &jpeg.Options{Quality: 85}); err != nil {
		return fmt.Errorf("JPEG エンコード失敗: %w", err)
	}
	return nil
}

// resizeLongEdge は img を長辺が maxEdge になるようにアスペクト比を保って縮小する。
// 画像がすでに maxEdge 以下なら元画像をそのまま返す。
func resizeLongEdge(img image.Image, maxEdge int) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w <= maxEdge && h <= maxEdge {
		return img
	}

	var newW, newH int
	if w >= h {
		newW = maxEdge
		newH = h * maxEdge / w
		if newH == 0 {
			newH = 1
		}
	} else {
		newH = maxEdge
		newW = w * maxEdge / h
		if newW == 0 {
			newW = 1
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}
