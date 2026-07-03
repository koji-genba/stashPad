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
	// strings.Fields は unicode.IsSpace で分割するため、全角スペース・タブもそのまま扱える
	for _, p := range strings.Fields(q) {
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

// escapeLike は LIKE パターン用に \ % _ をエスケープする。
// クエリ側は必ず `LIKE ? ESCAPE '\'` の形で使うこと。
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// likeContains は s をエスケープした上で部分一致用の LIKE パターン("%s%")を組み立てる。
// クエリ側は必ず `LIKE ? ESCAPE '\'` の形で使うこと。
func likeContains(s string) string {
	return "%" + escapeLike(s) + "%"
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
