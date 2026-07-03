package api

import (
	"reflect"
	"testing"
)

// TestParseSearchTerms は parseSearchTerms の単体テスト。
// table-driven で空文字・単一ターム・複数ターム・全角スペース・
// NOT 除外(-) などのケースを網羅する。
func TestParseSearchTerms(t *testing.T) {
	cases := []struct {
		name    string
		q       string
		include []string
		exclude []string
	}{
		{
			name:    "空文字",
			q:       "",
			include: nil,
			exclude: nil,
		},
		{
			name:    "スペースのみ",
			q:       "   ",
			include: nil,
			exclude: nil,
		},
		{
			name:    "単一ターム",
			q:       "キーワード",
			include: []string{"キーワード"},
			exclude: nil,
		},
		{
			name:    "半角スペース区切り複数ターム",
			q:       "foo bar",
			include: []string{"foo", "bar"},
			exclude: nil,
		},
		{
			name:    "全角スペース区切り複数ターム",
			q:       "foo　bar",
			include: []string{"foo", "bar"},
			exclude: nil,
		},
		{
			name:    "タブ区切り複数ターム",
			q:       "foo\tbar",
			include: []string{"foo", "bar"},
			exclude: nil,
		},
		{
			name:    "半角・全角・タブ混在",
			q:       "a　b\tc",
			include: []string{"a", "b", "c"},
			exclude: nil,
		},
		{
			name:    "前後空白トリム",
			q:       "  foo  bar  ",
			include: []string{"foo", "bar"},
			exclude: nil,
		},
		{
			name:    "NOT 除外単体",
			q:       "-除外",
			include: nil,
			exclude: []string{"除外"},
		},
		{
			name:    "include + exclude 混在",
			q:       "include -exclude",
			include: []string{"include"},
			exclude: []string{"exclude"},
		},
		{
			name:    "複数 exclude",
			q:       "-a -b",
			include: nil,
			exclude: []string{"a", "b"},
		},
		{
			name:    "-単体は無視",
			q:       "-",
			include: nil,
			exclude: nil,
		},
		{
			name:    "-単体が複数混在する場合は全て無視",
			q:       "- foo -",
			include: []string{"foo"},
			exclude: nil,
		},
		{
			name:    "後方互換: 空白なし単一クエリ",
			q:       "ASMR作品",
			include: []string{"ASMR作品"},
			exclude: nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			inc, exc := parseSearchTerms(tc.q)
			if !reflect.DeepEqual(inc, tc.include) {
				t.Errorf("include: got %v, want %v", inc, tc.include)
			}
			if !reflect.DeepEqual(exc, tc.exclude) {
				t.Errorf("exclude: got %v, want %v", exc, tc.exclude)
			}
		})
	}
}

// TestEscapeLike は escapeLike が LIKE パターン用の特殊文字(\ % _)を
// 正しくエスケープすることを検証する(issue #50)。
func TestEscapeLike(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "パーセント記号",
			in:   "100%OFF",
			want: `100\%OFF`,
		},
		{
			name: "アンダースコア",
			in:   "ver_2",
			want: `ver\_2`,
		},
		{
			name: "バックスラッシュ",
			in:   `a\b`,
			want: `a\\b`,
		},
		{
			name: "混在",
			in:   `100%OFF_ver\test`,
			want: `100\%OFF\_ver\\test`,
		},
		{
			name: "空文字",
			in:   "",
			want: "",
		},
		{
			name: "日本語はそのまま",
			in:   "日本語テスト",
			want: "日本語テスト",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := escapeLike(tc.in)
			if got != tc.want {
				t.Errorf("escapeLike(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestLikeContains は likeContains が escapeLike 済みの文字列を "%...%" で
// 包んだ部分一致パターンを組み立てることを検証する。
func TestLikeContains(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "特殊文字なし", in: "foo", want: "%foo%"},
		{name: "パーセント記号", in: "100%OFF", want: `%100\%OFF%`},
		{name: "空文字", in: "", want: "%%"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := likeContains(tc.in)
			if got != tc.want {
				t.Errorf("likeContains(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
