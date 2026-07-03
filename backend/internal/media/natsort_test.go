package media

import (
	"sort"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"page2.jpg", "page10.jpg", true},
		{"page10.jpg", "page2.jpg", false},
		{"01.mp3", "2.mp3", true},
		{"2.mp3", "10.mp3", true},
		{"トラック1", "トラック02", true},
		{"トラック02", "トラック10", true},
		{"a", "b", true},
		{"a", "a", false},
		{"a1", "a1b", true},
		{"file", "file2", true},
		// 数値が同値の場合("1" vs "01")は後続・長さで決まる(非 less)
		{"1.mp3", "01.mp3", false},
		// 巨大な数字列でも溢れない
		{"99999999999999999998", "99999999999999999999", true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.a+"<"+c.b, func(t *testing.T) {
			if got := NaturalLess(c.a, c.b); got != c.want {
				t.Errorf("NaturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestNaturalSortOrder(t *testing.T) {
	files := []string{"page10.jpg", "page2.jpg", "10.mp3", "01.mp3", "2.mp3", "トラック10", "トラック02", "トラック1"}
	sort.SliceStable(files, func(i, j int) bool { return NaturalLess(files[i], files[j]) })
	want := []string{"01.mp3", "2.mp3", "10.mp3", "page2.jpg", "page10.jpg", "トラック1", "トラック02", "トラック10"}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("sorted = %v, want %v", files, want)
		}
	}
}

// KindByExt のテストは kind_test.go に統合(#54 で拡張子を追加した際に集約)。
