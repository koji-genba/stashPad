// WorkDetailPage のテスト。
// 不正な ID(/works/abc 等)で無限スピナーにならず「作品が見つかりません」に落ちること、
// および正常 ID では詳細が表示されることを検証する(issue #56)。
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import type { WorkDetail } from '@/api/types';

// API クライアントをモック化
vi.mock('@/api/client', () => ({
  fetchWork: vi.fn(),
  refreshThumbnail: vi.fn().mockResolvedValue({ refreshed: false }),
  addCustomTag: vi.fn(),
  removeTag: vi.fn(),
  setWorkHidden: vi.fn(),
  setWorkFavorite: vi.fn(),
  setWorkRating: vi.fn(),
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
import { fetchWork, setWorkFavorite, setWorkRating } from '@/api/client';

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
  favorited: false,
  rating: null,
  tags: [],
  thumbnail_url: null,
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

describe('WorkDetailPage 評価(issue #95)', () => {
  afterEach(() => {
    cleanup();
    vi.mocked(fetchWork).mockReset();
    vi.mocked(setWorkRating).mockReset();
  });

  it('未評価の作品で 5 つ目の星を押すと rating=5 を保存し、表示が ★★★★★ になる', async () => {
    vi.mocked(fetchWork).mockResolvedValue(sampleWork);
    vi.mocked(setWorkRating).mockResolvedValue(undefined);
    renderPage('/works/1');

    await screen.findByText('テスト作品');

    fireEvent.click(screen.getByRole('button', { name: '評価を5にする' }));

    expect(setWorkRating).toHaveBeenCalledWith(1, 5);
    await waitFor(() => {
      const stars = screen.getAllByRole('button', { pressed: true }).filter((button) =>
        button.className.includes('ratingStar'),
      );
      expect(stars).toHaveLength(5);
      expect(screen.getByRole('button', { name: '評価を解除' })).toHaveTextContent('★');
    });
  });

  it('評価済みの現在値をもう一度押すと rating=null で解除する', async () => {
    vi.mocked(fetchWork).mockResolvedValue({ ...sampleWork, rating: 3 });
    vi.mocked(setWorkRating).mockResolvedValue(undefined);
    renderPage('/works/1');

    await screen.findByText('テスト作品');

    fireEvent.click(screen.getByRole('button', { name: '評価を解除' }));

    expect(setWorkRating).toHaveBeenCalledWith(1, null);
    await waitFor(() => {
      expect(screen.getByRole('button', { name: '評価を3にする' })).toHaveTextContent('☆');
    });
  });
});

describe('WorkDetailPage カスタムタグの視認性(issue #98)', () => {
  afterEach(() => {
    cleanup();
    vi.mocked(fetchWork).mockReset();
  });

  it('カスタムタグには通常タグと区別する専用スタイルが付く', async () => {
    vi.mocked(fetchWork).mockResolvedValue({
      ...sampleWork,
      tags: [
        { id: 1, name: '睡眠用', category: 'custom' },
        { id: 2, name: 'ASMR', category: 'genre' },
      ],
    });
    renderPage('/works/1');

    await screen.findByText('睡眠用');

    expect(screen.getByText('睡眠用').closest('span')?.className).toContain('tagCustom');
    expect(screen.getByText('ASMR').closest('span')?.className).not.toContain('tagCustom');
  });
});

describe('WorkDetailPage お気に入りトグル(issue #72)', () => {
  afterEach(() => {
    cleanup();
    vi.mocked(fetchWork).mockReset();
    vi.mocked(setWorkFavorite).mockReset();
  });

  it('未登録(favorited: false)では ☆ ボタンが表示され、クリックで登録されて ★ に変わる', async () => {
    vi.mocked(fetchWork).mockResolvedValue(sampleWork);
    vi.mocked(setWorkFavorite).mockResolvedValue(undefined);
    renderPage('/works/1');

    await waitFor(() => {
      expect(screen.getByText('テスト作品')).toBeTruthy();
    });

    const btn = screen.getByRole('button', { name: 'お気に入りに追加' });
    fireEvent.click(btn);

    expect(setWorkFavorite).toHaveBeenCalledWith(1, true);
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'お気に入りから削除' })).toBeInTheDocument();
    });
  });

  it('登録済み(favorited: true)では ★ ボタンが表示され、クリックで解除されて ☆ に変わる', async () => {
    vi.mocked(fetchWork).mockResolvedValue({ ...sampleWork, favorited: true });
    vi.mocked(setWorkFavorite).mockResolvedValue(undefined);
    renderPage('/works/1');

    await waitFor(() => {
      expect(screen.getByText('テスト作品')).toBeTruthy();
    });

    const btn = screen.getByRole('button', { name: 'お気に入りから削除' });
    fireEvent.click(btn);

    expect(setWorkFavorite).toHaveBeenCalledWith(1, false);
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'お気に入りに追加' })).toBeInTheDocument();
    });
  });
});
