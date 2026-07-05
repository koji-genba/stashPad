// textDecode.ts のユニットテスト。
// DLsite 同梱テキスト(readme.txt・台本 txt 等)は Shift_JIS(CP932)率が高いため、
// UTF-8 として不正なバイト列は Shift_JIS にフォールバックして復元できることを検証する(issue #53)。
import { describe, expect, it } from 'vitest';
import { decodeTextBuffer } from './textDecode';

function toBuffer(bytes: number[]): ArrayBuffer {
  return new Uint8Array(bytes).buffer;
}

describe('decodeTextBuffer', () => {
  it('UTF-8 の日本語をそのまま復元する', () => {
    // 「こんにちは」の UTF-8 バイト列
    const buf = new TextEncoder().encode('こんにちは').buffer;
    expect(decodeTextBuffer(buf)).toBe('こんにちは');
  });

  it('Shift_JIS バイト列を正しく復元する(「あ」)', () => {
    const buf = toBuffer([0x82, 0xa0]);
    expect(decodeTextBuffer(buf)).toBe('あ');
  });

  it('Shift_JIS バイト列を正しく復元する(「テスト」)', () => {
    const buf = toBuffer([0x83, 0x65, 0x83, 0x58, 0x83, 0x67]);
    expect(decodeTextBuffer(buf)).toBe('テスト');
  });

  it('UTF-8 BOM 付きバイト列は BOM が除去されて復元される', () => {
    const withoutBom = new TextEncoder().encode('こんにちは');
    const buf = new Uint8Array([0xef, 0xbb, 0xbf, ...withoutBom]).buffer;
    expect(decodeTextBuffer(buf)).toBe('こんにちは');
  });

  it('UTF-16LE BOM 付きバイト列を復元する(「あ」)', () => {
    // FF FE (BOM) + 0x3042 の LE バイト列(42 30)
    const buf = toBuffer([0xff, 0xfe, 0x42, 0x30]);
    expect(decodeTextBuffer(buf)).toBe('あ');
  });

  it('UTF-16BE BOM 付きバイト列を復元する(「あ」)', () => {
    // FE FF (BOM) + 0x3042 の BE バイト列(30 42)
    const buf = toBuffer([0xfe, 0xff, 0x30, 0x42]);
    expect(decodeTextBuffer(buf)).toBe('あ');
  });

  it('空バッファは空文字列を返す', () => {
    const buf = toBuffer([]);
    expect(decodeTextBuffer(buf)).toBe('');
  });
});
