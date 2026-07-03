// テキストファイルのバイト列 → 文字列デコード。

/**
 * ArrayBuffer をテキストにデコードする。
 *
 * DLsite 同梱のテキスト(readme.txt・台本 txt 等)は Shift_JIS(CP932)率が高く、
 * `res.text()` (常に UTF-8 前提)でデコードすると文字化けする(issue #53)。
 * そのため以下の優先順位で判定する。
 *
 * 1. BOM (FF FE / FE FF) があれば UTF-16LE / UTF-16BE として確定的にデコードする
 *    (TextDecoder は既定で BOM を除去する)。
 * 2. BOM が無ければ UTF-8 として厳密デコード(`fatal: true`)を試みる。
 *    不正バイト列でなければ UTF-8 とみなして採用する
 *    (UTF-8 BOM 付きの場合も既定で BOM が除去されて成功する)。
 * 3. UTF-8 として不正(TypeError)であれば Shift_JIS(CP932)とみなしてデコードする。
 */
export function decodeTextBuffer(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  if (bytes[0] === 0xff && bytes[1] === 0xfe) {
    return new TextDecoder('utf-16le').decode(buf);
  }
  if (bytes[0] === 0xfe && bytes[1] === 0xff) {
    return new TextDecoder('utf-16be').decode(buf);
  }
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(buf);
  } catch (e) {
    if (e instanceof TypeError) {
      return new TextDecoder('shift_jis').decode(buf);
    }
    throw e;
  }
}
