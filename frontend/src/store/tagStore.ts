// タグ一覧の共有キャッシュストア。
// WorksListPage と TagFacetPanel の両方が「全タグ取得」を必要とするため、
// ここで一度だけ fetchTags({}) を呼び、結果を共有する。
//
// 共有フェッチには AbortSignal を渡さない: 呼び出し元 A の signal で abort
// すると、B が早期 return で再フェッチを諦めたあとにキャッシュが空のまま
// 固まる構造的な脆さを避ける(レビュー #1)。
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
  ensureLoaded: () => Promise<void>;
  /** loaded 状態に関わらず強制再取得する。CSV インポート等の書き換え後に呼ぶ */
  refresh: () => Promise<void>;
}

async function fetchAndStore(set: (partial: Partial<TagState>) => void): Promise<void> {
  set({ loading: true, error: null });
  try {
    const data = await fetchTags({});
    set({ items: data.items, loaded: true, loading: false });
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : '取得失敗';
    set({ error: message, loading: false });
  }
}

export const useTagStore = create<TagState>()((set, get) => ({
  items: [],
  loaded: false,
  loading: false,
  error: null,

  ensureLoaded: async () => {
    // すでにロード済み or ロード中なら多重発火しない
    const { loaded, loading } = get();
    if (loaded || loading) return;
    await fetchAndStore(set);
  },

  refresh: async () => {
    await fetchAndStore(set);
  },
}));

/** タグ id → name の Map を返すセレクタフック。items が変わるときだけ再計算する。 */
export function useTagNameMap(): Map<number, string> {
  const items = useTagStore((s) => s.items);
  return useMemo(() => new Map(items.map((t) => [t.id, t.name])), [items]);
}
