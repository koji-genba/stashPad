package media

import (
	"path/filepath"
	"strings"
)

// KindByExt は拡張子から media_kind を判定する。
// 判定は implementation-notes.md §5 の表に閉じる(MIME 推定はしない)。
func KindByExt(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".flac", ".wav", ".mp3":
		return "audio"
	case ".mp4":
		return "video"
	case ".jpg", ".jpeg", ".png", ".webp":
		return "image"
	case ".txt":
		return "text"
	default:
		return "other"
	}
}
