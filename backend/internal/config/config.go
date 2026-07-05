// Package config は環境変数から設定を読み込む。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config はアプリケーション全体の設定を保持する。
type Config struct {
	// LibraryRoots はライブラリルートの絶対パスのスライス(STASHPAD_LIBRARY_ROOTS)。
	LibraryRoots []string
	// DataDir は SQLite DB とサムネイルキャッシュの置き場所(STASHPAD_DATA_DIR)。
	DataDir string
	// Addr はリッスンアドレス(STASHPAD_ADDR、デフォルト :8080)。
	Addr string
	// ScanOnStart が true なら起動時にライブラリスキャンを自動実行する
	// (STASHPAD_SCAN_ON_START=true/1)。
	ScanOnStart bool
}

// Load は環境変数から設定を読み込む。
// STASHPAD_LIBRARY_ROOTS と STASHPAD_DATA_DIR は必須。
func Load() (*Config, error) {
	roots := os.Getenv("STASHPAD_LIBRARY_ROOTS")
	if roots == "" {
		return nil, fmt.Errorf("STASHPAD_LIBRARY_ROOTS が設定されていません")
	}
	parts := strings.Split(roots, ",")
	var libraryRoots []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			// 末尾スラッシュ・二重スラッシュ等を正規化する。scanner 側は DB の
			// root_path(filepath.Join で Clean 済み)と生文字列の failedRoots を
			// 比較するため、config 側でも表現を統一しておく(PR #79 レビュー指摘)。
			libraryRoots = append(libraryRoots, filepath.Clean(p))
		}
	}
	if len(libraryRoots) == 0 {
		return nil, fmt.Errorf("STASHPAD_LIBRARY_ROOTS に有効なパスがありません")
	}

	dataDir := os.Getenv("STASHPAD_DATA_DIR")
	if dataDir == "" {
		return nil, fmt.Errorf("STASHPAD_DATA_DIR が設定されていません")
	}

	addr := os.Getenv("STASHPAD_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	scanOnStart := false
	switch strings.ToLower(os.Getenv("STASHPAD_SCAN_ON_START")) {
	case "1", "true", "yes":
		scanOnStart = true
	}

	return &Config{
		LibraryRoots: libraryRoots,
		DataDir:      dataDir,
		Addr:         addr,
		ScanOnStart:  scanOnStart,
	}, nil
}

// CheckLibraryRoots は各ライブラリルートを os.Stat で確認し、
// 存在しない・ディレクトリでないルートについて警告メッセージを返す(issue #70)。
// 設定ミスや NAS 未マウントに起動時点で気付けるようにするのが目的。
// 起動は止めない: 全ルート不存在でもスキャナ側の全滅ガード(issue #48)が
// DB の全件 NULL 化を防ぐため、警告ログのみで十分。
func CheckLibraryRoots(roots []string) []string {
	var warnings []string
	for _, root := range roots {
		st, err := os.Stat(root)
		switch {
		case err != nil:
			warnings = append(warnings, fmt.Sprintf(
				"ライブラリルート %q にアクセスできません(未マウント・パス誤りの可能性): %v", root, err))
		case !st.IsDir():
			warnings = append(warnings, fmt.Sprintf(
				"ライブラリルート %q はディレクトリではありません", root))
		}
	}
	return warnings
}
