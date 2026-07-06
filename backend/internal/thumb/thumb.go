// Package thumb はサムネイル生成を担当する。
// 作品フォルダから深さ 2 までを探索し、優先ファイル名ルールに従って画像を選び、
// 長辺 512px / jpeg q85 で {DATA_DIR}/thumbs/{work_id}.jpg に保存する。
package thumb

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif" // GIF デコード登録
	"image/jpeg"
	_ "image/png" // PNG デコード登録
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/koji-genba/stashpad/backend/internal/media"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // WebP デコード登録
)

// defaultMaxPixels はサムネイル生成時にデコードを許可する画像の最大ピクセル数(幅×高さ)。
// 20000x20000 の PNG のような decompression bomb が worker pool で同時に複数枚展開されると
// OOM でコンテナが落ちるリスクがあるため、デコード前にヘッダ情報だけで足切りする(#69)。
const defaultMaxPixels = 100_000_000 // 100MP(例: 10000x10000)

// priorityPattern はサムネイル優先ファイル名のパターン(大文字小文字無視)。
// thumbnail.* の特別ルールより低い優先度として使う。
var priorityPattern = regexp.MustCompile(`(?i)(表紙|cover|jacket|サムネ|main)`)

// thumbnailPattern はルート直下の thumbnail.(jpg|jpeg|png|webp) にマッチする(大文字小文字無視)。
var thumbnailPattern = regexp.MustCompile(`(?i)^thumbnail\.(jpg|jpeg|png|webp)$`)

// Generator はサムネイル生成器。
type Generator struct {
	ThumbsDir string // {DATA_DIR}/thumbs

	// maxPixels はデコードを許可する画像の最大ピクセル数(幅×高さ)。
	// New() で defaultMaxPixels が設定される。テストでは小さい値に差し替えて
	// decompression bomb ガードの挙動を検証できる。
	maxPixels int64
}

// New は Generator を生成する。
func New(thumbsDir string) *Generator {
	return &Generator{ThumbsDir: thumbsDir, maxPixels: defaultMaxPixels}
}

// Generate は workID の作品フォルダ rootPath からサムネイルを生成し、
// {ThumbsDir}/{workID}.jpg へ保存する。
// ソース画像の mtime がキャッシュより新しい(またはキャッシュが無い)場合のみ(再)生成する。
// 保存したパス(または既存パス)を返す。画像が見つからない場合は空文字列を返す。
func (g *Generator) Generate(workID int64, rootPath string) (string, error) {
	_, outPath, _, err := g.Refresh(workID, rootPath)
	return outPath, err
}

// Refresh は必要な場合のみサムネイルを(再)生成する。再生成した場合は true を返す。
// 画像が見つからない場合は古いキャッシュを削除し、candidateFound=false を返す。
//
// 再生成の判定: キャッシュが無い / 選ばれたソース画像が前回生成時(サイドカー
// {workID}.src に記録)と異なる / ソース画像の mtime がキャッシュより新しい。
// ソース記録のおかげで、ユーザーが mtime の古い thumbnail.* をコピーで置いた
// 場合でも確実に差し替わる。
func (g *Generator) Refresh(workID int64, rootPath string) (regenerated bool, outPath string, candidateFound bool, err error) {
	outPath = filepath.Join(g.ThumbsDir, fmt.Sprintf("%d.jpg", workID))
	srcRecord := filepath.Join(g.ThumbsDir, fmt.Sprintf("%d.src", workID))

	// 候補収集
	candidates, hadWalkError, err := collectImageCandidates(rootPath, 2)
	if err != nil {
		return false, "", false, fmt.Errorf("画像候補収集失敗: %w", err)
	}
	if len(candidates) == 0 {
		if hadWalkError {
			// サブディレクトリの探索中にエラーがあった状態で候補ゼロは「画像が無い」とは
			// 判定できない(一時的に読めないサブディレクトリにだけ画像がある可能性がある)。
			// 誤ってキャッシュを削除しないよう、削除せずエラーを返す
			// (呼び出し元はログして継続する既存経路に乗る。PR #89 レビュー指摘)。
			return false, "", false, fmt.Errorf(
				"画像候補の探索中にエラーが発生したため判定を保留します(rootPath=%q)", rootPath,
			)
		}
		if err := removeThumbnailCache(outPath, srcRecord); err != nil {
			return false, "", false, err
		}
		return false, "", false, nil
	}

	var candidateErrs []error
	for _, chosen := range orderedImageCandidates(rootPath, candidates) {
		srcStat, err := os.Stat(chosen)
		if err != nil {
			candidateErrs = append(candidateErrs, fmt.Errorf("%q: ソース画像 Stat 失敗: %w", chosen, err))
			continue
		}

		if !needsRegenerate(rootPath, chosen, outPath, srcRecord, srcStat) {
			// 旧バージョンで生成されたキャッシュには記録が無いので、次回の差し替え検出用に残す
			if _, recErr := os.Stat(srcRecord); recErr != nil {
				_ = os.WriteFile(srcRecord, []byte(chosen), 0o644)
			}
			return false, outPath, true, nil
		}

		if err := g.generateThumbnail(chosen, outPath); err != nil {
			candidateErrs = append(candidateErrs, fmt.Errorf("%q: %w", chosen, err))
			continue
		}
		_ = os.WriteFile(srcRecord, []byte(chosen), 0o644)
		return true, outPath, true, nil
	}

	return false, "", true, fmt.Errorf("サムネイル生成失敗: %w", errors.Join(candidateErrs...))
}

// RemoveCache は workID のサムネイルキャッシュファイル({workID}.jpg / {workID}.src)
// を削除する。scanner.ThumbnailGenerator インターフェースが要求する公開版で、
// フォルダ消失により work の root_path が NULL 化された際に呼ばれる
// (removeThumbnailCache の公開版。fix/80 で導入した内部関数を再利用)。
// ファイルが元から存在しない場合はエラーにしない。
func (g *Generator) RemoveCache(workID int64) error {
	outPath := filepath.Join(g.ThumbsDir, fmt.Sprintf("%d.jpg", workID))
	srcRecord := filepath.Join(g.ThumbsDir, fmt.Sprintf("%d.src", workID))
	return removeThumbnailCache(outPath, srcRecord)
}

func removeThumbnailCache(outPath, srcRecord string) error {
	for _, path := range []string{outPath, srcRecord} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("古いサムネイル削除失敗 %q: %w", path, err)
		}
	}
	return nil
}

// needsRegenerate はサムネイルを作り直すべきかを判定する。
func needsRegenerate(rootPath, chosen, outPath, srcRecord string, srcStat os.FileInfo) bool {
	cacheStat, err := os.Stat(outPath)
	if err != nil {
		return true // キャッシュなし
	}
	rec, err := os.ReadFile(srcRecord)
	if err != nil {
		// 生成元の記録が無い旧キャッシュ。ユーザー指定の thumbnail.* が選ばれている
		// 場合は差し替え検出ができないため mtime に関係なく作り直す
		return isRootThumbnail(rootPath, chosen)
	}
	if strings.TrimSpace(string(rec)) != chosen {
		return true // 生成元が変わった(thumbnail.* の設置・削除など)
	}
	return srcStat.ModTime().After(cacheStat.ModTime()) // 同一ソースの更新
}

// isRootThumbnail は path が作品ルート直下の thumbnail.(jpg|jpeg|png|webp) かを判定する。
func isRootThumbnail(rootPath, path string) bool {
	return filepath.Dir(path) == rootPath && thumbnailPattern.MatchString(filepath.Base(path))
}

// readDirFunc は os.ReadDir の差し替え可能な参照。テストからサブディレクトリの
// ReadDir 失敗を注入するために使う(chmod は root 権限下では効かないため)。
var readDirFunc = os.ReadDir

// collectImageCandidates は rootPath から maxDepth の深さまで画像ファイルを収集する。
// 自然順ソートは呼び出し元(chooseBestImage)で行う。
// hadError は探索中に(ルート自身を含め)ReadDir 失敗が 1 件以上あったかを示す。
// err はルート自身の ReadDir 失敗のみを伝える(従来どおり Refresh 側で
// 「画像候補収集失敗」として扱う)。
func collectImageCandidates(root string, maxDepth int) (candidates []string, hadError bool, err error) {
	hadError, err = walkDepth(root, 0, maxDepth, func(path string) {
		if isImageFile(path) {
			candidates = append(candidates, path)
		}
	})
	return candidates, hadError, err
}

// walkDepth は最大 maxDepth の深さまでディレクトリを再帰探索し、
// ファイルに callback を呼び出す。
// サブディレクトリの ReadDir 失敗(権限なし・壊れた symlink・NAS の一時的な読み取り
// 不能等)は意図的に握りつぶして残りのエントリの探索を続行するが、hadError=true として
// 呼び出し元に伝える。候補ゼロと組み合わさったときに「画像が本当に無い」のか
// 「探索できなかっただけ」なのかを呼び出し元(Refresh)が区別できるようにするため
// (PR #89 レビュー指摘)。
// err は dir 自身の ReadDir 失敗のみを返す(再帰呼び出しのエラーは hadError に畳み込む)。
func walkDepth(dir string, depth, maxDepth int, callback func(string)) (hadError bool, err error) {
	if depth > maxDepth {
		return false, nil
	}
	entries, err := readDirFunc(dir)
	if err != nil {
		return true, err
	}
	for _, e := range entries {
		if media.IsHiddenName(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if depth < maxDepth {
				subHadError, _ := walkDepth(path, depth+1, maxDepth, callback)
				if subHadError {
					hadError = true
				}
			}
		} else {
			callback(path)
		}
	}
	return hadError, nil
}

// isImageFile はサムネイル生成のデコード候補になる画像拡張子かどうかを判定する。
// media_kind としては image でも(例: avif)Go 標準ライブラリでデコードできない
// 形式はサムネイル候補から除外する(media.CanDecodeThumb を参照)。
func isImageFile(name string) bool {
	return media.CanDecodeThumb(name)
}

// chooseBestImage は候補から最適な画像を選ぶ。
// 優先順位:
//  1. 作品ルート直下の thumbnail.(jpg|jpeg|png|webp)(大文字小文字無視)
//  2. 名前が priorityPattern(表紙|cover|jacket|サムネ|main)にマッチするもの
//  3. 自然順で最初の画像
func chooseBestImage(rootPath string, candidates []string) string {
	ordered := orderedImageCandidates(rootPath, candidates)
	if len(ordered) == 0 {
		return ""
	}
	return ordered[0]
}

func orderedImageCandidates(rootPath string, candidates []string) []string {
	ordered := make([]string, 0, len(candidates))
	used := make([]bool, len(candidates))

	// 最優先: ルート直下の thumbnail.*
	for i, c := range candidates {
		if isRootThumbnail(rootPath, c) {
			ordered = append(ordered, c)
			used[i] = true
		}
	}

	// 次優先: 名前が priorityPattern にマッチするもの(ディレクトリ名は除いてファイル名部分だけを確認)
	for i, c := range candidates {
		if used[i] {
			continue
		}
		base := filepath.Base(c)
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		if priorityPattern.MatchString(noExt) {
			ordered = append(ordered, c)
			used[i] = true
		}
	}

	// なければ自然順で最初の画像
	rest := make([]string, 0, len(candidates)-len(ordered))
	for i, c := range candidates {
		if !used[i] {
			rest = append(rest, c)
		}
	}
	sortImageCandidates(rest)
	ordered = append(ordered, rest...)
	return ordered
}

func sortImageCandidates(candidates []string) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := filepath.Base(candidates[i])
		right := filepath.Base(candidates[j])
		if left == right {
			return candidates[i] < candidates[j]
		}
		return media.NaturalLess(left, right)
	})
}

// generateThumbnail は src 画像を読み込み、長辺 512px・jpeg q85 で dst に保存する。
// decompression bomb 対策として、全ピクセルを展開する image.Decode の前に
// image.DecodeConfig でヘッダだけ読み、寸法が g.maxPixels を超える場合はデコードせずエラーを返す。
func (g *Generator) generateThumbnail(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("ソース画像オープン失敗: %w", err)
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		// DecodeConfig が失敗するファイルは通常 Decode も失敗するため、
		// ここで即エラーを返し全ピクセル展開は行わない。
		return fmt.Errorf("画像ヘッダデコード失敗: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("画像サイズが不正です (%dx%d)", cfg.Width, cfg.Height)
	}
	// int64 での乗算によりオーバーフローを避ける。
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if pixels > g.maxPixels {
		return fmt.Errorf("画像が大きすぎます (%dx%d)", cfg.Width, cfg.Height)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("ソース画像シーク失敗: %w", err)
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("画像デコード失敗: %w", err)
	}

	resized := resizeLongEdge(img, 512)

	// 出力先ディレクトリ({DataDir}/thumbs)が無ければ作成する。
	// main.go の起動時 MkdirAll に依存せず、生成経路自身で保証する
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("出力ディレクトリ作成失敗: %w", err)
	}

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
