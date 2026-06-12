// 一覧ページの検索クエリを sessionStorage に保存・復元するユーティリティ。
// sessionStorage が使えない環境(プライベートモード等)では例外を握りつぶす。

const STORAGE_KEY = 'stashpad:listSearch';

/** 一覧ページの URLSearchParams 文字列を保存する */
export function saveListSearch(query: string): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, query);
  } catch {
    // 保存できない環境では何もしない
  }
}

/** 保存済みの検索クエリ文字列を取得する。未保存なら null を返す */
export function loadListSearch(): string | null {
  try {
    return sessionStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

/**
 * 詳細ページから一覧へ戻るときのパスを返す。
 * 保存済みクエリがあれば `/?{query}` を、なければ `/` を返す。
 */
export function listBackPath(): string {
  const saved = loadListSearch();
  return saved ? `/?${saved}` : '/';
}
