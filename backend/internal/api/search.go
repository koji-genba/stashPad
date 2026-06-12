package api

import (
	"strconv"
	"strings"
)

// searchTerm はキーワード検索の1つの検索語を表す。
type searchTerm struct {
	text    string // 検索語(先頭の `-` は除去済み)
	exclude bool   // true: NOT 検索
}

// parseSearchTerms はクエリ文字列を検索語に分割する。
// 半角スペース・全角スペース・タブで分割し、`-` で始まる語は除外語とする。
// `-` のみの語・空語は無視する。
func parseSearchTerms(q string) (include, exclude []string) {
	// 全角スペースを半角スペースに正規化してから分割
	q = strings.ReplaceAll(q, "　", " ")
	q = strings.ReplaceAll(q, "\t", " ")
	parts := strings.Fields(q)

	for _, p := range parts {
		if p == "-" || p == "" {
			// `-` 単体・空文字は無視
			continue
		}
		if strings.HasPrefix(p, "-") {
			term := p[1:] // 先頭の `-` を除去
			if term == "" {
				continue
			}
			exclude = append(exclude, term)
		} else {
			include = append(include, p)
		}
	}
	return include, exclude
}

// parseTagIDs はカンマ区切りのタグ ID 文字列を int64 スライスに変換する。
// 非数値・空文字は無視する(既存の tags パラメータと同じ挙動)。
func parseTagIDs(param string) []int64 {
	var ids []int64
	for _, ts := range strings.Split(param, ",") {
		ts = strings.TrimSpace(ts)
		if id, err := strconv.ParseInt(ts, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
