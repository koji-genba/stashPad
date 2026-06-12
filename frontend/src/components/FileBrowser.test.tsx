// FileBrowser のテスト。
// audio 行の「⋮」ボタンとボトムシート(QueueActionSheet)のキュー操作を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { usePlayerStore } from '@/store/playerStore';

// API クライアントをモック化
vi.mock('@/api/client', () => ({
  recordPlay: vi.fn().mockResolvedValue(undefined),
  fileUrl: (workId: number, path: string) => `/api/works/${workId}/file?path=${encodeURIComponent(path)}`,
  thumbnailUrl: (workId: number) => `/api/works/${workId}/thumbnail`,
  fetchEntries: vi.fn(),
}));

import { fetchEntries } from '@/api/client';
import FileBrowser from './FileBrowser';

// playerStore の初期状態(QueueScreen.test.tsx の initialState に準拠)
const initialState = {
  queue: [],
  index: -1,
  isPlaying: false,
  currentTime: 0,
  duration: 0,
  playbackRate: 1,
  seekRequest: null,
  loadNonce: 0,
  volume: 1,
  nextUid: 1,
};

function resetStore() {
  usePlayerStore.setState(initialState, false);
}

// テスト用エントリフィクスチャ
const mockEntries = {
  path: '',
  parent: '',
  entries: [
    { name: 'subdir', is_dir: true, size: 0, media_kind: '' as const },
    { name: 'track01.mp3', is_dir: false, size: 1024000, media_kind: 'audio' as const },
    { name: 'track02.mp3', is_dir: false, size: 2048000, media_kind: 'audio' as const },
    { name: 'cover.jpg', is_dir: false, size: 51200, media_kind: 'image' as const },
  ],
};

// サブディレクトリ用フィクスチャ
const mockSubEntries = {
  path: 'subdir',
  parent: '',
  entries: [
    { name: 'bonus.mp3', is_dir: false, size: 512000, media_kind: 'audio' as const },
  ],
};

/** FileBrowser をレンダリングして fetchEntries の解決を待つ */
async function renderAndWait(workId = 1, workTitle = 'テスト作品') {
  const result = render(<FileBrowser workId={workId} workTitle={workTitle} />);
  // ローディング完了を待つ
  await screen.findByText('track01.mp3');
  return result;
}

beforeEach(() => {
  resetStore();
  vi.mocked(fetchEntries).mockResolvedValue(mockEntries);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('FileBrowser ⋮ ボタンの表示条件', () => {
  it('audio 行にのみ ⋮ ボタンが表示される', async () => {
    await renderAndWait();
    // audio 行(track01, track02)には ⋮ ボタンがある
    expect(screen.getByRole('button', { name: 'track01.mp3 のキュー操作' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'track02.mp3 のキュー操作' })).toBeInTheDocument();
  });

  it('ディレクトリ行には ⋮ ボタンが表示されない', async () => {
    await renderAndWait();
    expect(screen.queryByRole('button', { name: 'subdir のキュー操作' })).toBeNull();
  });

  it('image 行には ⋮ ボタンが表示されない', async () => {
    await renderAndWait();
    expect(screen.queryByRole('button', { name: 'cover.jpg のキュー操作' })).toBeNull();
  });
});

describe('FileBrowser ボトムシートの開閉', () => {
  it('⋮ クリックでシートが開き、ファイル名と 3 アクション・キャンセルが表示される', async () => {
    await renderAndWait();
    fireEvent.click(screen.getByRole('button', { name: 'track01.mp3 のキュー操作' }));

    // パネルの role="dialog" と aria-label
    expect(screen.getByRole('dialog', { name: 'track01.mp3 のキュー操作' })).toBeInTheDocument();
    // ファイル名テキスト(パネル先頭の p 要素)
    // ※ aria-label と同名テキストが複数あるので getAllBy を使う
    const fileNames = screen.getAllByText('track01.mp3');
    expect(fileNames.length).toBeGreaterThanOrEqual(1);
    // 3 アクション
    expect(screen.getByRole('button', { name: '今の曲が終わったら再生' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'キューを置き換えて再生' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'キューの最後に追加' })).toBeInTheDocument();
    // キャンセル
    expect(screen.getByRole('button', { name: 'キャンセル' })).toBeInTheDocument();
  });

  it('キャンセルボタンでシートが閉じる', async () => {
    await renderAndWait();
    fireEvent.click(screen.getByRole('button', { name: 'track01.mp3 のキュー操作' }));
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'キャンセル' }));
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('バックドロップクリックでシートが閉じる', async () => {
    await renderAndWait();
    fireEvent.click(screen.getByRole('button', { name: 'track01.mp3 のキュー操作' }));
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    // バックドロップは aria-hidden なので querySelector で取得
    const backdrop = document.querySelector('[aria-hidden="true"]') as HTMLElement;
    expect(backdrop).not.toBeNull();
    fireEvent.click(backdrop);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('Escape キーでシートが閉じる', async () => {
    await renderAndWait();
    fireEvent.click(screen.getByRole('button', { name: 'track01.mp3 のキュー操作' }));
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});

describe('FileBrowser キュー操作の store 検証', () => {
  // 初期キュー: [x, y] の x が再生中
  function setupTwoTrackQueue() {
    usePlayerStore.setState({
      queue: [
        { uid: 10, workId: 99, workTitle: '他の作品', name: 'x.mp3', path: 'x.mp3' },
        { uid: 11, workId: 99, workTitle: '他の作品', name: 'y.mp3', path: 'y.mp3' },
      ],
      index: 0,
      isPlaying: true,
      loadNonce: 5,
      nextUid: 100,
    });
  }

  it('「今の曲が終わったら再生」: 現在の直後に挿入され track の各フィールドが正しい', async () => {
    setupTwoTrackQueue();
    await renderAndWait(1, 'テスト作品');

    fireEvent.click(screen.getByRole('button', { name: 'track01.mp3 のキュー操作' }));
    fireEvent.click(screen.getByRole('button', { name: '今の曲が終わったら再生' }));

    const { queue, index } = usePlayerStore.getState();
    // 3 トラックになる
    expect(queue).toHaveLength(3);
    // 元の x が index=0 のまま
    expect(index).toBe(0);
    expect(queue[0].name).toBe('x.mp3');
    // index=1 に挿入された新トラック
    const inserted = queue[1];
    expect(inserted.workId).toBe(1);
    expect(inserted.workTitle).toBe('テスト作品');
    expect(inserted.path).toBe('track01.mp3'); // dir='' のときは name そのまま
    expect(inserted.name).toBe('track01.mp3');
    // y は末尾に押し出される
    expect(queue[2].name).toBe('y.mp3');
    // シートが閉じる
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('「キューを置き換えて再生」: queue が 1 曲になり isPlaying=true', async () => {
    setupTwoTrackQueue();
    await renderAndWait(1, 'テスト作品');

    fireEvent.click(screen.getByRole('button', { name: 'track01.mp3 のキュー操作' }));
    fireEvent.click(screen.getByRole('button', { name: 'キューを置き換えて再生' }));

    const { queue, index, isPlaying } = usePlayerStore.getState();
    expect(queue).toHaveLength(1);
    expect(index).toBe(0);
    expect(isPlaying).toBe(true);
    expect(queue[0].name).toBe('track01.mp3');
    expect(queue[0].workId).toBe(1);
    expect(queue[0].workTitle).toBe('テスト作品');
    expect(queue[0].path).toBe('track01.mp3');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('「キューの最後に追加」: 末尾に追加される', async () => {
    setupTwoTrackQueue();
    await renderAndWait(1, 'テスト作品');

    fireEvent.click(screen.getByRole('button', { name: 'track01.mp3 のキュー操作' }));
    fireEvent.click(screen.getByRole('button', { name: 'キューの最後に追加' }));

    const { queue, index } = usePlayerStore.getState();
    expect(queue).toHaveLength(3);
    // 再生中は変わらず x
    expect(index).toBe(0);
    expect(queue[0].name).toBe('x.mp3');
    expect(queue[1].name).toBe('y.mp3');
    // 末尾に追加
    const appended = queue[2];
    expect(appended.name).toBe('track01.mp3');
    expect(appended.workId).toBe(1);
    expect(appended.path).toBe('track01.mp3');
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});

describe('FileBrowser サブディレクトリ閲覧中の path join', () => {
  it('サブディレクトリ内の audio 行の path は dir/名前 になる', async () => {
    // ルートのエントリをまず返し、次にサブディレクトリのエントリを返す
    vi.mocked(fetchEntries)
      .mockResolvedValueOnce(mockEntries)
      .mockResolvedValueOnce(mockSubEntries);

    render(<FileBrowser workId={1} workTitle='テスト作品' />);

    // ルート読み込み完了を待つ
    await screen.findByText('track01.mp3');

    // ディレクトリ行("subdir")をクリックしてサブディレクトリへ遷移
    // ボタンのアクセシブル名はアイコン込みになるため、テキスト要素から辿る
    fireEvent.click(screen.getByText('subdir').closest('button')!);

    // サブディレクトリの読み込み完了を待つ
    await screen.findByText('bonus.mp3');

    // ⋮ ボタンをクリック
    fireEvent.click(screen.getByRole('button', { name: 'bonus.mp3 のキュー操作' }));
    fireEvent.click(screen.getByRole('button', { name: 'キューを置き換えて再生' }));

    const { queue } = usePlayerStore.getState();
    expect(queue).toHaveLength(1);
    // path は subdir/bonus.mp3 に join される
    expect(queue[0].path).toBe('subdir/bonus.mp3');
    expect(queue[0].name).toBe('bonus.mp3');
  });
});

describe('FileBrowser 行本体の挙動が維持される', () => {
  it('audio 行の行本体クリックは startFromEntries を呼ぶ(openEntry 維持)', async () => {
    await renderAndWait(1, 'テスト作品');

    // track01.mp3 の行本体ボタン(aria-label ではなくテキストで判別)
    fireEvent.click(screen.getByText('track01.mp3').closest('button')!);

    const { queue, isPlaying } = usePlayerStore.getState();
    // audio のみのキューが作られる
    expect(queue.length).toBe(2); // track01, track02
    expect(queue[0].name).toBe('track01.mp3');
    expect(isPlaying).toBe(true);
  });

  it('ディレクトリ行クリックはサブディレクトリへ遷移する', async () => {
    vi.mocked(fetchEntries)
      .mockResolvedValueOnce(mockEntries)
      .mockResolvedValueOnce(mockSubEntries);

    render(<FileBrowser workId={1} workTitle='テスト作品' />);
    await screen.findByText('track01.mp3');

    // ボタンのアクセシブル名はアイコン込みになるため、テキスト要素から辿る
    fireEvent.click(screen.getByText('subdir').closest('button')!);

    await screen.findByText('bonus.mp3');
    expect(screen.queryByText('track01.mp3')).toBeNull();
  });
});
