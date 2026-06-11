// FullscreenPlayer コンポーネントのテスト。
// playerStore に状態を仕込んで描画・操作を検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, act } from '@testing-library/react';
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
  nextUid: 1,
};

function resetStore() {
  usePlayerStore.setState(initialState, false);
}

// テスト用の最低限の状態を設定するヘルパ
function setupPlayingState() {
  usePlayerStore.setState({
    queue: [
      { uid: 1, workId: 42, workTitle: 'テスト作品タイトル', name: 'track01.mp3', path: 'track01.mp3' },
      { uid: 2, workId: 42, workTitle: 'テスト作品タイトル', name: 'track02.mp3', path: 'track02.mp3' },
      { uid: 3, workId: 42, workTitle: 'テスト作品タイトル', name: 'track03.mp3', path: 'track03.mp3' },
    ],
    index: 1,
    isPlaying: true,
    currentTime: 30,
    duration: 120,
    expanded: true,
    nextUid: 100,
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
      queue: [{ uid: 1, workId: 1, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' }],
      index: 0,
      expanded: false,
      nextUid: 100,
    });
    const { container } = renderPlayer();
    expect(container.firstChild).toBeNull();
  });

  it('現在トラックが無い(キューが空)のとき何も描画しない', () => {
    usePlayerStore.setState({ queue: [], index: -1, expanded: true });
    const { container } = renderPlayer();
    expect(container.firstChild).toBeNull();
  });

  it('expanded=true かつ 現在トラックがあるとき描画される', () => {
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

describe('FullscreenPlayer 操作系(追加)', () => {
  beforeEach(() => {
    resetStore();
    setupPlayingState();
  });
  afterEach(cleanup);

  it('再生/一時停止ボタンクリックで isPlaying がトグルする', () => {
    renderPlayer();
    // 初期状態は isPlaying=true → ボタンは「一時停止」
    const pauseBtn = screen.getByRole('button', { name: '一時停止' });
    fireEvent.click(pauseBtn);
    expect(usePlayerStore.getState().isPlaying).toBe(false);
    // 再度クリックで再生に戻る
    const playBtn = screen.getByRole('button', { name: '再生' });
    fireEvent.click(playBtn);
    expect(usePlayerStore.getState().isPlaying).toBe(true);
  });

  it('−10 ボタンで currentTime が 10 秒戻る', () => {
    renderPlayer();
    const btn = screen.getByRole('button', { name: '10秒戻る' });
    fireEvent.click(btn);
    // currentTime=30 → 30-10=20
    expect(usePlayerStore.getState().currentTime).toBe(20);
  });

  it('+10 ボタンで currentTime が 10 秒進む', () => {
    renderPlayer();
    const btn = screen.getByRole('button', { name: '10秒進む' });
    fireEvent.click(btn);
    // currentTime=30 → 30+10=40
    expect(usePlayerStore.getState().currentTime).toBe(40);
  });

  it('「前のトラック」: queue 1 件以下で disabled', () => {
    usePlayerStore.setState({
      queue: [{ uid: 1, workId: 42, workTitle: 'テスト作品タイトル', name: 'only.mp3', path: 'only.mp3' }],
      index: 0,
    });
    renderPlayer();
    const prevBtn = screen.getByRole('button', { name: '前のトラック' });
    expect(prevBtn).toBeDisabled();
  });

  it('「次のトラック」: 末尾 index で disabled', () => {
    // setupPlayingState では queue.length=3, index=1 なので末尾に変更
    usePlayerStore.setState({ index: 2 });
    renderPlayer();
    const nextBtn = screen.getByRole('button', { name: '次のトラック' });
    expect(nextBtn).toBeDisabled();
  });

  it('currentTime > 3 で「前のトラック」を押すと index は変わらず currentTime が 0 になる', () => {
    // setupPlayingState: index=1, currentTime=30
    renderPlayer();
    const prevBtn = screen.getByRole('button', { name: '前のトラック' });
    fireEvent.click(prevBtn);
    const state = usePlayerStore.getState();
    expect(state.index).toBe(1); // index は変わらない
    expect(state.currentTime).toBe(0); // 先頭へシーク
  });

  it('音量スライダーの change で store.volume が反映される', () => {
    renderPlayer();
    const slider = screen.getByRole('slider', { name: '音量' });
    fireEvent.change(slider, { target: { value: '0.3' } });
    expect(usePlayerStore.getState().volume).toBeCloseTo(0.3);
  });

  it('再生速度 select の change で store.playbackRate が反映される', () => {
    renderPlayer();
    const select = screen.getByRole('combobox', { name: '再生速度' });
    fireEvent.change(select, { target: { value: '1.5' } });
    expect(usePlayerStore.getState().playbackRate).toBe(1.5);
  });

  it('シークバーの change で store.currentTime と seekRequest が更新される', () => {
    renderPlayer();
    const seekBar = screen.getByRole('slider', { name: '再生位置' });
    fireEvent.change(seekBar, { target: { value: '60' } });
    const state = usePlayerStore.getState();
    expect(state.currentTime).toBe(60);
    expect(state.seekRequest).not.toBeNull();
    expect(state.seekRequest!.time).toBe(60);
  });

  it('キュー見出しに「キュー(2/3)」のように現在番号/総数が表示される', () => {
    // setupPlayingState: index=1(表示は 2), queue.length=3
    renderPlayer();
    expect(screen.getByText('キュー(2/3)')).toBeInTheDocument();
  });
});

describe('FullscreenPlayer スワイプ操作', () => {
  beforeEach(() => {
    resetStore();
    setupPlayingState();
  });
  afterEach(cleanup);

  it('ヘッダ領域の下方向スワイプ(dy=+100, dx≈0)で expanded が false になる', () => {
    renderPlayer();
    // ヘッダ要素: 閉じるボタンの親要素を辿る
    const closeBtn = screen.getByRole('button', { name: 'ミニプレイヤーに戻る' });
    const header = closeBtn.parentElement!;
    fireEvent.touchStart(header, {
      touches: [{ clientX: 100, clientY: 50 }],
    });
    fireEvent.touchEnd(header, {
      changedTouches: [{ clientX: 102, clientY: 150 }], // dy=+100
    });
    expect(usePlayerStore.getState().expanded).toBe(false);
  });

  it('横優位スワイプ(dx=120, dy=80)では閉じない', () => {
    renderPlayer();
    const closeBtn = screen.getByRole('button', { name: 'ミニプレイヤーに戻る' });
    const header = closeBtn.parentElement!;
    fireEvent.touchStart(header, {
      touches: [{ clientX: 100, clientY: 50 }],
    });
    fireEvent.touchEnd(header, {
      changedTouches: [{ clientX: 220, clientY: 130 }], // dx=120, dy=80
    });
    expect(usePlayerStore.getState().expanded).toBe(true);
  });

  it('縦 60px 以下のスワイプでは閉じない', () => {
    renderPlayer();
    const closeBtn = screen.getByRole('button', { name: 'ミニプレイヤーに戻る' });
    const header = closeBtn.parentElement!;
    fireEvent.touchStart(header, {
      touches: [{ clientX: 100, clientY: 50 }],
    });
    fireEvent.touchEnd(header, {
      changedTouches: [{ clientX: 100, clientY: 90 }], // dy=40(60px 未満)
    });
    expect(usePlayerStore.getState().expanded).toBe(true);
  });
});

describe('FullscreenPlayer body スクロールロック', () => {
  beforeEach(() => {
    resetStore();
  });
  afterEach(() => {
    cleanup();
    // body.style.overflow をリセット
    document.body.style.overflow = '';
  });

  it('表示中は document.body.style.overflow が "hidden" になる', () => {
    setupPlayingState(); // expanded=true
    renderPlayer();
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('setExpanded(false) にすると overflow が元に戻る', () => {
    setupPlayingState(); // expanded=true
    renderPlayer();
    expect(document.body.style.overflow).toBe('hidden');
    // store 更新による再レンダーで effect のクリーンアップが走る
    act(() => {
      usePlayerStore.getState().setExpanded(false);
    });
    expect(document.body.style.overflow).not.toBe('hidden');
  });
});
