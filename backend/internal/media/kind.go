package media

import (
	"path/filepath"
	"strings"
)

// KindByExt は拡張子から media_kind を判定する。
// 判定は implementation-notes.md §5 の表に閉じる(MIME 推定はしない)。
func KindByExt(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".flac", ".wav", ".mp3", ".m4a", ".aac", ".ogg", ".opus":
		// 注(ブラウザ互換): Safari は ogg/opus のネイティブ再生に対応していない場合がある。
		// 配信自体はこの kind に基づき行うため、非対応ブラウザでの再生失敗は別途 UI 側で考慮する。
		return "audio"
	case ".mp4", ".webm", ".m4v":
		return "video"
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif":
		// 注: avif は media_kind としては image だが、Go 標準ライブラリでデコードできない
		// ためサムネイル生成の対象外(CanDecodeThumb を参照)。配信・プレイヤー表示は
		// ブラウザのネイティブ avif 対応に委ねる。
		return "image"
	case ".txt":
		return "text"
	default:
		return "other"
	}
}

// CanDecodeThumb は name がサムネイル生成のためにデコード可能な画像形式かどうかを判定する。
// KindByExt が "image" を返す拡張子すべてがサムネイル候補になるわけではない
// (avif は Go 標準ライブラリにデコーダが無いため除外する)。
func CanDecodeThumb(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}
