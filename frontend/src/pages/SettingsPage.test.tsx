// SettingsPage のテスト。
// メンテナンス節の「再生履歴を全削除」ボタン(confirm → API 呼び出し → 結果表示)を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import type { ThumbnailRebuildStatus } from '@/api/types';

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
  importMetadata: vi.fn(),
  rebuildThumbnails: vi.fn(),
  runScan: vi.fn(),
  setWorkHidden: vi.fn(),
  deleteHistory: vi.fn().mockResolvedValue({ deleted: 5 }),
}));

const { tagStoreRefreshMock } = vi.hoisted(() => ({ tagStoreRefreshMock: vi.fn() }));
vi.mock('@/store/tagStore', () => ({
  useTagStore: {
    getState: () => ({ refresh: tagStoreRefreshMock }),
  },
}));

import SettingsPage from './SettingsPage';
import {
  deleteHistory,
  fetchThumbnailRebuildStatus,
  fetchWorks,
  importCsv,
  importMetadata,
  rebuildThumbnails,
} from '@/api/client';

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

describe('SettingsPage CSV インポート', () => {
  beforeEach(() => {
    vi.mocked(importCsv).mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('インポート結果にスキップ件数を表示する', async () => {
    vi.mocked(importCsv).mockResolvedValue({
      created: 0,
      updated: 2,
      linked: 1,
      skipped: 3,
      errors: [],
    });
    const { container } = renderPage();

    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['rj_number,title\nRJ1,Title\n'], 'works.csv', {
      type: 'text/csv',
    });
    fireEvent.change(input, { target: { files: [file] } });
    fireEvent.click(screen.getByRole('button', { name: 'インポート実行' }));

    await waitFor(() => expect(importCsv).toHaveBeenCalledWith(file));
    await screen.findByText((_, node) => node?.textContent === 'スキップ 3');
  });
});

// メタデータのエクスポート/インポート(issue #78)。
// エクスポートは fetch 不要のダウンロードリンク、インポートは JSON ファイルを
// 選択して POST する既存 CSV インポート UI の流儀を踏襲する。
describe('SettingsPage メタデータのエクスポート/インポート', () => {
  beforeEach(() => {
    vi.mocked(importMetadata).mockReset();
    tagStoreRefreshMock.mockClear();
    vi.mocked(fetchWorks).mockClear();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('エクスポートは /api/export への download リンクである', () => {
    const { container } = renderPage();
    const link = container.querySelector('a[download]') as HTMLAnchorElement | null;
    expect(link).not.toBeNull();
    expect(link?.getAttribute('href')).toBe('/api/export');
  });

  it('インポート成功時に一致・見つからず・タグ付与件数を表示する', async () => {
    vi.mocked(importMetadata).mockResolvedValue({
      matched: 4,
      skipped: 1,
      tags_added: 6,
      errors: [],
    });
    const { container } = renderPage();

    const inputs = container.querySelectorAll('input[type="file"]');
    // 1つ目が CSV インポート用、2つ目がメタデータインポート用
    const input = inputs[1] as HTMLInputElement;
    const file = new File(['{"version":1,"works":[]}'], 'stashpad-metadata-20260705.json', {
      type: 'application/json',
    });
    fireEvent.change(input, { target: { files: [file] } });
    fireEvent.click(screen.getByRole('button', { name: 'メタデータをインポート' }));

    await waitFor(() => expect(importMetadata).toHaveBeenCalledWith(file));
    await screen.findByText((_, node) => node?.textContent === '一致 4');
    await screen.findByText((_, node) => node?.textContent === '見つからず 1');
    await screen.findByText((_, node) => node?.textContent === 'タグ付与 6');
  });

  it('インポート成功後に tagStore.refresh と非表示作品一覧の再取得が呼ばれる', async () => {
    vi.mocked(importMetadata).mockResolvedValue({
      matched: 4,
      skipped: 1,
      tags_added: 6,
      errors: [],
    });
    const { container } = renderPage();

    // マウント時の非表示作品一覧の初回取得分をクリアしてからカウントする
    await waitFor(() => expect(fetchWorks).toHaveBeenCalled());
    vi.mocked(fetchWorks).mockClear();

    const inputs = container.querySelectorAll('input[type="file"]');
    const input = inputs[1] as HTMLInputElement;
    const file = new File(['{"version":1,"works":[]}'], 'stashpad-metadata-20260705.json', {
      type: 'application/json',
    });
    fireEvent.change(input, { target: { files: [file] } });
    fireEvent.click(screen.getByRole('button', { name: 'メタデータをインポート' }));

    await waitFor(() => expect(importMetadata).toHaveBeenCalledWith(file));
    await waitFor(() => expect(tagStoreRefreshMock).toHaveBeenCalled());
    await waitFor(() =>
      expect(fetchWorks).toHaveBeenCalledWith({ hidden: true, limit: 200 }),
    );
  });

  it('インポート失敗時にエラーメッセージを表示する', async () => {
    vi.mocked(importMetadata).mockRejectedValue(new Error('version が不正です'));
    const { container } = renderPage();

    const inputs = container.querySelectorAll('input[type="file"]');
    const input = inputs[1] as HTMLInputElement;
    const file = new File(['{"version":2,"works":[]}'], 'bad.json', {
      type: 'application/json',
    });
    fireEvent.change(input, { target: { files: [file] } });
    fireEvent.click(screen.getByRole('button', { name: 'メタデータをインポート' }));

    await screen.findByText('version が不正です');
  });

  it('インポート結果に行単位のエラー一覧を表示する', async () => {
    vi.mocked(importMetadata).mockResolvedValue({
      matched: 1,
      skipped: 0,
      tags_added: 0,
      errors: ['RJ999999: タグ作成失敗'],
    });
    const { container } = renderPage();

    const inputs = container.querySelectorAll('input[type="file"]');
    const input = inputs[1] as HTMLInputElement;
    const file = new File(['{"version":1,"works":[]}'], 'meta.json', {
      type: 'application/json',
    });
    fireEvent.change(input, { target: { files: [file] } });
    fireEvent.click(screen.getByRole('button', { name: 'メタデータをインポート' }));

    await screen.findByText('RJ999999: タグ作成失敗');
  });
});

// サムネイル再生成ポーリングの in-flight fetch(PR #79 レビュー)。
// - unmount 時に、進行中のポーリング fetch を AbortController で中断する
// - 前回の fetch が完了していない間は次の 1 秒 tick で新たな fetch を発行しない(in-flight ガード)
// setInterval を含むため vi.useFakeTimers + advanceTimersByTimeAsync で時間を制御する
// (実時間 sleep には依存しない)。
describe('SettingsPage サムネイル再生成ポーリングの in-flight fetch', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  // タイマー進行に伴う setState を act() でラップし、テストの警告を抑える
  async function advance(ms: number) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms);
    });
  }

  it('unmount すると進行中のポーリング fetch が abort される', async () => {
    vi.mocked(rebuildThumbnails).mockResolvedValue({
      running: true,
      checked: 0,
      regenerated: 0,
      total: 5,
    });
    let capturedSignal: AbortSignal | undefined;
    vi.mocked(fetchThumbnailRebuildStatus).mockImplementation((signal?: AbortSignal) => {
      if (signal) capturedSignal = signal;
      return new Promise(() => {
        /* 意図的に完了しない Promise でポーリング中を模す */
      });
    });

    const { unmount } = renderPage();
    // マウント時復帰チェック(実行中ジョブなし)の microtask を流す
    await advance(0);

    const btn = screen.getByRole('button', { name: 'サムネイル再生成' });
    fireEvent.click(btn);
    await advance(0); // rebuildThumbnails の resolve → ポーリング開始

    await advance(1000); // 最初の tick
    expect(capturedSignal).toBeDefined();
    expect(capturedSignal!.aborted).toBe(false);

    unmount();
    expect(capturedSignal!.aborted).toBe(true);
  });

  it('前回のポーリング fetch が完了しないうちは次の tick で新たな fetch を発行しない', async () => {
    vi.mocked(rebuildThumbnails).mockResolvedValue({
      running: true,
      checked: 0,
      regenerated: 0,
      total: 5,
    });

    let pollCalls = 0;
    let resolveFirst: ((v: ThumbnailRebuildStatus) => void) | null = null;
    vi.mocked(fetchThumbnailRebuildStatus).mockImplementation((signal?: AbortSignal) => {
      if (!signal) {
        // マウント時の復帰チェック(実行中ジョブなし)
        return Promise.resolve({ running: false, checked: 0, regenerated: 0, total: 0 });
      }
      pollCalls++;
      if (pollCalls === 1) {
        return new Promise<ThumbnailRebuildStatus>((resolve) => {
          resolveFirst = resolve;
        });
      }
      return Promise.resolve({ running: false, checked: 5, regenerated: 5, total: 5 });
    });

    renderPage();
    await advance(0);

    const btn = screen.getByRole('button', { name: 'サムネイル再生成' });
    fireEvent.click(btn);
    await advance(0);

    await advance(1000); // 1 回目の tick
    expect(pollCalls).toBe(1);

    await advance(1000); // 2 回目の tick。1 回目が未完了なのでガードされる
    expect(pollCalls).toBe(1);

    resolveFirst!({ running: true, checked: 1, regenerated: 0, total: 5 });
    await advance(0); // in-flight フラグ解除
    await advance(1000); // 次の tick で 2 回目が発行される
    expect(pollCalls).toBe(2);
  });
});
