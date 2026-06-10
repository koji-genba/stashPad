// Package config は環境変数から設定を読み込む。
package config

import (
	"fmt"
	"os"
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
			libraryRoots = append(libraryRoots, p)
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

	return &Config{
		LibraryRoots: libraryRoots,
		DataDir:      dataDir,
		Addr:         addr,
	}, nil
}
