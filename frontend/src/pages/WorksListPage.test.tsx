// WorksListPage のテスト。
// フィルタドロワー開閉時の body スクロールロック動作を検証する。
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';

// API クライアントをモック化
vi.mock('@/api/client', () => ({
  fetchWorks: vi.fn().mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 }),
  fetchTags: vi.fn().mockResolvedValue({ items: [] }),
  thumbnailUrl: (workId: number) => `/api/works/${workId}/thumbnail`,
}));

// tagStore をモック化(実際のフェッチを行わない)
vi.mock('@/store/tagStore', () => ({
  useTagStore: {
    getState: () => ({ ensureLoaded: vi.fn() }),
  },
  useTagNameMap: () => new Map<number, string>(),
}));

// listSearchMemory をモック化
vi.mock('@/lib/listSearchMemory', () => ({
  saveListSearch: vi.fn(),
}));

// サブコンポーネントをモック化(内部で fetch 等を行うため)
vi.mock('@/components/TagFacetPanel', () => ({ default: () => null }));
vi.mock('@/components/CircleFacetPanel', () => ({ default: () => null }));
vi.mock('@/components/WorkCard', () => ({ default: () => null }));

import WorksListPage from './WorksListPage';

function renderPage() {
  return render(
    <MemoryRouter>
      <WorksListPage />
    </MemoryRouter>,
  );
}

describe('WorksListPage body スクロールロック', () => {
  afterEach(() => {
    cleanup();
    // body.style.overflow をリセット
    document.body.style.overflow = '';
  });

  it('タグ絞り込みボタンを押すとドロワーが開き document.body.style.overflow が "hidden" になる', async () => {
    renderPage();
    const btn = screen.getByRole('button', { name: /タグ絞り込み/ });
    fireEvent.click(btn);
    await waitFor(() => {
      expect(document.body.style.overflow).toBe('hidden');
    });
  });

  it('ドロワーの閉じるボタンを押して閉じると document.body.style.overflow が元に戻る', async () => {
    renderPage();
    // ドロワーを開く
    const openBtn = screen.getByRole('button', { name: /タグ絞り込み/ });
    fireEvent.click(openBtn);
    await waitFor(() => {
      expect(document.body.style.overflow).toBe('hidden');
    });
    // ✕ ボタン(aria-label="閉じる")で閉じる
    const closeBtn = screen.getByRole('button', { name: '閉じる' });
    fireEvent.click(closeBtn);
    await waitFor(() => {
      expect(document.body.style.overflow).not.toBe('hidden');
    });
  });
});
