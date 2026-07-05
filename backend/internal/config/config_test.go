package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadLibraryRootsNotSet は STASHPAD_LIBRARY_ROOTS 未設定でエラーになることをテスト。
func TestLoadLibraryRootsNotSet(t *testing.T) {
	t.Setenv("STASHPAD_LIBRARY_ROOTS", "")
	t.Setenv("STASHPAD_DATA_DIR", "/tmp/data")

	_, err := Load()
	if err == nil {
		t.Error("STASHPAD_LIBRARY_ROOTS 未設定なのにエラーにならなかった")
	}
}

// TestLoadLibraryRootsMultiple はカンマ区切り複数パス+空白トリムが正しく処理されることをテスト。
func TestLoadLibraryRootsMultiple(t *testing.T) {
	t.Setenv("STASHPAD_LIBRARY_ROOTS", " /media/audio , /media/manga , /media/video ")
	t.Setenv("STASHPAD_DATA_DIR", "/tmp/data")
	t.Setenv("STASHPAD_ADDR", "")
	t.Setenv("STASHPAD_SCAN_ON_START", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失敗: %v", err)
	}
	want := []string{"/media/audio", "/media/manga", "/media/video"}
	if len(cfg.LibraryRoots) != len(want) {
		t.Fatalf("LibraryRoots 件数 = %d, want %d", len(cfg.LibraryRoots), len(want))
	}
	for i, w := range want {
		if cfg.LibraryRoots[i] != w {
			t.Errorf("LibraryRoots[%d] = %q, want %q", i, cfg.LibraryRoots[i], w)
		}
	}
}

// TestLoadLibraryRootsOnlyCommasAndSpaces はカンマと空白だけ(",, ")でエラーになることをテスト。
func TestLoadLibraryRootsOnlyCommasAndSpaces(t *testing.T) {
	t.Setenv("STASHPAD_LIBRARY_ROOTS", ",, ")
	t.Setenv("STASHPAD_DATA_DIR", "/tmp/data")

	_, err := Load()
	if err == nil {
		t.Error("有効なパスがないのにエラーにならなかった")
	}
}

// TestLoadDataDirNotSet は STASHPAD_DATA_DIR 未設定でエラーになることをテスト。
func TestLoadDataDirNotSet(t *testing.T) {
	t.Setenv("STASHPAD_LIBRARY_ROOTS", "/media/audio")
	t.Setenv("STASHPAD_DATA_DIR", "")

	_, err := Load()
	if err == nil {
		t.Error("STASHPAD_DATA_DIR 未設定なのにエラーにならなかった")
	}
}

// TestLoadAddrDefault は STASHPAD_ADDR 未設定のデフォルト値が ":8080" であることをテスト。
func TestLoadAddrDefault(t *testing.T) {
	t.Setenv("STASHPAD_LIBRARY_ROOTS", "/media/audio")
	t.Setenv("STASHPAD_DATA_DIR", "/tmp/data")
	t.Setenv("STASHPAD_ADDR", "")
	t.Setenv("STASHPAD_SCAN_ON_START", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失敗: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
}

// TestLoadAddrCustom は STASHPAD_ADDR が設定されている場合にその値が使われることをテスト。
func TestLoadAddrCustom(t *testing.T) {
	t.Setenv("STASHPAD_LIBRARY_ROOTS", "/media/audio")
	t.Setenv("STASHPAD_DATA_DIR", "/tmp/data")
	t.Setenv("STASHPAD_ADDR", ":9090")
	t.Setenv("STASHPAD_SCAN_ON_START", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失敗: %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want :9090", cfg.Addr)
	}
}

// TestLoadScanOnStartTrue は STASHPAD_SCAN_ON_START が true になるパターンをテスト。
func TestLoadScanOnStartTrue(t *testing.T) {
	cases := []string{"1", "true", "TRUE", "yes"}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			t.Setenv("STASHPAD_LIBRARY_ROOTS", "/media/audio")
			t.Setenv("STASHPAD_DATA_DIR", "/tmp/data")
			t.Setenv("STASHPAD_ADDR", "")
			t.Setenv("STASHPAD_SCAN_ON_START", v)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load 失敗: %v", err)
			}
			if !cfg.ScanOnStart {
				t.Errorf("STASHPAD_SCAN_ON_START=%q → ScanOnStart = false, want true", v)
			}
		})
	}
}

// TestLoadScanOnStartFalse は STASHPAD_SCAN_ON_START が false になるパターンをテスト。
func TestLoadScanOnStartFalse(t *testing.T) {
	cases := []string{"", "0", "false", "no", "other"}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			t.Setenv("STASHPAD_LIBRARY_ROOTS", "/media/audio")
			t.Setenv("STASHPAD_DATA_DIR", "/tmp/data")
			t.Setenv("STASHPAD_ADDR", "")
			t.Setenv("STASHPAD_SCAN_ON_START", v)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load 失敗: %v", err)
			}
			if cfg.ScanOnStart {
				t.Errorf("STASHPAD_SCAN_ON_START=%q → ScanOnStart = true, want false", v)
			}
		})
	}
}

// TestCheckLibraryRootsAllValid は全ルートが実在ディレクトリなら警告なしになることをテスト。
func TestCheckLibraryRootsAllValid(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	warnings := CheckLibraryRoots([]string{dir1, dir2})
	if len(warnings) != 0 {
		t.Errorf("警告 = %v, want なし", warnings)
	}
}

// TestCheckLibraryRootsNotExist は存在しないルートに警告が出ることをテスト(起動は止めない)。
func TestCheckLibraryRootsNotExist(t *testing.T) {
	valid := t.TempDir()
	missing := filepath.Join(valid, "does-not-exist")

	warnings := CheckLibraryRoots([]string{valid, missing})
	if len(warnings) != 1 {
		t.Fatalf("警告件数 = %d, want 1(%v)", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], missing) {
		t.Errorf("警告にパス %q が含まれていない: %q", missing, warnings[0])
	}
}

// TestCheckLibraryRootsNotDirectory はディレクトリでないルート(通常ファイル)に警告が出ることをテスト。
func TestCheckLibraryRootsNotDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := CheckLibraryRoots([]string{file})
	if len(warnings) != 1 {
		t.Fatalf("警告件数 = %d, want 1(%v)", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "ディレクトリではありません") {
		t.Errorf("警告メッセージが想定と異なる: %q", warnings[0])
	}
}

// TestLoadLibraryRootsCleansTrailingSlash は末尾スラッシュ・二重スラッシュ付きルートが
// filepath.Clean されることをテスト(PR #79 レビュー指摘: scanner 側の failedRoots
// 判定は DB の root_path(filepath.Join で Clean 済み)と生文字列を比較するため、
// config でも表現を統一しておく必要がある)。
func TestLoadLibraryRootsCleansTrailingSlash(t *testing.T) {
	t.Setenv("STASHPAD_LIBRARY_ROOTS", "/media/audio/, /media/manga//")
	t.Setenv("STASHPAD_DATA_DIR", "/tmp/data")
	t.Setenv("STASHPAD_ADDR", "")
	t.Setenv("STASHPAD_SCAN_ON_START", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失敗: %v", err)
	}
	want := []string{"/media/audio", "/media/manga"}
	if len(cfg.LibraryRoots) != len(want) {
		t.Fatalf("LibraryRoots 件数 = %d, want %d(%v)", len(cfg.LibraryRoots), len(want), cfg.LibraryRoots)
	}
	for i, w := range want {
		if cfg.LibraryRoots[i] != w {
			t.Errorf("LibraryRoots[%d] = %q, want %q", i, cfg.LibraryRoots[i], w)
		}
	}
}

// TestLoadSingleRoot は単一パスが正しく読み込まれることをテスト。
func TestLoadSingleRoot(t *testing.T) {
	t.Setenv("STASHPAD_LIBRARY_ROOTS", "/single/path")
	t.Setenv("STASHPAD_DATA_DIR", "/data")
	t.Setenv("STASHPAD_ADDR", "")
	t.Setenv("STASHPAD_SCAN_ON_START", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失敗: %v", err)
	}
	if len(cfg.LibraryRoots) != 1 || cfg.LibraryRoots[0] != "/single/path" {
		t.Errorf("LibraryRoots = %v, want [/single/path]", cfg.LibraryRoots)
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want /data", cfg.DataDir)
	}
}
