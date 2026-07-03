package media

import "testing"

// TestMimeByExt は拡張子から Content-Type を明示的に決定する MimeByExt をテストする。
// mime.TypeByExtension は環境依存(distroless に /etc/mime.types が無い等)なので、
// 既知の拡張子はこのテーブルで固定していることを確認する。
func TestMimeByExt(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"a.flac", "audio/flac"},
		{"a.wav", "audio/wav"},
		{"a.mp3", "audio/mpeg"},
		{"a.m4a", "audio/mp4"},
		{"a.aac", "audio/aac"},
		{"a.ogg", "audio/ogg"},
		{"a.opus", "audio/ogg"}, // Opus in Ogg コンテナ
		{"a.mp4", "video/mp4"},
		{"a.m4v", "video/mp4"},
		{"a.webm", "video/webm"},
		{"a.jpg", "image/jpeg"},
		{"a.jpeg", "image/jpeg"},
		{"a.png", "image/png"},
		{"a.webp", "image/webp"},
		{"a.gif", "image/gif"},
		{"a.avif", "image/avif"},
		{"a.txt", "text/plain"}, // charset は付けない
		{"a.zip", ""},
		{"noext", ""},
		// 大文字小文字無視
		{"A.MP3", "audio/mpeg"},
		{"A.FLAC", "audio/flac"},
	}
	for _, c := range cases {
		if got := MimeByExt(c.name); got != c.want {
			t.Errorf("MimeByExt(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
