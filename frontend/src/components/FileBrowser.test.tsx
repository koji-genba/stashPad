// FileBrowser のテスト。
// audio 行の「⋮」ボタンとボトムシート(QueueActionSheet)のキュー操作を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { usePlayerStore } from '@/store/playerStore';
import { useOverlayStore } from '@/store/overlayStore';

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
  // overlayStore も毎テスト初期化(video/text を null へ戻す)
  useOverlayStore.setState({ image: null, video: null, text: null }, false);
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

describe('FileBrowser 再生中ファイルのインジケータ (issue #31)', () => {
  // 「再生中」の判定は (workId, path) の組で行う。
  // path は FileBrowser 上で joinPath(currentDir, entry.name) と組み立てた値と一致させる。

  it('再生中の audio エントリには aria-current="true" が付く', async () => {
    usePlayerStore.setState({
      queue: [
        { uid: 1, workId: 1, workTitle: 'テスト作品', name: 'track01.mp3', path: 'track01.mp3' },
      ],
      index: 0,
      isPlaying: true,
    });
    await renderAndWait(1, 'テスト作品');

    const btn = screen.getByText('track01.mp3').closest('button')!;
    expect(btn).toHaveAttribute('aria-current', 'true');
  });

  it('再生中でない audio エントリには aria-current が付かない', async () => {
    usePlayerStore.setState({
      queue: [
        { uid: 1, workId: 1, workTitle: 'テスト作品', name: 'track01.mp3', path: 'track01.mp3' },
      ],
      index: 0,
      isPlaying: true,
    });
    await renderAndWait(1, 'テスト作品');

    const btn = screen.getByText('track02.mp3').closest('button')!;
    expect(btn).not.toHaveAttribute('aria-current');
  });

  it('別作品の再生中トラックは強調されない (workId 一致が必須)', async () => {
    usePlayerStore.setState({
      queue: [
        // path は同じだが workId が違う
        { uid: 1, workId: 999, workTitle: '別作品', name: 'track01.mp3', path: 'track01.mp3' },
      ],
      index: 0,
      isPlaying: true,
    });
    await renderAndWait(1, 'テスト作品');

    const btn = screen.getByText('track01.mp3').closest('button')!;
    expect(btn).not.toHaveAttribute('aria-current');
  });

  it('再生中の video オーバーレイ表示中はその video 行が aria-current="true"', async () => {
    vi.mocked(fetchEntries).mockResolvedValue({
      path: '',
      parent: '',
      entries: [
        { name: 'movie.mp4', is_dir: false, size: 1024000, media_kind: 'video' as const },
        { name: 'other.mp4', is_dir: false, size: 2048000, media_kind: 'video' as const },
      ],
    });
    useOverlayStore.setState({
      video: { workId: 1, workTitle: 'テスト作品', path: 'movie.mp4', name: 'movie.mp4' },
    });

    render(<FileBrowser workId={1} workTitle='テスト作品' />);
    await screen.findByText('movie.mp4');

    expect(screen.getByText('movie.mp4').closest('button')!).toHaveAttribute('aria-current', 'true');
    expect(screen.getByText('other.mp4').closest('button')!).not.toHaveAttribute('aria-current');
  });

  it('text オーバーレイ表示中はその text 行が aria-current="true"', async () => {
    vi.mocked(fetchEntries).mockResolvedValue({
      path: '',
      parent: '',
      entries: [
        { name: 'readme.txt', is_dir: false, size: 1024, media_kind: 'text' as const },
      ],
    });
    useOverlayStore.setState({
      text: { workId: 1, path: 'readme.txt', name: 'readme.txt' },
    });

    render(<FileBrowser workId={1} workTitle='テスト作品' />);
    await screen.findByText('readme.txt');

    expect(screen.getByText('readme.txt').closest('button')!).toHaveAttribute('aria-current', 'true');
  });

  it('image / other / ディレクトリ は再生中強調の対象外', async () => {
    // image overlay は本 issue の対象外(ページ列で構造が異なる)
    // 一方で同名・同 workId の audio がキュー再生中であっても image 行は強調されない
    usePlayerStore.setState({
      queue: [
        { uid: 1, workId: 1, workTitle: 'テスト作品', name: 'cover.jpg', path: 'cover.jpg' },
      ],
      index: 0,
      isPlaying: true,
    });
    await renderAndWait(1, 'テスト作品');

    expect(screen.getByText('cover.jpg').closest('button')!).not.toHaveAttribute('aria-current');
    expect(screen.getByText('subdir').closest('button')!).not.toHaveAttribute('aria-current');
  });

  it('サブディレクトリ内でも joined path が一致すれば強調される', async () => {
    vi.mocked(fetchEntries)
      .mockResolvedValueOnce(mockEntries)
      .mockResolvedValueOnce(mockSubEntries);
    usePlayerStore.setState({
      queue: [
        { uid: 1, workId: 1, workTitle: 'テスト作品', name: 'bonus.mp3', path: 'subdir/bonus.mp3' },
      ],
      index: 0,
      isPlaying: true,
    });

    render(<FileBrowser workId={1} workTitle='テスト作品' />);
    await screen.findByText('track01.mp3');
    fireEvent.click(screen.getByText('subdir').closest('button')!);
    await screen.findByText('bonus.mp3');

    expect(screen.getByText('bonus.mp3').closest('button')!).toHaveAttribute('aria-current', 'true');
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
