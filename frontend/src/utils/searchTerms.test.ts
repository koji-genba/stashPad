// searchTerms.ts のユニットテスト。
// backend/internal/api/search.go の parseSearchTerms と同じ挙動を検証する
// (空白区切り・`-` プレフィクスで除外・`-` 単体や空語は無視)。
import { describe, expect, it } from 'vitest';
import { parseSearchTerms } from './searchTerms';

describe('parseSearchTerms', () => {
  it('空文字を渡すと include/exclude ともに空配列を返す', () => {
    expect(parseSearchTerms('')).toEqual({ include: [], exclude: [] });
  });

  it('単一の語は include に入る', () => {
    expect(parseSearchTerms('foo')).toEqual({ include: ['foo'], exclude: [] });
  });

  it('半角スペース区切りで複数語を分割する', () => {
    expect(parseSearchTerms('foo bar')).toEqual({ include: ['foo', 'bar'], exclude: [] });
  });

  it('全角スペース・タブでも分割する', () => {
    expect(parseSearchTerms('foo　bar\tbaz')).toEqual({
      include: ['foo', 'bar', 'baz'],
      exclude: [],
    });
  });

  it('`-` プレフィクスの語は exclude に入り `-` は除去される', () => {
    expect(parseSearchTerms('-foo')).toEqual({ include: [], exclude: ['foo'] });
  });

  it('include と exclude が混在する場合、それぞれに振り分けられる', () => {
    expect(parseSearchTerms('foo -bar baz -qux')).toEqual({
      include: ['foo', 'baz'],
      exclude: ['bar', 'qux'],
    });
  });

  it('`-` 単体の語は無視する', () => {
    expect(parseSearchTerms('foo - bar')).toEqual({ include: ['foo', 'bar'], exclude: [] });
  });

  it('連続する空白による空語は無視する', () => {
    expect(parseSearchTerms('  foo   bar  ')).toEqual({ include: ['foo', 'bar'], exclude: [] });
  });

  it('前後の余分な空白のみの入力は空配列を返す', () => {
    expect(parseSearchTerms('   ')).toEqual({ include: [], exclude: [] });
  });
});
