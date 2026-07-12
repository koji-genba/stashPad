// 検索キーワード文字列を include/exclude 語に分割する純関数。
// backend/internal/api/search.go の parseSearchTerms と同じ挙動にすること
// (半角スペース・全角スペース・タブで分割、`-` プレフィクスは除外語、
//  `-` 単体・空語は無視)。

export interface ParsedSearchTerms {
  include: string[];
  exclude: string[];
}

/**
 * Go の unicode.IsSpace(Unicode White_Space プロパティ)と厳密一致させた空白文字クラス。
 * JS の \s とは食い違いがあるため使わない: \s は U+0085(NEL、Go にはある)を含まず、
 * 逆に U+FEFF(BOM/ZWNBSP、Go にはない)を含んでしまう。
 */
const GO_WHITE_SPACE = /[\t\n\v\f\r\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+/u;

/**
 * JS 版 strings.Fields 相当: Unicode 空白文字で分割し空要素を除く。
 * チップの ✕ クリックで特定の語だけを q から除去する処理でも同じ分割ロジックを
 * 使うため、外部から呼べるようエクスポートする。
 */
export function splitQuery(q: string): string[] {
  return q.split(GO_WHITE_SPACE).filter((s) => s !== '');
}

export function parseSearchTerms(q: string): ParsedSearchTerms {
  const include: string[] = [];
  const exclude: string[] = [];
  for (const p of splitQuery(q)) {
    if (p === '-') continue;
    if (p.startsWith('-')) {
      const term = p.slice(1);
      if (term === '') continue;
      exclude.push(term);
    } else {
      include.push(p);
    }
  }
  return { include, exclude };
}
