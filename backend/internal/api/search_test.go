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
