package media

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// パス検証の結果。HTTP ハンドラはこれをステータスコードに対応付ける。
var (
	// ErrForbidden は作品ルート外へのアクセス試行(→ 403)。
	ErrForbidden = errors.New("path escapes work root")
	// ErrNotFound は対象が存在しない(→ 404)。
	ErrNotFound = errors.New("path not found")
)

// ResolvePath は作品ルート root と相対パス rel を結合し、symlink を解決した上で
// 結果が root 配下に留まることを検証する(implementation-notes.md §6)。
// 検証を通った実パス(symlink 解決済み)を返す。
func ResolvePath(root, rel string) (string, error) {
	// 絶対パス・Windows ドライブ・バックスラッシュ・NUL は即拒否
	if strings.ContainsRune(rel, 0) || strings.Contains(rel, `\`) {
		return "", ErrForbidden
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", ErrForbidden
	}

	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		// 作品ルート自体が消えている(マウント外れ等)
		return "", ErrNotFound
	}

	// Join は ".." を畳み込むので、この時点でルート外に出ていれば traversal
	joined := filepath.Join(rootReal, rel)
	if !within(rootReal, joined) {
		return "", ErrForbidden
	}

	real, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	// symlink がルート外を指していた場合を拒否
	if !within(rootReal, real) {
		return "", ErrForbidden
	}
	return real, nil
}

// within は p が root と同一、または root 配下にあるかを判定する。
// 単純な prefix 比較ではなくパス区切り境界で判定する(/media/a と /media/ab を区別)。
func within(root, p string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(filepath.Separator))
}

// IsHiddenName は macOS の AppleDouble(._*)や .DS_Store を含む dotfile / dotdir
// として扱う名前かどうかを返す。os.ReadDir のエントリ名に対して使う想定。
func IsHiddenName(name string) bool {
	return strings.HasPrefix(name, ".")
}
