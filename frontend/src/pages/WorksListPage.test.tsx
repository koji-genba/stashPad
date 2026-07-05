// WorksListPage のテスト。
// フィルタドロワー開閉時の body スクロールロック動作を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter, useNavigationType, useSearchParams } from 'react-router-dom';

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

// ページャー検証用のダミー応答(3 ページ分)。#35 のテストでも使い回す。
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
      favorited: false,
    },
  ],
  total: 100,
  limit: 40,
  page: 1,
};

// useSearchParams の現在値・現在の history 遷移種別をテストから直接覗くための
// プローブコンポーネント。単独レンダー時は getParams/getNavType を呼ばないテストには無影響。
function renderPage(initialUrl = '/') {
  let currentParams = new URLSearchParams();
  let currentNavType = '';
  function Probe() {
    const [p] = useSearchParams();
    const navType = useNavigationType();
    currentParams = p;
    currentNavType = navType;
    return null;
  }
  const utils = render(
    <MemoryRouter initialEntries={[initialUrl]}>
      <WorksListPage />
      <Probe />
    </MemoryRouter>,
  );
  return { ...utils, getParams: () => currentParams, getNavType: () => currentNavType };
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
  // ページネーションを確認するためダミーアイテムを含む応答(PAGINATED_RESPONSE)を使う。
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

describe('お気に入りフィルタ (issue #72)', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    vi.mocked(fetchWorks).mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('「★ お気に入りのみ」トグルを押すと URL に favorite=1 が付き、fetchWorks が favorite: true で呼ばれる', async () => {
    renderPage();

    await waitFor(() => {
      expect(vi.mocked(fetchWorks)).toHaveBeenCalled();
    });

    const toggle = screen.getByRole('button', { name: /お気に入りのみ/ });
    fireEvent.click(toggle);

    await waitFor(() => {
      expect(vi.mocked(fetchWorks)).toHaveBeenLastCalledWith(
        expect.objectContaining({ favorite: true }),
        expect.anything(),
      );
    });
  });

  it('URL ?favorite=1 で開くとトグルが有効状態で表示され、fetchWorks が favorite: true で呼ばれる', async () => {
    renderPage('/?favorite=1');

    await waitFor(() => {
      expect(vi.mocked(fetchWorks)).toHaveBeenLastCalledWith(
        expect.objectContaining({ favorite: true }),
        expect.anything(),
      );
    });

    const toggle = screen.getByRole('button', { name: /お気に入りのみ/ });
    expect(toggle).toHaveAttribute('aria-pressed', 'true');
  });

  it('有効状態でもう一度押すと favorite パラメータが消える', async () => {
    const { getParams } = renderPage('/?favorite=1');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /お気に入りのみ/ })).toHaveAttribute(
        'aria-pressed',
        'true',
      );
    });

    fireEvent.click(screen.getByRole('button', { name: /お気に入りのみ/ }));

    await waitFor(() => {
      expect(getParams().has('favorite')).toBe(false);
    });
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

describe('ソート昇順/降順トグル (#59)', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    vi.mocked(fetchWorks).mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('初期状態(デフォルト desc)ではトグルに「降順 ↓」が表示され、order パラメータは URL に付かない', async () => {
    const { getParams } = renderPage();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: '昇順に切り替え' })).toHaveTextContent('降順 ↓');
    });
    expect(getParams().has('order')).toBe(false);
  });

  it('トグルを押すと order=asc が URL に付き、fetchWorks が order: "asc" で呼ばれる', async () => {
    const { getParams } = renderPage();
    await waitFor(() => expect(fetchWorks).toHaveBeenCalled());

    fireEvent.click(screen.getByRole('button', { name: '昇順に切り替え' }));

    await waitFor(() => {
      expect(getParams().get('order')).toBe('asc');
    });
    await waitFor(() => {
      expect(vi.mocked(fetchWorks)).toHaveBeenLastCalledWith(
        expect.objectContaining({ order: 'asc' }),
        expect.anything(),
      );
    });
  });

  it('昇順状態でもう一度押すと order パラメータが URL から消える(デフォルト desc は付与しない)', async () => {
    const { getParams } = renderPage('/?order=asc');
    await waitFor(() => {
      expect(screen.getByRole('button', { name: '降順に切り替え' })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: '降順に切り替え' }));

    await waitFor(() => {
      expect(getParams().has('order')).toBe(false);
    });
  });

  it('順序トグルは page パラメータをリセットする', async () => {
    const { getParams } = renderPage('/?page=3');
    await waitFor(() => expect(fetchWorks).toHaveBeenCalled());

    fireEvent.click(screen.getByRole('button', { name: '昇順に切り替え' }));

    await waitFor(() => {
      expect(getParams().has('page')).toBe(false);
    });
  });

  it('順序トグルをクリックすると window.scrollTo(0, 0) が呼ばれる', async () => {
    renderPage();
    await waitFor(() => expect(fetchWorks).toHaveBeenCalled());

    fireEvent.click(screen.getByRole('button', { name: '昇順に切り替え' }));

    expect(window.scrollTo).toHaveBeenCalledWith(0, 0);
  });
});

describe('検索キーワードの include/exclude チップ (#29)', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    vi.mocked(fetchWorks).mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('q=foo -bar で開くと include チップ "foo" と exclude チップ "−bar" が表示される', async () => {
    renderPage('/?q=foo%20-bar');

    await waitFor(() => {
      const chips = screen.getAllByTitle('クリックで解除');
      expect(chips.some((c) => c.textContent?.includes('foo'))).toBe(true);
      expect(chips.some((c) => c.textContent?.includes('−bar'))).toBe(true);
    });
  });

  it('include チップの ✕ をクリックすると該当語だけ q から除去され、他の語は残る', async () => {
    const { getParams } = renderPage('/?q=foo%20-bar');

    await waitFor(() => {
      expect(screen.getAllByTitle('クリックで解除').some((c) => c.textContent?.includes('foo'))).toBe(
        true,
      );
    });

    const fooChip = screen
      .getAllByTitle('クリックで解除')
      .find((c) => c.textContent?.includes('foo') && !c.textContent?.includes('−bar'))!;
    fireEvent.click(fooChip);

    await waitFor(() => {
      expect(getParams().get('q')).toBe('-bar');
    });
  });

  it('exclude チップの ✕ をクリックすると該当語だけ q から除去され、他の語は残る', async () => {
    const { getParams } = renderPage('/?q=foo%20-bar');

    await waitFor(() => {
      expect(screen.getAllByTitle('クリックで解除').some((c) => c.textContent?.includes('−bar'))).toBe(
        true,
      );
    });

    const barChip = screen
      .getAllByTitle('クリックで解除')
      .find((c) => c.textContent?.includes('−bar'))!;
    fireEvent.click(barChip);

    await waitFor(() => {
      expect(getParams().get('q')).toBe('foo');
    });
  });

  it('q が単一の include 語のみの場合、チップ操作で q が空になると q パラメータごと消える', async () => {
    const { getParams } = renderPage('/?q=foo');

    await waitFor(() => {
      expect(screen.getAllByTitle('クリックで解除').some((c) => c.textContent?.includes('foo'))).toBe(
        true,
      );
    });

    const fooChip = screen
      .getAllByTitle('クリックで解除')
      .find((c) => c.textContent?.includes('foo'))!;
    fireEvent.click(fooChip);

    await waitFor(() => {
      expect(getParams().has('q')).toBe(false);
    });
  });
});

describe('デバウンス由来の history 汚染防止 (#58)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    vi.mocked(fetchWorks).mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('検索ボックスへの入力によるデバウンス反映は replace ナビゲーションになる(history を積まない)', async () => {
    const { getNavType } = renderPage();
    // 初回データ取得(Promise の resolve)を待つ
    await act(async () => {
      await Promise.resolve();
    });

    const input = screen.getByRole('searchbox');
    fireEvent.change(input, { target: { value: 'foo' } });

    // 300ms のデバウンスタイマーを進める
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });

    expect(getNavType()).toBe('REPLACE');
  });

  it('Enter による明示的な検索確定は push のままである(デバウンス反映とは異なる)', async () => {
    const { getNavType } = renderPage();
    await act(async () => {
      await Promise.resolve();
    });

    const input = screen.getByRole('searchbox');
    fireEvent.change(input, { target: { value: 'foo' } });
    // デバウンスが先に発火しないよう、確定前にタイマーは進めない

    const form = input.closest('form')!;
    fireEvent.submit(form);

    await act(async () => {
      await Promise.resolve();
    });

    expect(getNavType()).toBe('PUSH');
  });
});

describe('fetch 失敗時の再試行導線 (issue #70)', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    // 呼び出し回数のアサーションのため、前のテストの履歴をクリアする
    vi.mocked(fetchWorks).mockClear();
  });

  afterEach(() => {
    cleanup();
    vi.mocked(fetchWorks).mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 });
    vi.restoreAllMocks();
  });

  it('fetch 失敗でエラーメッセージと再試行ボタンが表示される', async () => {
    vi.mocked(fetchWorks).mockRejectedValueOnce(new Error('サーバエラー'));

    renderPage();

    await screen.findByText('サーバエラー');
    expect(screen.getByRole('button', { name: '再試行' })).toBeInTheDocument();
  });

  it('再試行クリックで再 fetch され、成功すると一覧表示に戻る', async () => {
    vi.mocked(fetchWorks)
      .mockRejectedValueOnce(new Error('サーバエラー'))
      .mockResolvedValueOnce(PAGINATED_RESPONSE);

    renderPage();

    await screen.findByText('サーバエラー');
    fireEvent.click(screen.getByRole('button', { name: '再試行' }));

    await waitFor(() => {
      expect(screen.getByText('100 件')).toBeInTheDocument();
    });
    expect(vi.mocked(fetchWorks)).toHaveBeenCalledTimes(2);
    expect(screen.queryByText('サーバエラー')).toBeNull();
  });
});

describe('ページャーの数値入力化 (#35)', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    vi.mocked(fetchWorks).mockResolvedValue(PAGINATED_RESPONSE);
  });

  afterEach(() => {
    cleanup();
    vi.mocked(fetchWorks).mockResolvedValue({ items: [], total: 0, limit: 40, page: 1 });
    vi.restoreAllMocks();
  });

  it('select ではなく数値入力欄が表示され、現在ページと総ページ数が「n / N ページ」の形で見える', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByRole('spinbutton', { name: 'ページ番号' })).toHaveValue(1);
    });
    expect(screen.queryByRole('combobox', { name: 'ページを選択' })).not.toBeInTheDocument();
    expect(screen.getByText(/\/ 3 ページ/)).toBeInTheDocument();
  });

  it('1 ページ目では「≪ 最初へ」「前へ」が disabled になる', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: '≪ 最初へ' })).toBeDisabled();
    });
    expect(screen.getByRole('button', { name: '前へ' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '次へ' })).not.toBeDisabled();
    expect(screen.getByRole('button', { name: '最後へ ≫' })).not.toBeDisabled();
  });

  it('数値入力欄に総ページ数を超える値を入れて Enter すると総ページ数にクランプされる', async () => {
    const { getParams } = renderPage();

    await waitFor(() => {
      expect(screen.getByRole('spinbutton', { name: 'ページ番号' })).toBeInTheDocument();
    });

    const input = screen.getByRole('spinbutton', { name: 'ページ番号' });
    fireEvent.change(input, { target: { value: '99' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    await waitFor(() => {
      expect(getParams().get('page')).toBe('3');
    });
    expect(input).toHaveValue(3);
  });

  it('数値入力欄で 0 以下を入れて blur すると 1 にクランプされる', async () => {
    renderPage('/?page=2');

    await waitFor(() => {
      expect(screen.getByRole('spinbutton', { name: 'ページ番号' })).toHaveValue(2);
    });

    const input = screen.getByRole('spinbutton', { name: 'ページ番号' });
    fireEvent.change(input, { target: { value: '0' } });
    fireEvent.blur(input);

    await waitFor(() => {
      expect(input).toHaveValue(1);
    });
  });

  it('「最後へ ≫」をクリックすると最終ページに移動する', async () => {
    const { getParams } = renderPage();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: '最後へ ≫' })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: '最後へ ≫' }));

    await waitFor(() => {
      expect(getParams().get('page')).toBe('3');
    });
  });

  it('最終ページでは「次へ」「最後へ ≫」が disabled になる', async () => {
    renderPage('/?page=3');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: '次へ' })).toBeDisabled();
    });
    expect(screen.getByRole('button', { name: '最後へ ≫' })).toBeDisabled();
  });
});
