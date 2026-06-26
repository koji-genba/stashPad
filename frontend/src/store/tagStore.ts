// タグ一覧の共有キャッシュストア。
// WorksListPage と TagFacetPanel の両方が「全タグ取得」を必要とするため、
// ここで一度だけ fetchTags({}) を呼び、結果を共有する。
import { useMemo } from 'react';
import { create } from 'zustand';
import type { TagFacet } from '@/api/types';
import { fetchTags } from '@/api/client';

interface TagState {
  items: TagFacet[];
  loaded: boolean;
  loading: boolean;
  error: string | null;

  /** ロードされていない場合のみ fetchTags({}) を実行する(並列呼び出し安全) */
  ensureLoaded: (signal?: AbortSignal) => Promise<void>;
  /** loaded 状態に関わらず強制再取得する */
  refresh: (signal?: AbortSignal) => Promise<void>;
}

export const useTagStore = create<TagState>()((set, get) => ({
  items: [],
  loaded: false,
  loading: false,
  error: null,

  ensureLoaded: async (signal?: AbortSignal) => {
    // すでにロード済み or ロード中なら多重発火しない
    const { loaded, loading } = get();
    if (loaded || loading) return;

    set({ loading: true, error: null });
    try {
      const data = await fetchTags({}, signal);
      set({ items: data.items, loaded: true, loading: false });
    } catch (e: unknown) {
      // AbortError はユーザー操作によるキャンセルなので無視する
      if (e instanceof Error && e.name === 'AbortError') {
        set({ loading: false });
        return;
      }
      const message = e instanceof Error ? e.message : '取得失敗';
      set({ error: message, loading: false });
    }
  },

  refresh: async (signal?: AbortSignal) => {
    set({ loading: true, error: null });
    try {
      const data = await fetchTags({}, signal);
      set({ items: data.items, loaded: true, loading: false });
    } catch (e: unknown) {
      if (e instanceof Error && e.name === 'AbortError') {
        set({ loading: false });
        return;
      }
      const message = e instanceof Error ? e.message : '取得失敗';
      set({ error: message, loading: false });
    }
  },
}));

/** タグ id → name の Map を返すセレクタフック。items が変わるときだけ再計算する。 */
export function useTagNameMap(): Map<number, string> {
  const items = useTagStore((s) => s.items);
  return useMemo(() => new Map(items.map((t) => [t.id, t.name])), [items]);
}
