package thumb

import (
	"image"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDecodeConcurrency は decodeConcurrency(numCPU) が仕様どおりの値を返すことをテスト
// (issue #82: NumCPU < 1 → 1、1 or 2 → そのまま、3 以上 → 2)。
func TestDecodeConcurrency(t *testing.T) {
	cases := []struct {
		numCPU int
		want   int
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{4, 2},
		{16, 2},
	}

	for _, tc := range cases {
		got := decodeConcurrency(tc.numCPU)
		if got != tc.want {
			t.Errorf("decodeConcurrency(%d) = %d, want %d", tc.numCPU, got, tc.want)
		}
	}
}

// TestGenerateThumbnail_LimitsConcurrentDecodes は、worker pool から並列に
// Generator.Generate を呼んでも同時デコード数が decodeSlots の容量を超えないことをテスト
// (issue #82)。decodeImageFunc を計測ラッパーに差し替え、実行中の最大同時数を記録する。
func TestGenerateThumbnail_LimitsConcurrentDecodes(t *testing.T) {
	origSlots := decodeSlots
	decodeSlots = make(chan struct{}, 2)
	t.Cleanup(func() { decodeSlots = origSlots })

	origDecodeFunc := decodeImageFunc
	t.Cleanup(func() { decodeImageFunc = origDecodeFunc })

	var current int32
	var maxSeen int32
	decodeImageFunc = func(r io.Reader) (image.Image, string, error) {
		n := atomic.AddInt32(&current, 1)
		for {
			m := atomic.LoadInt32(&maxSeen)
			if n <= m || atomic.CompareAndSwapInt32(&maxSeen, m, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		defer atomic.AddInt32(&current, -1)
		return image.Decode(r)
	}

	dir := t.TempDir()
	thumbsDir := t.TempDir()
	g := New(thumbsDir)

	const workCount = 8
	roots := make([]string, workCount)
	for i := 0; i < workCount; i++ {
		root := filepath.Join(dir, "work"+string(rune('0'+i)))
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		createTestImage(t, filepath.Join(root, "cover.png"), 50, 50)
		roots[i] = root
	}

	var wg sync.WaitGroup
	errs := make([]error, workCount)
	for i := 0; i < workCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := g.Generate(int64(1000+i), roots[i])
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("work %d: Generate 失敗: %v", i, err)
		}
	}

	if got := atomic.LoadInt32(&maxSeen); got > 2 {
		t.Errorf("同時デコード数の最大値 = %d, want <= 2", got)
	}
}
