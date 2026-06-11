// FullscreenPlayer コンポーネントのテスト。
// playerStore に状態を仕込んで描画・操作を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { usePlayerStore } from '@/store/playerStore';

// API クライアントをモック化(playerStore.test.ts と同様)
vi.mock('@/api/client', () => ({
  recordPlay: vi.fn().mockResolvedValue(undefined),
  fileUrl: (workId: number, path: string) => `/api/works/${workId}/file?path=${encodeURIComponent(path)}`,
  thumbnailUrl: (workId: number) => `/api/works/${workId}/thumbnail`,
}));

// react-router-dom の useNavigate をモック化
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// vi.mock はファイル先頭に巻き上げられるため、この import 時点でモックは適用済み
import FullscreenPlayer from './FullscreenPlayer';

// ストアの初期状態リセット用
const initialState = {
  ctx: null,
  queue: [],
  index: -1,
  isPlaying: false,
  currentTime: 0,
  duration: 0,
  playbackRate: 1,
  seekRequest: null,
  loadNonce: 0,
  expanded: false,
  volume: 1,
};

function resetStore() {
  usePlayerStore.setState(initialState, false);
}

// テスト用の最低限の状態を設定するヘルパ
function setupPlayingState() {
  usePlayerStore.setState({
    ctx: { workId: 42, workTitle: 'テスト作品タイトル', dir: '' },
    queue: [
      { name: 'track01.mp3', path: 'track01.mp3' },
      { name: 'track02.mp3', path: 'track02.mp3' },
      { name: 'track03.mp3', path: 'track03.mp3' },
    ],
    index: 1,
    isPlaying: true,
    currentTime: 30,
    duration: 120,
    expanded: true,
  });
}

function renderPlayer() {
  return render(
    <MemoryRouter>
      <FullscreenPlayer />
    </MemoryRouter>,
  );
}

describe('FullscreenPlayer 描画条件', () => {
  beforeEach(resetStore);
  afterEach(cleanup);

  it('expanded=false のとき何も描画しない', () => {
    usePlayerStore.setState({
      ctx: { workId: 1, workTitle: 'テスト', dir: '' },
      queue: [{ name: 'a.mp3', path: 'a.mp3' }],
      index: 0,
      expanded: false,
    });
    const { container } = renderPlayer();
    expect(container.firstChild).toBeNull();
  });

  it('ctx=null のとき何も描画しない', () => {
    usePlayerStore.setState({ ctx: null, expanded: true });
    const { container } = renderPlayer();
    expect(container.firstChild).toBeNull();
  });

  it('expanded=true かつ ctx があるとき描画される', () => {
    setupPlayingState();
    renderPlayer();
    // トラック名はトラック情報エリアとキュー一覧の両方に表示されるので getAllByText を使う
    const trackNames = screen.getAllByText('track02.mp3');
    expect(trackNames.length).toBeGreaterThanOrEqual(1);
  });
});

describe('FullscreenPlayer コンテンツ', () => {
  beforeEach(() => {
    resetStore();
    setupPlayingState();
  });
  afterEach(cleanup);

  it('トラック名が表示される', () => {
    renderPlayer();
    // 現在再生中のトラック名はトラック情報エリアとキュー一覧の両方に表示される
    const trackNames = screen.getAllByText('track02.mp3');
    expect(trackNames.length).toBeGreaterThanOrEqual(1);
  });

  it('作品タイトルが表示される', () => {
    renderPlayer();
    // ヘッダの作品タイトルボタンとメタエリアの両方に表示される
    const titles = screen.getAllByText('テスト作品タイトル');
    expect(titles.length).toBeGreaterThanOrEqual(1);
  });

  it('キュー一覧の全トラックが表示される', () => {
    renderPlayer();
    // キュー一覧内のボタンで確認(aria-label で特定)
    expect(screen.getByRole('button', { name: /track01\.mp3/ })).toBeInTheDocument();
    // track02.mp3 はキュー一覧ボタンとトラック情報エリアに複数存在する
    const track02Elements = screen.getAllByText('track02.mp3');
    expect(track02Elements.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByRole('button', { name: /track03\.mp3/ })).toBeInTheDocument();
  });
});

describe('FullscreenPlayer 操作', () => {
  beforeEach(() => {
    resetStore();
    setupPlayingState();
  });
  afterEach(cleanup);

  it('閉じるボタンで expanded が false になる', () => {
    renderPlayer();
    const closeBtn = screen.getByRole('button', { name: 'ミニプレイヤーに戻る' });
    fireEvent.click(closeBtn);
    expect(usePlayerStore.getState().expanded).toBe(false);
  });

  it('作品タイトルボタンクリックで navigate と setExpanded(false) が呼ばれる', () => {
    renderPlayer();
    const titleBtn = screen.getByRole('button', { name: 'テスト作品タイトル の作品ページを開く' });
    fireEvent.click(titleBtn);
    expect(mockNavigate).toHaveBeenCalledWith('/works/42');
    expect(usePlayerStore.getState().expanded).toBe(false);
  });

  it('キュー行クリックで playIndex が呼ばれる', () => {
    renderPlayer();
    // track01.mp3 は index=0
    const track01Btn = screen.getByRole('button', { name: /track01\.mp3/ });
    fireEvent.click(track01Btn);
    expect(usePlayerStore.getState().index).toBe(0);
  });

  it('−5 ボタンで currentTime が 5 秒戻る', () => {
    renderPlayer();
    const btn = screen.getByRole('button', { name: '5秒戻る' });
    fireEvent.click(btn);
    // seekTo が呼ばれて currentTime が 25 になる
    expect(usePlayerStore.getState().currentTime).toBe(25);
  });

  it('+5 ボタンで currentTime が 5 秒進む', () => {
    renderPlayer();
    const btn = screen.getByRole('button', { name: '5秒進む' });
    fireEvent.click(btn);
    expect(usePlayerStore.getState().currentTime).toBe(35);
  });

  it('−30 ボタンで currentTime が 30 秒戻る', () => {
    renderPlayer();
    const btn = screen.getByRole('button', { name: '30秒戻る' });
    fireEvent.click(btn);
    expect(usePlayerStore.getState().currentTime).toBe(0); // クランプ: 30-30=0
  });

  it('+30 ボタンで currentTime が 30 秒進む', () => {
    renderPlayer();
    const btn = screen.getByRole('button', { name: '30秒進む' });
    fireEvent.click(btn);
    expect(usePlayerStore.getState().currentTime).toBe(60);
  });
});

describe('FullscreenPlayer キーボード', () => {
  beforeEach(() => {
    resetStore();
    setupPlayingState();
  });
  afterEach(cleanup);

  it('Escape キーで expanded が false になる', () => {
    renderPlayer();
    expect(usePlayerStore.getState().expanded).toBe(true);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(usePlayerStore.getState().expanded).toBe(false);
  });

  it('アンマウント後は Escape キーを無視する', () => {
    const { unmount } = renderPlayer();
    unmount();
    // expanded を再び true に設定
    usePlayerStore.setState({ expanded: true });
    fireEvent.keyDown(window, { key: 'Escape' });
    // クリーンアップ済みなので expanded は true のまま
    expect(usePlayerStore.getState().expanded).toBe(true);
  });
});
