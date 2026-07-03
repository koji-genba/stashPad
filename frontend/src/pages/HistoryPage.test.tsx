// HistoryPage のテスト。
// 履歴行の削除ボタン(confirm → API 呼び出し → 一覧からの除去)を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';

vi.mock('@/api/client', () => {
  const historyItems = [
    {
      work: { id: 1, title: '猫の物語', thumbnail_url: '/api/works/1/thumbnail' },
      last_played_at: '2026-01-02T10:00:00Z',
      last_file_path: 'a/02.mp3',
      play_count: 2,
    },
    {
      work: { id: 2, title: '犬の日記', thumbnail_url: '/api/works/2/thumbnail' },
      last_played_at: '2026-01-01T10:00:00Z',
      last_file_path: 'b/01.mp3',
      play_count: 1,
    },
  ];
  return {
    fetchHistory: vi.fn().mockResolvedValue({ items: historyItems, page: 1, limit: 40 }),
    deleteHistory: vi.fn().mockResolvedValue({ deleted: 1 }),
  };
});

import HistoryPage from './HistoryPage';
import { deleteHistory } from '@/api/client';

function renderPage() {
  return render(
    <MemoryRouter>
      <HistoryPage />
    </MemoryRouter>,
  );
}

describe('HistoryPage 履歴削除', () => {
  beforeEach(() => {
    vi.mocked(deleteHistory).mockClear();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('削除ボタン押下 → confirm OK で deleteHistory が呼ばれ、行が一覧から消える', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    renderPage();

    await screen.findByText('猫の物語');

    const deleteButtons = screen.getAllByRole('button', { name: 'この作品の履歴を削除' });
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => expect(deleteHistory).toHaveBeenCalledWith(1));

    await waitFor(() => expect(screen.queryByText('猫の物語')).toBeNull());
    expect(screen.getByText('犬の日記')).toBeTruthy();
  });

  it('confirm でキャンセルした場合は deleteHistory を呼ばない', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    renderPage();

    await screen.findByText('猫の物語');
    const deleteButtons = screen.getAllByRole('button', { name: 'この作品の履歴を削除' });
    fireEvent.click(deleteButtons[0]);

    expect(deleteHistory).not.toHaveBeenCalled();
    expect(screen.getByText('猫の物語')).toBeTruthy();
  });
});
