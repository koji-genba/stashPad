// format.ts の全エクスポート関数に対するユニットテスト。
// 境界値(0・負値・巨大値・null/undefined)を網羅する。
import { describe, expect, it } from 'vitest';
import {
  basename,
  formatBytes,
  formatDateTime,
  formatTime,
  joinPath,
  pathCrumbs,
} from './format';

// ---- formatBytes ----
describe('formatBytes', () => {
  it('0 を渡すと空文字を返す', () => {
    expect(formatBytes(0)).toBe('');
  });

  it('負値を渡すと空文字を返す', () => {
    expect(formatBytes(-1)).toBe('');
    expect(formatBytes(-1024)).toBe('');
  });

  it('1 バイトは "1B" を返す', () => {
    expect(formatBytes(1)).toBe('1B');
  });

  it('999 バイトは "999B" を返す', () => {
    expect(formatBytes(999)).toBe('999B');
  });

  it('1024 バイトは "1KB" を返す(割り切れる値に ".0" を付けない)', () => {
    expect(formatBytes(1024)).toBe('1KB');
  });

  it('1535 バイトは "1.5KB" を返す', () => {
    // 1535 / 1024 ≈ 1.499... → toFixed(1) = "1.5"
    expect(formatBytes(1535)).toBe('1.5KB');
  });

  it('10 * 1024 = 10240 バイトは "10KB" を返す(value=10 は 10 以上なので小数なし)', () => {
    // value=10, i=1 → 10 >= 10 → toFixed(0) → "10KB"
    expect(formatBytes(10 * 1024)).toBe('10KB');
  });

  it('1MB は "1MB" を返す', () => {
    expect(formatBytes(1024 * 1024)).toBe('1MB');
  });

  it('1GB は "1GB" を返す', () => {
    expect(formatBytes(1024 ** 3)).toBe('1GB');
  });

  it('1TB は "1TB" を返す', () => {
    expect(formatBytes(1024 ** 4)).toBe('1TB');
  });

  it('TB を超える巨大値でも TB 単位で返す(上限は TB)', () => {
    // 1024^5 = 1 PB だが units の最大は TB なので、1024 TB になる
    const pb = 1024 ** 5;
    const result = formatBytes(pb);
    expect(result).toMatch(/TB$/);
  });

  it('1.5MB を "1.5MB" で返す', () => {
    // value=1.5, i=2 → 1.5 < 10 && i != 0 → toFixed(1) → "1.5MB"
    expect(formatBytes(1.5 * 1024 * 1024)).toBe('1.5MB');
  });
});

// ---- formatTime ----
describe('formatTime', () => {
  it('0 秒は "0:00" を返す', () => {
    expect(formatTime(0)).toBe('0:00');
  });

  it('負値は "0:00" を返す', () => {
    expect(formatTime(-1)).toBe('0:00');
  });

  it('Infinity は "0:00" を返す', () => {
    expect(formatTime(Infinity)).toBe('0:00');
  });

  it('-Infinity は "0:00" を返す', () => {
    expect(formatTime(-Infinity)).toBe('0:00');
  });

  it('NaN は "0:00" を返す', () => {
    expect(formatTime(NaN)).toBe('0:00');
  });

  it('59 秒は "0:59" を返す', () => {
    expect(formatTime(59)).toBe('0:59');
  });

  it('60 秒は "1:00" を返す', () => {
    expect(formatTime(60)).toBe('1:00');
  });

  it('3599 秒は "59:59" を返す', () => {
    expect(formatTime(3599)).toBe('59:59');
  });

  it('3600 秒(1時間)は "1:00:00" を返す', () => {
    expect(formatTime(3600)).toBe('1:00:00');
  });

  it('3661 秒は "1:01:01" を返す', () => {
    expect(formatTime(3661)).toBe('1:01:01');
  });

  it('7384 秒(2時間3分4秒)は "2:03:04" を返す', () => {
    expect(formatTime(7384)).toBe('2:03:04');
  });

  it('小数を含む秒数は切り捨てる', () => {
    expect(formatTime(61.9)).toBe('1:01');
  });
});

// ---- formatDateTime ----
describe('formatDateTime', () => {
  it('null を渡すと空文字を返す', () => {
    expect(formatDateTime(null)).toBe('');
  });

  it('undefined を渡すと空文字を返す', () => {
    expect(formatDateTime(undefined)).toBe('');
  });

  it('空文字を渡すと空文字を返す', () => {
    expect(formatDateTime('')).toBe('');
  });

  it('無効な日時文字列はそのまま返す', () => {
    expect(formatDateTime('not-a-date')).toBe('not-a-date');
  });

  it('ISO 8601 文字列を "YYYY/MM/DD HH:mm" 形式に整形する', () => {
    // タイムゾーンに依存しないようローカル時刻で解析される文字列を使用
    // "2026/01/04 10:44" 形式は Date コンストラクタで解析できる
    const result = formatDateTime('2026/01/04 10:44');
    expect(result).toBe('2026/01/04 10:44');
  });

  it('ISO UTC 文字列は Date オブジェクトが解析できれば整形される', () => {
    // UTCでパースされるので、ローカルタイムに変換された結果が返る
    // ここでは変換結果の形式のみチェック
    const result = formatDateTime('2026-06-01T00:00:00.000Z');
    expect(result).toMatch(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}$/);
  });

  it('月・日・時・分が 1 桁の場合でもゼロ埋めされる', () => {
    // 2026/01/04 のような値でゼロ埋めされることを確認
    const result = formatDateTime('2026/01/04 09:05');
    expect(result).toBe('2026/01/04 09:05');
  });
});

// ---- basename ----
describe('basename', () => {
  it('パスの末尾要素を返す', () => {
    expect(basename('dir/subdir/file.txt')).toBe('file.txt');
  });

  it('末尾スラッシュを除いた末尾要素を返す', () => {
    // filter(Boolean) で末尾空文字を除外するため "dir/subdir/".split("/") = ["dir","subdir",""]
    // filter(Boolean) → ["dir","subdir"] → "subdir"
    expect(basename('dir/subdir/')).toBe('subdir');
  });

  it('ファイル名のみのパスはそのまま返す', () => {
    expect(basename('file.txt')).toBe('file.txt');
  });

  it('空文字はそのまま返す', () => {
    expect(basename('')).toBe('');
  });

  it('スラッシュのみのパスはスラッシュを返す', () => {
    // "/" を split("/") → ["", ""] → filter(Boolean) → [] → parts.length === 0 → path を返す
    expect(basename('/')).toBe('/');
  });

  it('日本語ファイル名を正しく返す', () => {
    expect(basename('作品フォルダ/サブフォルダ/ファイル.mp3')).toBe('ファイル.mp3');
  });
});

// ---- joinPath ----
describe('joinPath', () => {
  it('空の dir は name をそのまま返す', () => {
    expect(joinPath('', 'file.txt')).toBe('file.txt');
  });

  it('通常の結合', () => {
    expect(joinPath('dir', 'file.txt')).toBe('dir/file.txt');
  });

  it('dir 末尾のスラッシュを除去して結合する', () => {
    expect(joinPath('dir/', 'file.txt')).toBe('dir/file.txt');
  });

  it('複数の末尾スラッシュも除去する', () => {
    expect(joinPath('dir///', 'file.txt')).toBe('dir/file.txt');
  });

  it('ネストしたパスの結合', () => {
    expect(joinPath('parent/child', 'file.mp3')).toBe('parent/child/file.mp3');
  });
});

// ---- pathCrumbs ----
describe('pathCrumbs', () => {
  it('空文字は空配列を返す', () => {
    expect(pathCrumbs('')).toEqual([]);
  });

  it('1 階層のパス', () => {
    expect(pathCrumbs('dir')).toEqual([{ name: 'dir', path: 'dir' }]);
  });

  it('2 階層のパス', () => {
    expect(pathCrumbs('dir/subdir')).toEqual([
      { name: 'dir', path: 'dir' },
      { name: 'subdir', path: 'dir/subdir' },
    ]);
  });

  it('3 階層のパス', () => {
    expect(pathCrumbs('a/b/c')).toEqual([
      { name: 'a', path: 'a' },
      { name: 'b', path: 'a/b' },
      { name: 'c', path: 'a/b/c' },
    ]);
  });

  it('先頭スラッシュを無視する', () => {
    // filter(Boolean) で先頭空文字を除外
    expect(pathCrumbs('/dir/subdir')).toEqual([
      { name: 'dir', path: 'dir' },
      { name: 'subdir', path: 'dir/subdir' },
    ]);
  });

  it('末尾スラッシュを無視する', () => {
    expect(pathCrumbs('dir/subdir/')).toEqual([
      { name: 'dir', path: 'dir' },
      { name: 'subdir', path: 'dir/subdir' },
    ]);
  });

  it('日本語セグメントを含むパス', () => {
    expect(pathCrumbs('作品フォルダ/サブフォルダ')).toEqual([
      { name: '作品フォルダ', path: '作品フォルダ' },
      { name: 'サブフォルダ', path: '作品フォルダ/サブフォルダ' },
    ]);
  });
});
