package media

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// テスト用の作品フォルダを組み立てる。
//
//	root/
//	├── mp3/01_オープニング.mp3
//	├── 台本.txt
//	└── link_out -> root外のファイル
func setupRoot(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "RJ404669_作品フォルダ")
	if err := os.MkdirAll(filepath.Join(root, "mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"mp3/01_オープニング.mp3", "台本.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outside = filepath.Join(base, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

func TestResolvePathOK(t *testing.T) {
	root, _ := setupRoot(t)
	for _, rel := range []string{"", "mp3", "mp3/01_オープニング.mp3", "台本.txt", "./mp3"} {
		got, err := ResolvePath(root, rel)
		if err != nil {
			t.Errorf("ResolvePath(%q) error = %v", rel, err)
			continue
		}
		rootReal, _ := filepath.EvalSymlinks(root)
		if !within(rootReal, got) {
			t.Errorf("ResolvePath(%q) = %q escapes root", rel, got)
		}
	}
}

func TestResolvePathTraversal(t *testing.T) {
	root, _ := setupRoot(t)
	cases := []string{
		"../../etc/passwd",
		"..",
		"mp3/../../secret.txt",
		"/etc/passwd",
		`..\..\etc\passwd`,
		"C:\\Windows\\system32",
		"mp3/\x00evil",
	}
	for _, rel := range cases {
		if _, err := ResolvePath(root, rel); !errors.Is(err, ErrForbidden) {
			t.Errorf("ResolvePath(%q) error = %v, want ErrForbidden", rel, err)
		}
	}
}

// URL デコード済みの "..%2f" 相当("../")が来るケース。
// ハンドラ層で r.URL.Query() がデコードするため、ここには素の "../" が届く。
func TestResolvePathEncodedTraversal(t *testing.T) {
	root, _ := setupRoot(t)
	decoded, err := url.QueryUnescape("..%2f..%2fetc%2fpasswd")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolvePath(root, decoded); !errors.Is(err, ErrForbidden) {
		t.Errorf("ResolvePath(%q) error = %v, want ErrForbidden", decoded, err)
	}
}

func TestResolvePathSymlinkEscape(t *testing.T) {
	root, outside := setupRoot(t)
	link := filepath.Join(root, "link_out")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if _, err := ResolvePath(root, "link_out"); !errors.Is(err, ErrForbidden) {
		t.Errorf("symlink escape: error = %v, want ErrForbidden", err)
	}
	// ルート内を指す symlink は許可される
	inLink := filepath.Join(root, "link_in")
	if err := os.Symlink(filepath.Join(root, "台本.txt"), inLink); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolvePath(root, "link_in"); err != nil {
		t.Errorf("symlink within root: error = %v, want nil", err)
	}
}

func TestResolvePathNotFound(t *testing.T) {
	root, _ := setupRoot(t)
	if _, err := ResolvePath(root, "存在しない.mp3"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
	// 作品ルート自体が消えている場合も 404 扱い
	if _, err := ResolvePath(filepath.Join(root, "gone"), ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing root: error = %v, want ErrNotFound", err)
	}
}
