package media

import "strings"

// NaturalLess は自然順比較(page2.jpg < page10.jpg)。
// 文字列を数字列/非数字列のトークンに分け、数字列は数値として比較する。
// 非数字部分はバイト列比較(UTF-8 のコードポイント順と一致する)。
func NaturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			ai := i
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			bj := j
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			// 先頭ゼロを除いた桁数 → 値 の順で比較(任意長の数字列でも溢れない)
			an := strings.TrimLeft(a[ai:i], "0")
			bn := strings.TrimLeft(b[bj:j], "0")
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			// 数値が等しい("1" と "01")場合は続きを比較
		} else {
			if a[i] != b[j] {
				return a[i] < b[j]
			}
			i++
			j++
		}
	}
	return len(a)-i < len(b)-j
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }
