// WorkDetailPage のテスト。
// 不正な ID(/works/abc 等)で無限スピナーにならず「作品が見つかりません」に落ちること、
// および正常 ID では詳細が表示されることを検証する(issue #56)。
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { WorkDetail } from '@/api/types';

// API クライアントをモック化
vi.mock('@/api/client', () => ({
  fetchWork: vi.fn(),
  refreshThumbnail: vi.fn().mockResolvedValue({ refreshed: false }),
  addCustomTag: vi.fn(),
  removeTag: vi.fn(),
  setWorkHidden: vi.fn(),
  thumbnailUrl: (workId: number) => `/api/works/${workId}/thumbnail`,
}));

// listSearchMemory をモック化(戻りリンクの URL 生成のみ)
vi.mock('@/lib/listSearchMemory', () => ({
  listBackPath: () => '/',
}));

// 内部で fetch を行うサブコンポーネントをモック化
vi.mock('@/components/FileBrowser', () => ({ default: () => null }));
vi.mock('@/components/Thumbnail', () => ({ default: () => null }));

import WorkDetailPage from './WorkDetailPage';
import { fetchWork } from '@/api/client';

const sampleWork: WorkDetail = {
  id: 1,
  rj_number: 'RJ404669',
  title: 'テスト作品',
  circle: null,
  series_name: null,
  purchase_date: null,
  work_type: null,
  age_rating: null,
  file_format: null,
  file_size_text: null,
  has_folder: false,
  hidden: false,
  tags: [],
};

function renderPage(initialUrl: string) {
  return render(
    <MemoryRouter initialEntries={[initialUrl]}>
      <Routes>
        <Route path="/works/:id" element={<WorkDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('WorkDetailPage 不正 ID の堅牢性', () => {
  afterEach(() => {
    cleanup();
    vi.mocked(fetchWork).mockReset();
  });

  it('数値でない ID では「作品が見つかりません」を表示しスピナーを出し続けない', async () => {
    renderPage('/works/abc');
    await waitFor(() => {
      expect(screen.getByText('作品が見つかりません')).toBeTruthy();
    });
    expect(document.querySelector('.spinner')).toBeNull();
    // 不正 ID では API を呼ばない
    expect(fetchWork).not.toHaveBeenCalled();
  });

  it('正常な ID では読み込み後に作品タイトルを表示する', async () => {
    vi.mocked(fetchWork).mockResolvedValue(sampleWork);
    renderPage('/works/1');
    await waitFor(() => {
      expect(screen.getByText('テスト作品')).toBeTruthy();
    });
    expect(fetchWork).toHaveBeenCalledWith(1, expect.anything());
  });
});
