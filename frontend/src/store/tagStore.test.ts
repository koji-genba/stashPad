// tagStore の状態遷移テスト。
// 各テスト前にストアを初期状態にリセットする。
import { beforeEach, describe, expect, it, vi } from 'vitest';

// fetchTags をモック化して実際の HTTP リクエストを発生させない。
// vi.mock は巻き上げられるので import より前に宣言する。
vi.mock('@/api/client', () => ({
  fetchTags: vi.fn(),
}));

import { useTagStore, useTagNameMap } from './tagStore';
import { fetchTags } from '@/api/client';
import type { TagFacet } from '@/api/types';
import { renderHook } from '@testing-library/react';

const fetchTagsMock = vi.mocked(fetchTags);

// テスト間のストア汚染を防ぐため、各テスト前に初期状態へリセットする。
function resetStore() {
  useTagStore.setState(
    { items: [], loaded: false, loading: false, error: null },
    false,
  );
}

// テスト用の TagFacet フィクスチャ生成ヘルパ
function makeTag(id: number, name: string): TagFacet {
  return { id, name, category: 'genre', work_count: id };
}

const SAMPLE_TAGS: TagFacet[] = [
  makeTag(1, 'ファンタジー'),
  makeTag(2, '現代'),
  makeTag(3, 'SF'),
];

describe('tagStore 初期状態', () => {
  beforeEach(resetStore);

  it('items=[], loaded=false, loading=false, error=null', () => {
    const s = useTagStore.getState();
    expect(s.items).toEqual([]);
    expect(s.loaded).toBe(false);
    expect(s.loading).toBe(false);
    expect(s.error).toBeNull();
  });
});

describe('ensureLoaded', () => {
  beforeEach(() => {
    resetStore();
    fetchTagsMock.mockClear();
  });

  it('1 回呼ぶと fetchTags が 1 回呼ばれ、items と loaded が埋まる', async () => {
    fetchTagsMock.mockResolvedValueOnce({ items: SAMPLE_TAGS });

    await useTagStore.getState().ensureLoaded();

    expect(fetchTagsMock).toHaveBeenCalledTimes(1);
    const s = useTagStore.getState();
    expect(s.items).toEqual(SAMPLE_TAGS);
    expect(s.loaded).toBe(true);
    expect(s.loading).toBe(false);
  });

  it('連続 3 回呼んでも fetchTags は 1 回しか呼ばれない(loading 中の重複防止)', async () => {
    // 解決を遅延させて 3 回目の call が loading=true に当たるようにする
    let resolve!: (value: { items: TagFacet[] }) => void;
    fetchTagsMock.mockReturnValueOnce(
      new Promise<{ items: TagFacet[] }>((r) => (resolve = r)),
    );

    const p1 = useTagStore.getState().ensureLoaded();
    const p2 = useTagStore.getState().ensureLoaded();
    const p3 = useTagStore.getState().ensureLoaded();

    resolve({ items: SAMPLE_TAGS });
    await Promise.all([p1, p2, p3]);

    expect(fetchTagsMock).toHaveBeenCalledTimes(1);
  });

  it('完了後に再度 ensureLoaded を呼んでも fetchTags は呼ばれない(loaded ガード)', async () => {
    fetchTagsMock.mockResolvedValueOnce({ items: SAMPLE_TAGS });

    await useTagStore.getState().ensureLoaded();
    await useTagStore.getState().ensureLoaded();
    await useTagStore.getState().ensureLoaded();

    expect(fetchTagsMock).toHaveBeenCalledTimes(1);
  });

  it('fetch エラー時に error がセットされ、loading=false に戻る', async () => {
    fetchTagsMock.mockRejectedValueOnce(new Error('Network error'));

    await useTagStore.getState().ensureLoaded();

    const s = useTagStore.getState();
    expect(s.error).toBe('Network error');
    expect(s.loading).toBe(false);
    expect(s.loaded).toBe(false);
  });
});

describe('refresh', () => {
  beforeEach(() => {
    resetStore();
    fetchTagsMock.mockClear();
  });

  it('loaded=false のとき refresh を呼ぶと fetchTags が実行される', async () => {
    fetchTagsMock.mockResolvedValueOnce({ items: SAMPLE_TAGS });

    await useTagStore.getState().refresh();

    expect(fetchTagsMock).toHaveBeenCalledTimes(1);
    expect(useTagStore.getState().loaded).toBe(true);
  });

  it('loaded=true でも refresh を呼ぶと強制再取得される', async () => {
    // 最初のロード
    fetchTagsMock.mockResolvedValueOnce({ items: SAMPLE_TAGS });
    await useTagStore.getState().ensureLoaded();

    // refresh で再取得
    const newTags = [makeTag(10, '新しいタグ')];
    fetchTagsMock.mockResolvedValueOnce({ items: newTags });
    await useTagStore.getState().refresh();

    expect(fetchTagsMock).toHaveBeenCalledTimes(2);
    expect(useTagStore.getState().items).toEqual(newTags);
  });
});

describe('useTagNameMap', () => {
  beforeEach(resetStore);

  it('items に 3 件入った状態で正しい id→name の Map を返す', () => {
    useTagStore.setState({ items: SAMPLE_TAGS });

    const { result } = renderHook(() => useTagNameMap());
    const map = result.current;

    expect(map.size).toBe(3);
    expect(map.get(1)).toBe('ファンタジー');
    expect(map.get(2)).toBe('現代');
    expect(map.get(3)).toBe('SF');
  });

  it('items が空のとき空の Map を返す', () => {
    useTagStore.setState({ items: [] });

    const { result } = renderHook(() => useTagNameMap());
    expect(result.current.size).toBe(0);
  });
});
