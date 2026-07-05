// playbackMemory のユニットテスト。
// localStorage キー stashpad-playback-positions への保存/復元・削除ルール・
// LRU 上限・throttle・破損 JSON 耐性を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  _resetForTest,
  clearProgress,
  flushProgress,
  loadResumePosition,
  recordProgress,
} from './playbackMemory';

const STORAGE_KEY = 'stashpad-playback-positions';

function rawStore(): Record<string, { position: number; duration: number; updatedAt: number }> {
  const raw = localStorage.getItem(STORAGE_KEY);
  return raw ? JSON.parse(raw) : {};
}

beforeEach(() => {
  localStorage.clear();
  _resetForTest();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('loadResumePosition', () => {
  it('未保存のときは null を返す', () => {
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });

  it('recordProgress で保存した position を返す', () => {
    recordProgress(1, 'a.mp3', 100, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(100);
  });

  it('workId:path の組み合わせごとに別エントリとして扱う', () => {
    recordProgress(1, 'a.mp3', 100, 300);
    recordProgress(2, 'a.mp3', 200, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(100);
    expect(loadResumePosition(2, 'a.mp3')).toBe(200);
  });
});

describe('recordProgress 保存ルール', () => {
  it('position < 30 秒はエントリを保存しない', () => {
    recordProgress(1, 'a.mp3', 29, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });

  it('position が 30 秒ちょうどは保存する', () => {
    recordProgress(1, 'a.mp3', 30, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(30);
  });

  it('duration - position < 30 秒(終端付近)はエントリを保存しない', () => {
    recordProgress(1, 'a.mp3', 280, 300); // 残り20秒
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });

  it('既存エントリが終端付近の再保存で削除される', () => {
    recordProgress(1, 'a.mp3', 100, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(100);
    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.now() + 10_000));
    recordProgress(1, 'a.mp3', 285, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });

  it('duration が 0(未取得)のときは終端ルールを適用せず保存する', () => {
    recordProgress(1, 'a.mp3', 100, 0);
    expect(loadResumePosition(1, 'a.mp3')).toBe(100);
  });

  it('通常範囲の position は upsert される', () => {
    recordProgress(1, 'a.mp3', 100, 300);
    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.now() + 10_000));
    recordProgress(1, 'a.mp3', 150, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(150);
  });
});

describe('recordProgress の throttle(5秒)', () => {
  it('5秒以内の連続呼び出しは 2 回目以降を無視する', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(1_000_000));
    recordProgress(1, 'a.mp3', 100, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(100);

    vi.setSystemTime(new Date(1_000_000 + 2_000)); // 2秒後
    recordProgress(1, 'a.mp3', 120, 300);
    // throttle により無視され、100 のまま
    expect(loadResumePosition(1, 'a.mp3')).toBe(100);
  });

  it('5秒経過後は保存される', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(1_000_000));
    recordProgress(1, 'a.mp3', 100, 300);

    vi.setSystemTime(new Date(1_000_000 + 5_001));
    recordProgress(1, 'a.mp3', 130, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(130);
  });

  it('throttle はキーごとに独立している', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(1_000_000));
    recordProgress(1, 'a.mp3', 100, 300);
    recordProgress(2, 'b.mp3', 200, 300); // 別キーなので throttle されない

    expect(loadResumePosition(1, 'a.mp3')).toBe(100);
    expect(loadResumePosition(2, 'b.mp3')).toBe(200);
  });
});

describe('flushProgress', () => {
  it('throttle を無視して即座に保存する', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(1_000_000));
    recordProgress(1, 'a.mp3', 100, 300);

    vi.setSystemTime(new Date(1_000_000 + 100)); // 5秒未満
    flushProgress(1, 'a.mp3', 140, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(140);
  });

  it('削除ルールも適用される(position < 30)', () => {
    recordProgress(1, 'a.mp3', 100, 300);
    flushProgress(1, 'a.mp3', 5, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });
});

describe('clearProgress', () => {
  it('保存済みエントリを削除する', () => {
    recordProgress(1, 'a.mp3', 100, 300);
    expect(loadResumePosition(1, 'a.mp3')).toBe(100);
    clearProgress(1, 'a.mp3');
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });

  it('存在しないエントリに対しても例外を投げない', () => {
    expect(() => clearProgress(999, 'nope.mp3')).not.toThrow();
  });
});

describe('LRU 上限 200 件', () => {
  it('201 件目を追加すると最も古い updatedAt のエントリが削除される', () => {
    vi.useFakeTimers();
    const base = 1_000_000;
    for (let i = 0; i < 200; i++) {
      vi.setSystemTime(new Date(base + i * 10_000)); // throttle(5秒)を超える間隔
      recordProgress(i, `track-${i}.mp3`, 100, 300);
    }
    expect(Object.keys(rawStore())).toHaveLength(200);
    expect(loadResumePosition(0, 'track-0.mp3')).toBe(100);

    // 201 件目を追加 → 最古(workId=0)が追い出される
    vi.setSystemTime(new Date(base + 200 * 10_000));
    recordProgress(200, 'track-200.mp3', 100, 300);

    expect(Object.keys(rawStore())).toHaveLength(200);
    expect(loadResumePosition(0, 'track-0.mp3')).toBeNull();
    expect(loadResumePosition(200, 'track-200.mp3')).toBe(100);
    // 2番目に古かったものはまだ残っている
    expect(loadResumePosition(1, 'track-1.mp3')).toBe(100);
  });
});

describe('破損データ耐性', () => {
  it('localStorage の JSON が壊れていても例外を投げず null を返す', () => {
    localStorage.setItem(STORAGE_KEY, '{not valid json');
    expect(() => loadResumePosition(1, 'a.mp3')).not.toThrow();
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });

  it('壊れた JSON があっても recordProgress は例外を投げず上書き保存できる', () => {
    localStorage.setItem(STORAGE_KEY, '{not valid json');
    expect(() => recordProgress(1, 'a.mp3', 100, 300)).not.toThrow();
    expect(loadResumePosition(1, 'a.mp3')).toBe(100);
  });

  it('localStorage.setItem が例外を投げても recordProgress は例外を投げない(quota 超過等)', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError');
    });
    expect(() => recordProgress(1, 'a.mp3', 100, 300)).not.toThrow();
    spy.mockRestore();
  });

  it('localStorage.getItem が例外を投げても loadResumePosition は例外を投げず null を返す', () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError');
    });
    expect(() => loadResumePosition(1, 'a.mp3')).not.toThrow();
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
    spy.mockRestore();
  });
});

// PR #79 レビュー: 「妥当な JSON だが形が壊れている」エントリへの耐性。
// JSON.parse 自体は成功するが、エントリが null だったり position が数値でないなど
// PositionEntry の形を満たさない場合、loadResumePosition が誤った値を返したり、
// LRU 上限到達時に enforceLru が updatedAt 読み取りで例外を投げる穴があった。
describe('妥当な JSON だが形が壊れているエントリへの耐性', () => {
  it('エントリが null の場合は無視して null を返す', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ '1:a.mp3': null }));
    expect(() => loadResumePosition(1, 'a.mp3')).not.toThrow();
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });

  it('position が文字列の場合は無視して null を返す(数値のふりをした不正値を弾く)', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ '1:a.mp3': { position: '100', duration: 300, updatedAt: Date.now() } }),
    );
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
  });

  it('duration や updatedAt が数値でないエントリも無視して null を返す', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        '1:a.mp3': { position: 100, duration: 'nope', updatedAt: Date.now() },
        '2:b.mp3': { position: 100, duration: 300, updatedAt: null },
      }),
    );
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
    expect(loadResumePosition(2, 'b.mp3')).toBeNull();
  });

  it('壊れた形のエントリが混在していても recordProgress は例外を投げず新たな値を保存できる', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        '1:a.mp3': null,
        '2:b.mp3': { position: 'bad', duration: 300, updatedAt: Date.now() },
      }),
    );
    expect(() => recordProgress(3, 'c.mp3', 100, 300)).not.toThrow();
    expect(loadResumePosition(3, 'c.mp3')).toBe(100);
    // 壊れたエントリは読み込み時に捨てられている(書き戻したストアにも残らない)
    expect(loadResumePosition(1, 'a.mp3')).toBeNull();
    expect(loadResumePosition(2, 'b.mp3')).toBeNull();
  });

  it('LRU 上限到達時に不正エントリ(null)が混在していても enforceLru が例外を投げない', () => {
    vi.useFakeTimers();
    const base = 1_000_000;
    const seed: Record<string, unknown> = {};
    for (let i = 0; i < 199; i++) {
      seed[`seed-${i}:t.mp3`] = { position: 100, duration: 300, updatedAt: base + i * 10_000 };
    }
    // 壊れたエントリ(null)を混在させる。修正前は enforceLru の
    // `store[keys[i]].updatedAt` 読み取りで TypeError を投げていた。
    seed['broken:entry.mp3'] = null;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(seed)); // 199 valid + 1 broken = 200 件

    vi.setSystemTime(new Date(base + 199 * 10_000));
    expect(() => recordProgress(999, 'new.mp3', 100, 300)).not.toThrow();
    expect(loadResumePosition(999, 'new.mp3')).toBe(100);
  });
});
