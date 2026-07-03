// SettingsPage のテスト。
// メンテナンス節の「再生履歴を全削除」ボタン(confirm → API 呼び出し → 結果表示)を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';

vi.mock('@/api/client', () => ({
  cleanupTags: vi.fn(),
  fetchThumbnailRebuildStatus: vi.fn().mockResolvedValue({
    running: false,
    checked: 0,
    regenerated: 0,
    total: 0,
  }),
  fetchWorks: vi.fn().mockResolvedValue({ items: [], total: 0, limit: 200, page: 1 }),
  importCsv: vi.fn(),
  rebuildThumbnails: vi.fn(),
  runScan: vi.fn(),
  setWorkHidden: vi.fn(),
  deleteHistory: vi.fn().mockResolvedValue({ deleted: 5 }),
}));

vi.mock('@/store/tagStore', () => ({
  useTagStore: {
    getState: () => ({ refresh: vi.fn() }),
  },
}));

import SettingsPage from './SettingsPage';
import { deleteHistory } from '@/api/client';

function renderPage() {
  return render(
    <MemoryRouter>
      <SettingsPage />
    </MemoryRouter>,
  );
}

describe('SettingsPage 再生履歴の全削除', () => {
  beforeEach(() => {
    vi.mocked(deleteHistory).mockClear();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('confirm OK で deleteHistory() が呼ばれ、削除件数が表示される', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    renderPage();

    const btn = await screen.findByRole('button', { name: '再生履歴を全削除' });
    fireEvent.click(btn);

    expect(confirmSpy).toHaveBeenCalledWith(
      '再生履歴を全て削除しますか?この操作は取り消せません',
    );
    await waitFor(() => expect(deleteHistory).toHaveBeenCalledWith());

    await screen.findByText((_, node) => node?.textContent === '削除 5 件');
  });

  it('confirm でキャンセルした場合は deleteHistory を呼ばない', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    renderPage();

    const btn = await screen.findByRole('button', { name: '再生履歴を全削除' });
    fireEvent.click(btn);

    expect(deleteHistory).not.toHaveBeenCalled();
  });
});
