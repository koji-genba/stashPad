package media

import (
	"path/filepath"
	"strings"
)

// MimeByExt は拡張子から配信用 Content-Type を明示的に決定する。
//
// mime.TypeByExtension(標準ライブラリ)は OS の /etc/mime.types 等に依存し環境依存になる
// (distroless イメージには /etc/mime.types が無く、.flac が application/octet-stream に
// なるなどの問題があった)。既知の拡張子はこのテーブルで固定し、対応外は空文字列を返す
// ので、呼び出し側で mime.TypeByExtension 等へフォールバックすること。
func MimeByExt(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".ogg":
		return "audio/ogg"
	case ".opus":
		// Opus in Ogg コンテナ。audio/opus ではなく audio/ogg を使う。
		return "audio/ogg"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".avif":
		return "image/avif"
	case ".txt":
		// charset は付けない。テキストの文字コード判定(SJIS/UTF-8)はフロント側で行うため。
		return "text/plain"
	default:
		return ""
	}
}
