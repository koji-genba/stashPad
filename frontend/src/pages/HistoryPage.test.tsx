// HistoryPage のテスト。
// 履歴行の削除ボタン(confirm → API 呼び出し → 一覧からの除去)を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

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
    fetchHistory: vi.fn().mockResolvedValue({ items: historyItems, total: 2, page: 1, limit: 40 }),
    deleteHistory: vi.fn().mockResolvedValue({ deleted: 1 }),
  };
});

import HistoryPage from './HistoryPage';
import { deleteHistory, fetchHistory } from '@/api/client';

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

  it('1 件削除に成功すると総件数表示もローカルでデクリメントされる(issue #60)', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    renderPage();

    await screen.findByText('猫の物語');
    expect(screen.getByText('2 件')).toBeTruthy();

    const deleteButtons = screen.getAllByRole('button', { name: 'この作品の履歴を削除' });
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => expect(screen.queryByText('猫の物語')).toBeNull());
    expect(screen.getByText('1 件')).toBeTruthy();
  });
});

// 非先頭ページで表示中の全行を削除すると行き止まりになる問題の回帰テスト(PR #79 レビュー)。
// 履歴 41 件・limit 40 で 2 ページ目(1 件)を削除すると items が空になり、
// ページャ自体が非空分岐の内側にあるため消えてしまい、total>0 なのに
// 「まだ再生履歴がありません」から復帰できなくなっていた。
// 修正: 削除後に残 items が 0 かつ page>1 なら前ページへ戻し、再フェッチさせる。
//
// このテストは以前、削除直後の再フェッチを検索デバウンス由来の再フェッチと区別するため
// waitFor の timeout を 150ms に絞っていた。しかし検索欄の 300ms デバウンス effect
// (useEffect(() => { setTimeout(() => { setQ(qInput); setPage(1); }, 300) }, [qInput]))
// は「qInput が現在の q と同じなら発火しない」ガードを持たず、マウント時に一度スケジュールされた
// タイマーは(qInput を一切変更しない本テストでも)実時間で 300ms 経過すると必ず発火し、
// page を 1 に戻してしまう。CI の遅いランナーでは「次へ」クリックから削除完了までの実時間が
// 300ms を超えることがあり、意図した削除経由の page=1 遷移より先にこの無関係なデバウンス
// タイマーが発火して 1 ページ目に戻ってしまい、999 ではなく作品 1 の削除ボタンを叩く
// 結果になって deleteHistory(999) の呼び出しが観測できず落ちていた(実時間の競合)。
// 修正方針: vi.useFakeTimers() でこの 300ms タイマーを明示的に止め、テスト内では一切
// advance しないことで「検索デバウンスによる偶然の page=1 遷移」を構造的に起こり得なくする。
// これにより、観測される page=1 への遷移は削除ロジック由来であることが決定的に保証される。
describe('HistoryPage 非先頭ページの全件削除', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  // pending な Promise の解決とそれに伴う setState を act() でラップして反映させる。
  // 検索デバウンスの 300ms タイマーには一切到達しない範囲でのみ使う。
  async function advance(ms: number) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms);
    });
  }

  it('2 ページ目の最後の 1 件を削除すると 1 ページ目へ戻って再フェッチする', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    const page1Items = Array.from({ length: 40 }, (_, i) => ({
      work: { id: i + 1, title: `作品${i + 1}`, thumbnail_url: '' },
      last_played_at: '2026-01-01T00:00:00Z',
      last_file_path: `f${i}.mp3`,
      play_count: 1,
    }));
    const page2Item = {
      work: { id: 999, title: '最後の作品', thumbnail_url: '' },
      last_played_at: '2025-12-01T00:00:00Z',
      last_file_path: 'last.mp3',
      play_count: 1,
    };

    // mockImplementationOnce を使い、3 回目以降の呼び出しは(このテストでは起きないはずだが)
    // vi.mock 側のデフォルト実装にフォールバックさせて他 describe への汚染を避ける
    vi.mocked(fetchHistory)
      .mockImplementationOnce(async () => ({ items: page1Items, total: 41, page: 1, limit: 40 }))
      .mockImplementationOnce(async () => ({ items: [page2Item], total: 41, page: 2, limit: 40 }))
      .mockImplementationOnce(async () => ({ items: page1Items, total: 41, page: 1, limit: 40 }));

    renderPage();
    await advance(0);
    expect(screen.getByText('作品1')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: '次へ' }));
    await advance(0);
    expect(screen.getByText('最後の作品')).toBeTruthy();

    const deleteButtons = screen.getAllByRole('button', { name: 'この作品の履歴を削除' });
    fireEvent.click(deleteButtons[0]);
    await advance(0);

    expect(deleteHistory).toHaveBeenCalledWith(999);

    // 前ページ(page=1)へ戻って再フェッチされ、「まだ再生履歴がありません」には陥らない。
    // 検索デバウンスの 300ms タイマーには到達していないため、この page=1 遷移は
    // 削除ロジック由来であることが構造的に保証されている。
    expect(fetchHistory).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1 }),
      expect.anything(),
    );
    expect(screen.getByText('作品1')).toBeTruthy();
    expect(screen.queryByText('まだ再生履歴がありません')).toBeNull();
  });
});

// total を用いた hasMore 判定(issue #60)。
// items.length >= limit のヒューリスティックでは、作品数がちょうど limit の倍数のとき
// 最終ページでも「次へ」が有効になり空ページに遷移してしまう。
// total を使った `page * limit < total` 判定に置き換えたことを検証する。
describe('HistoryPage 総件数と次へページングの整合性', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('総件数が一覧上部に表示される', async () => {
    renderPage();
    await screen.findByText('猫の物語');
    expect(screen.getByText('2 件')).toBeTruthy();
  });

  it('total が limit×page ちょうどのとき「次へ」が disabled になる(空ページ遷移防止)', async () => {
    const page1Items = [
      {
        work: { id: 10, title: '作品10', thumbnail_url: '' },
        last_played_at: '2026-01-02T00:00:00Z',
        last_file_path: 'a.mp3',
        play_count: 1,
      },
      {
        work: { id: 11, title: '作品11', thumbnail_url: '' },
        last_played_at: '2026-01-01T00:00:00Z',
        last_file_path: 'b.mp3',
        play_count: 1,
      },
    ];
    const page2Items = [
      {
        work: { id: 12, title: '作品12', thumbnail_url: '' },
        last_played_at: '2025-12-31T00:00:00Z',
        last_file_path: 'c.mp3',
        play_count: 1,
      },
      {
        work: { id: 13, title: '作品13', thumbnail_url: '' },
        last_played_at: '2025-12-30T00:00:00Z',
        last_file_path: 'd.mp3',
        play_count: 1,
      },
    ];
    // 全 4 件・limit=2 で 2 ページ分。ちょうど limit の倍数(4 = 2 ページ × limit 2)。
    vi.mocked(fetchHistory)
      .mockResolvedValueOnce({ items: page1Items, total: 4, page: 1, limit: 2 })
      .mockResolvedValueOnce({ items: page2Items, total: 4, page: 2, limit: 2 });

    renderPage();
    await screen.findByText('作品10');

    const nextBtn = screen.getByRole('button', { name: '次へ' });
    expect(nextBtn).not.toBeDisabled();
    fireEvent.click(nextBtn);

    await screen.findByText('作品12');
    expect(screen.getByRole('button', { name: '次へ' })).toBeDisabled();
  });
});
