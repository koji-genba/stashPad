package media

import "testing"

// TestKindByExt は拡張子から media_kind への判定をテストする(implementation-notes.md §5)。
func TestKindByExt(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		// audio
		{"a.flac", "audio"},
		{"a.wav", "audio"},
		{"a.mp3", "audio"},
		{"a.m4a", "audio"},
		{"a.aac", "audio"},
		{"a.ogg", "audio"},
		{"a.opus", "audio"},
		// video
		{"a.mp4", "video"},
		{"a.webm", "video"},
		{"a.m4v", "video"},
		// image
		{"a.jpg", "image"},
		{"a.jpeg", "image"},
		{"a.png", "image"},
		{"a.webp", "image"},
		{"a.gif", "image"},
		{"a.avif", "image"},
		// text
		{"a.txt", "text"},
		// other
		{"a.zip", "other"},
		{"a.pdf", "other"},
		{"noext", "other"},
		{"k.mp3.bak", "other"},
		// 大文字小文字無視
		{"A.MP3", "audio"},
		{"A.WEBM", "video"},
		{"A.GIF", "image"},
		{"A.AVIF", "image"},
	}
	for _, c := range cases {
		if got := KindByExt(c.name); got != c.want {
			t.Errorf("KindByExt(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestCanDecodeThumb は「サムネイル生成のためにデコード可能な画像形式か」を判定する
// CanDecodeThumb をテストする。KindByExt では image 扱いの avif は、
// Go 標準ライブラリでデコードできないため候補から除外する必要がある。
func TestCanDecodeThumb(t *testing.T) {
	decodable := []string{"a.jpg", "a.jpeg", "a.png", "a.webp", "a.gif", "A.GIF", "A.PNG"}
	for _, n := range decodable {
		if !CanDecodeThumb(n) {
			t.Errorf("CanDecodeThumb(%q) = false, want true", n)
		}
	}

	notDecodable := []string{"a.avif", "A.AVIF", "a.mp3", "a.txt", "a.bmp", "a.mp4"}
	for _, n := range notDecodable {
		if CanDecodeThumb(n) {
			t.Errorf("CanDecodeThumb(%q) = true, want false", n)
		}
	}
}
