// WorksListPage のテスト。
// フィルタドロワー開閉時の body スクロールロック動作を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter, useSearchParams } from 'react-router-dom';

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
import { fetchWorks } from '@/api/client';

// useSearchParams の現在値をテストから直接覗くためのプローブコンポーネント。
// 単独レンダー時は `getParams` を呼ばないテストには無影響。
function renderPage(initialUrl = '/') {
  let currentParams = new URLSearchParams();
  function Probe() {
    const [p] = useSearchParams();
    currentParams = p;
    return null;
  }
  const utils = render(
    <MemoryRouter initialEntries={[initialUrl]}>
      <WorksListPage />
      <Probe />
    </MemoryRouter>,
  );
  return { ...utils, getParams: () => currentParams };
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

describe('フィルタ操作時のスクロールトップ (#36)', () => {
  // items が空だと「該当する作品がありません」分岐になりページャーが出ない。
  // ページネーションを確認するためダミーアイテムを含む応答を使う。
  const PAGINATED_RESPONSE = {
    items: [
      {
        id: 1,
        rj_number: 'RJ000001',
        title: 'テスト作品',
        circle: 'テストサークル',
        age_rating: 'general',
        has_folder: true,
        thumbnail_url: '/api/works/1/thumbnail',
      },
    ],
    total: 100,
    limit: 40,
    page: 1,
  };

  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    // ページャーが出るように件数を多くしアイテムを 1 件含める
    vi.mocked(fetchWorks).mockResolvedValue(PAGINATED_RESPONSE);
  });

  afterEach(() => {
    cleanup();
    // デフォルトモック値に戻す
    vi.mocked(fetchWorks).mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 });
    vi.restoreAllMocks();
  });

  it('goPage でページ送りすると window.scrollTo(0, 0) が呼ばれる', async () => {
    renderPage();

    // スピナーが消えてページャーが表示されるまで待つ
    // total:100 / limit:40 = 3 ページなので「次へ」ボタンが出るはず
    await waitFor(() => {
      expect(screen.getByRole('button', { name: '次へ' })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: '次へ' }));

    expect(window.scrollTo).toHaveBeenCalledWith(0, 0);
  });

  it('ソートを変更すると window.scrollTo(0, 0) が呼ばれる', async () => {
    renderPage();

    const sortSelect = screen.getByRole('combobox', { name: '並び替え' });
    fireEvent.change(sortSelect, { target: { value: 'title' } });

    expect(window.scrollTo).toHaveBeenCalledWith(0, 0);
  });

  it('circle チップの ✕ をクリックすると window.scrollTo(0, 0) が呼ばれる', async () => {
    renderPage('/?circle=TestCircle');

    // サークルチップが表示されるまで待つ
    await waitFor(() => {
      expect(screen.getByTitle('クリックで解除')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByTitle('クリックで解除'));

    expect(window.scrollTo).toHaveBeenCalledWith(0, 0);
  });
});

describe('全てクリアボタン (#30)', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    // 他 describe の状態に依存せず自己完結させる
    vi.mocked(fetchWorks).mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('フィルタが何もないとき (初期 URL /) 「全てクリア」ボタンは表示されない', () => {
    renderPage('/');
    expect(screen.queryByRole('button', { name: '全てクリア' })).not.toBeInTheDocument();
  });

  it('URL ?circle=Foo で開くと「全てクリア」ボタンが表示される', async () => {
    renderPage('/?circle=Foo');
    await waitFor(() => {
      expect(screen.getByRole('button', { name: '全てクリア' })).toBeInTheDocument();
    });
  });

  it('「全てクリア」をクリックすると URL から q / tags / circle が全て消える', async () => {
    const { getParams } = renderPage('/?q=hello&circle=Foo&tags=1&exclude_tags=2&series=Bar&page=3');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: '全てクリア' })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: '全てクリア' }));

    await waitFor(() => {
      const p = getParams();
      expect(p.has('q')).toBe(false);
      expect(p.has('tags')).toBe(false);
      expect(p.has('exclude_tags')).toBe(false);
      expect(p.has('circle')).toBe(false);
      expect(p.has('series')).toBe(false);
      expect(p.has('page')).toBe(false);
    });
  });

  it('「全てクリア」をクリックすると検索入力欄も空になる', async () => {
    // q だけでは chips が出ないため circle も同時に指定する
    renderPage('/?q=hello&circle=Foo');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: '全てクリア' })).toBeInTheDocument();
    });

    // 入力欄に初期値 hello が入っていることを確認
    const input = screen.getByRole('searchbox');
    expect(input).toHaveValue('hello');

    fireEvent.click(screen.getByRole('button', { name: '全てクリア' }));

    await waitFor(() => {
      expect(input).toHaveValue('');
    });
  });

  it('「全てクリア」をクリックすると window.scrollTo(0, 0) が呼ばれる', async () => {
    renderPage('/?circle=Foo');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: '全てクリア' })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: '全てクリア' }));

    expect(window.scrollTo).toHaveBeenCalledWith(0, 0);
  });
});
