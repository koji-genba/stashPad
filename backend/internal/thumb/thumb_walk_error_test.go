package thumb

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCollectImageCandidatesPropagatesSubdirError は、サブディレクトリの ReadDir が
// 失敗した場合に collectImageCandidates が hadError=true を返すことをテスト
// (PR #89 レビュー指摘)。
//
// walkDepth はサブディレクトリの ReadDir 失敗自体は握りつぶして探索を続行する方針だが、
// 「探索中にエラーがあった」という事実は呼び出し元に伝える必要がある。伝えないと、
// 画像が読めないサブディレクトリにしかない場合でも candidates=0 になり、
// Refresh が「画像なし」と誤判定してキャッシュを削除してしまう。
//
// chmod による権限剥奪は root 権限では効かないため、os.ReadDir を差し替え可能な
// readDirFunc 経由でエラーを注入する。
func TestCollectImageCandidatesPropagatesSubdirError(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	createTestImage(t, filepath.Join(dir, "cover.png"), 10, 10)

	origReadDir := readDirFunc
	defer func() { readDirFunc = origReadDir }()

	injectedErr := errors.New("読み込み失敗(注入)")
	readDirFunc = func(name string) ([]os.DirEntry, error) {
		if name == sub {
			return nil, injectedErr
		}
		return origReadDir(name)
	}

	candidates, hadError, err := collectImageCandidates(dir, 2)
	if err != nil {
		t.Fatalf("collectImageCandidates はルート自体の失敗ではないのでエラーを返すべきではない: %v", err)
	}
	if !hadError {
		t.Error("hadError = false, want true(サブディレクトリの探索エラーを伝播すべき)")
	}
	// cover.png はルート直下にあるので収集されているはず
	found := false
	for _, c := range candidates {
		if filepath.Base(c) == "cover.png" {
			found = true
		}
	}
	if !found {
		t.Error("cover.png が候補に含まれなかった")
	}
}

// TestCollectImageCandidatesRootErrorPropagatesAsErr はルート自体の ReadDir が
// 失敗した場合、hadError と err の両方が伝わることをテスト(既存の Refresh 経路の維持)。
func TestCollectImageCandidatesRootErrorPropagatesAsErr(t *testing.T) {
	_, hadError, err := collectImageCandidates("/nonexistent/path/xyz", 2)
	if err == nil {
		t.Fatal("存在しないルートなのにエラーにならなかった")
	}
	if !hadError {
		t.Error("hadError = false, want true")
	}
}

// TestRefreshSubdirErrorWithoutOtherCandidatesDoesNotDeleteCache は、探索エラーが
// あり候補が 0 件の場合、Refresh がキャッシュを削除せずエラーを返すことをテスト
// (PR #89 レビュー指摘: 画像が読めないサブディレクトリにだけあるケースの誤削除防止)。
func TestRefreshSubdirErrorWithoutOtherCandidatesDoesNotDeleteCache(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	g := New(thumbsDir)

	// 事前に既存のキャッシュファイルを置いておく(誤って消えないかを確認するため)
	outPath := filepath.Join(thumbsDir, "500.jpg")
	srcRecord := filepath.Join(thumbsDir, "500.src")
	if err := os.WriteFile(outPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcRecord, []byte("old-src"), 0o644); err != nil {
		t.Fatal(err)
	}

	origReadDir := readDirFunc
	defer func() { readDirFunc = origReadDir }()
	injectedErr := errors.New("読み込み失敗(注入)")
	readDirFunc = func(name string) ([]os.DirEntry, error) {
		if name == sub {
			return nil, injectedErr
		}
		return origReadDir(name)
	}

	_, outPathGot, candidateFound, err := g.Refresh(500, dir)
	if err == nil {
		t.Fatal("探索エラーがあり候補 0 件のとき Refresh はエラーを返すべき")
	}
	if candidateFound {
		t.Error("candidateFound = true, want false")
	}
	if outPathGot != "" {
		t.Errorf("outPath = %q, want 空文字", outPathGot)
	}

	// 既存キャッシュは削除されていないはず
	if _, statErr := os.Stat(outPath); statErr != nil {
		t.Errorf("探索エラー時に既存キャッシュが削除されてしまった: %v", statErr)
	}
	if _, statErr := os.Stat(srcRecord); statErr != nil {
		t.Errorf("探索エラー時に既存 .src が削除されてしまった: %v", statErr)
	}
}
