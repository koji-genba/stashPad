// FullscreenPlayer コンポーネントのテスト。
// playerStore に再生状態を、MemoryRouter(location.state)に表示状態を仕込んで
// 描画・操作を検証する。閉じる操作 = history が 1 段戻ること。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { usePlayerStore } from '@/store/playerStore';

// API クライアントをモック化(playerStore.test.ts と同様)
vi.mock('@/api/client', () => ({
  recordPlay: vi.fn().mockResolvedValue(undefined),
  fileUrl: (workId: number, path: string) => `/api/works/${workId}/file?path=${encodeURIComponent(path)}`,
  thumbnailUrl: (workId: number) => `/api/works/${workId}/thumbnail`,
}));

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
  volume: 1,
  nextUid: 1,
  sleepMode: 'off' as const,
  sleepEndsAt: null,
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
    nextUid: 100,
  });
}

// 現在の location(パスとオーバーレイフラグ)を観測するテスト用プローブ
function LocationProbe() {
  const location = useLocation();
  const s = (location.state ?? {}) as { fsPlayer?: boolean; fsQueue?: boolean };
  return (
    <div data-testid="location-probe">
      {`path=${location.pathname} player=${s.fsPlayer === true} queue=${s.fsQueue === true}`}
    </div>
  );
}

const probeText = () => screen.getByTestId('location-probe').textContent ?? '';
/** プレイヤー本体が描画されているか(閉じるボタンの有無で判定) */
const playerVisible = () =>
  screen.queryByRole('button', { name: 'ミニプレイヤーに戻る' }) !== null;

/** プレイヤーを開いた状態(フラグ付きエントリを積んだ history)で描画する */
function renderPlayer(opts: { open?: boolean } = {}) {
  const open = opts.open ?? true;
  const entries = open
    ? [{ pathname: '/' }, { pathname: '/', state: { fsPlayer: true } }]
    : [{ pathname: '/' }];
  return render(
    <MemoryRouter initialEntries={entries} initialIndex={entries.length - 1}>
      <FullscreenPlayer />
      <LocationProbe />
    </MemoryRouter>,
  );
}

describe('FullscreenPlayer 描画条件', () => {
  beforeEach(resetStore);
  afterEach(cleanup);

  it('プレイヤーを開いていない(history にフラグなし)とき何も描画しない', () => {
    usePlayerStore.setState({
      queue: [{ uid: 1, workId: 1, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' }],
      index: 0,
      nextUid: 100,
    });
    renderPlayer({ open: false });
    expect(playerVisible()).toBe(false);
  });

  it('現在トラックが無い(キューが空)のとき何も描画しない', () => {
    usePlayerStore.setState({ queue: [], index: -1 });
    renderPlayer();
    expect(playerVisible()).toBe(false);
  });

  it('開いていて現在トラックがあるとき描画される', () => {
    setupPlayingState();
    renderPlayer();
    expect(screen.getByText('track02.mp3')).toBeInTheDocument();
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
    expect(screen.getByText('track02.mp3')).toBeInTheDocument();
  });

  it('作品タイトルが表示される', () => {
    renderPlayer();
    // ヘッダの作品タイトルボタンとメタエリアの両方に表示される
    const titles = screen.getAllByText('テスト作品タイトル');
    expect(titles.length).toBeGreaterThanOrEqual(1);
  });
});

describe('FullscreenPlayer 操作', () => {
  beforeEach(() => {
    resetStore();
    setupPlayingState();
  });
  afterEach(cleanup);

  it('閉じるボタンでプレイヤーが閉じる(history が 1 段戻る)', () => {
    renderPlayer();
    const closeBtn = screen.getByRole('button', { name: 'ミニプレイヤーに戻る' });
    fireEvent.click(closeBtn);
    expect(playerVisible()).toBe(false);
    expect(probeText()).toContain('player=false');
    expect(probeText()).toContain('path=/');
  });

  it('作品タイトルボタンクリックで作品ページへ遷移し、プレイヤーが閉じる', () => {
    renderPlayer();
    const titleBtn = screen.getByRole('button', { name: 'テスト作品タイトル の作品ページを開く' });
    fireEvent.click(titleBtn);
    expect(probeText()).toContain('path=/works/42');
    // 遷移先エントリにはフラグが無いのでプレイヤーは閉じる
    expect(playerVisible()).toBe(false);
  });

  it('キューボタンに現在番号/総数が表示され、クリックでキュー画面が開く', () => {
    renderPlayer();
    const btn = screen.getByRole('button', { name: '再生キューを表示' });
    expect(btn).toHaveTextContent('キュー(2/3)');
    fireEvent.click(btn);
    expect(probeText()).toContain('queue=true');
    // キュー画面(QueueScreen)が重なって表示される
    expect(screen.getByText('再生キュー(2/3)')).toBeInTheDocument();
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

  it('Escape キーでプレイヤーが閉じる', () => {
    renderPlayer();
    expect(playerVisible()).toBe(true);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(playerVisible()).toBe(false);
    expect(probeText()).toContain('player=false');
  });

  it('Escape はキュー画面 → プレイヤーの順に 1 段ずつ閉じる', () => {
    renderPlayer();
    fireEvent.click(screen.getByRole('button', { name: '再生キューを表示' }));
    expect(probeText()).toContain('queue=true');
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(probeText()).toContain('queue=false');
    expect(playerVisible()).toBe(true);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(playerVisible()).toBe(false);
  });

  it('アンマウント後は Escape キーを無視する(リスナーがクリーンアップされる)', () => {
    const entries = [{ pathname: '/' }, { pathname: '/', state: { fsPlayer: true } }];
    const { rerender } = render(
      <MemoryRouter initialEntries={entries} initialIndex={1}>
        <FullscreenPlayer />
        <LocationProbe />
      </MemoryRouter>,
    );
    expect(playerVisible()).toBe(true);
    // FullscreenPlayer だけ外す(MemoryRouter と history はそのまま)
    rerender(
      <MemoryRouter initialEntries={entries} initialIndex={1}>
        <LocationProbe />
      </MemoryRouter>,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    // リスナーがクリーンアップ済みなので history は戻らない
    expect(probeText()).toContain('player=true');
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

});

describe('FullscreenPlayer スワイプ操作', () => {
  beforeEach(() => {
    resetStore();
    setupPlayingState();
  });
  afterEach(cleanup);

  it('ヘッダ領域の下方向スワイプ(dy=+100, dx≈0)でプレイヤーが閉じる', () => {
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
    expect(playerVisible()).toBe(false);
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
    expect(playerVisible()).toBe(true);
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
    expect(playerVisible()).toBe(true);
  });
});

describe('FullscreenPlayer スリープタイマー', () => {
  beforeEach(() => {
    resetStore();
    setupPlayingState();
  });
  afterEach(cleanup);

  it('未設定のときプリセット(15/30/60分)と「曲終わり」ボタンが表示される', () => {
    renderPlayer();
    expect(screen.getByRole('button', { name: '15分後に停止' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '30分後に停止' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '60分後に停止' })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'このトラックの終わりで停止' }),
    ).toBeInTheDocument();
  });

  it('プリセットをクリックすると sleepMode=duration になり残り時間が表示される', () => {
    renderPlayer();
    fireEvent.click(screen.getByRole('button', { name: '30分後に停止' }));
    expect(usePlayerStore.getState().sleepMode).toBe('duration');
    expect(usePlayerStore.getState().sleepEndsAt).not.toBeNull();
    // 作動中インジケータ(status ロール)に切り替わる
    expect(screen.getByRole('status')).toHaveTextContent('停止まで');
  });

  it('「このトラックの終わりで停止」をクリックすると endOfTrack 表示になる', () => {
    renderPlayer();
    fireEvent.click(
      screen.getByRole('button', { name: 'このトラックの終わりで停止' }),
    );
    expect(usePlayerStore.getState().sleepMode).toBe('endOfTrack');
    expect(screen.getByRole('status')).toHaveTextContent('このトラックの終わりで停止');
  });

  it('作動中に「解除」を押すと未設定に戻る', () => {
    usePlayerStore.setState({ sleepMode: 'endOfTrack', sleepEndsAt: null });
    renderPlayer();
    fireEvent.click(screen.getByRole('button', { name: 'スリープタイマーを解除' }));
    expect(usePlayerStore.getState().sleepMode).toBe('off');
    // 解除後はプリセットボタンが再表示される
    expect(screen.getByRole('button', { name: '15分後に停止' })).toBeInTheDocument();
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
    setupPlayingState();
    renderPlayer();
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('閉じると overflow が元に戻る', () => {
    setupPlayingState();
    renderPlayer();
    expect(document.body.style.overflow).toBe('hidden');
    // 閉じる(history が戻る)→ 再レンダーで effect のクリーンアップが走る
    fireEvent.click(screen.getByRole('button', { name: 'ミニプレイヤーに戻る' }));
    expect(document.body.style.overflow).not.toBe('hidden');
  });
});
