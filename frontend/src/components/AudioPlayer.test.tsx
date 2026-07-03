// AudioPlayer コンポーネントのテスト。
// <audio> 要素を ref で保持し、store の命令を反映する構造をテストする。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { usePlayerStore } from '@/store/playerStore';

// API クライアントをモック化(FullscreenPlayer.test.tsx と同一)
vi.mock('@/api/client', () => ({
  recordPlay: vi.fn().mockResolvedValue(undefined),
  fileUrl: (workId: number, path: string) => `/api/works/${workId}/file?path=${encodeURIComponent(path)}`,
  thumbnailUrl: (workId: number) => `/api/works/${workId}/thumbnail`,
}));

// vi.mock はファイル先頭に巻き上げられるため、この import 時点でモックは適用済み
import AudioPlayer from './AudioPlayer';
import { _resetForTest, loadResumePosition } from '@/lib/playbackMemory';

const PLAYBACK_POSITIONS_KEY = 'stashpad-playback-positions';

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

/** フルスクリーンプレイヤーが開いているか(閉じるボタンの有無で判定) */
function fullscreenVisible(): boolean {
  return screen.queryByRole('button', { name: 'ミニプレイヤーに戻る' }) !== null;
}

function renderPlayer() {
  return render(
    <MemoryRouter>
      <AudioPlayer />
    </MemoryRouter>,
  );
}

// jsdom は HTMLMediaElement.play/pause/load を実装していないため全テストでスタブする
beforeEach(() => {
  resetStore();
  localStorage.clear();
  _resetForTest();
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => undefined);
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => undefined);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

/** ミニバー要素を取得する。<audio> と異なり aria-label を持たないため最初の div で特定する */
function queryBar(container: HTMLElement): HTMLElement | null {
  return container.querySelector('div');
}

describe('AudioPlayer 描画条件', () => {
  it('キューが空(現在トラックなし)のとき何も描画しない', () => {
    usePlayerStore.setState({ queue: [], index: -1 });
    const { container } = renderPlayer();
    // 現在トラックが無いのでミニバー自体がレンダーされない(<audio> は残る)
    expect(container.querySelector('[aria-label="フルスクリーンプレイヤーを開く"]')).toBeNull();
  });
});

describe('AudioPlayer フルスクリーン展開', () => {
  beforeEach(setupPlayingState);

  it('サムネ/メタ領域ボタンクリックでフルスクリーンプレイヤーが開く', () => {
    renderPlayer();
    const btn = screen.getByRole('button', { name: 'フルスクリーンプレイヤーを開く' });
    fireEvent.click(btn);
    expect(fullscreenVisible()).toBe(true);
  });

  it('ミニバーの上方向スワイプ(dy=-100, dx≈0)でフルスクリーンプレイヤーが開く', () => {
    const { container } = renderPlayer();
    const bar = queryBar(container);
    expect(bar).not.toBeNull();
    fireEvent.touchStart(bar!, {
      touches: [{ clientX: 100, clientY: 300 }],
    });
    fireEvent.touchEnd(bar!, {
      changedTouches: [{ clientX: 102, clientY: 200 }],
    });
    expect(fullscreenVisible()).toBe(true);
  });

  it('縦移動が 60px 以下のスワイプでは開かない', () => {
    const { container } = renderPlayer();
    const bar = queryBar(container);
    expect(bar).not.toBeNull();
    fireEvent.touchStart(bar!, {
      touches: [{ clientX: 100, clientY: 300 }],
    });
    fireEvent.touchEnd(bar!, {
      changedTouches: [{ clientX: 100, clientY: 260 }], // dy=-40
    });
    expect(fullscreenVisible()).toBe(false);
  });

  it('横優位スワイプ(dx=-120, dy=-80)では開かない', () => {
    const { container } = renderPlayer();
    const bar = queryBar(container);
    expect(bar).not.toBeNull();
    fireEvent.touchStart(bar!, {
      touches: [{ clientX: 200, clientY: 300 }],
    });
    fireEvent.touchEnd(bar!, {
      changedTouches: [{ clientX: 80, clientY: 220 }], // dx=-120, dy=-80
    });
    expect(fullscreenVisible()).toBe(false);
  });
});

describe('AudioPlayer <audio> 要素へのストア反映', () => {
  beforeEach(setupPlayingState);

  it('store.volume を 0.5 にすると <audio> 要素の volume が 0.5 になる', () => {
    const { container } = renderPlayer();
    const audio = container.querySelector('audio');
    expect(audio).not.toBeNull();
    act(() => {
      usePlayerStore.getState().setVolume(0.5);
    });
    expect(audio!.volume).toBe(0.5);
  });

  it('store.playbackRate を変えると <audio> の playbackRate に反映される', () => {
    const { container } = renderPlayer();
    const audio = container.querySelector('audio');
    expect(audio).not.toBeNull();
    act(() => {
      usePlayerStore.getState().setRate(1.5);
    });
    expect(audio!.playbackRate).toBe(1.5);
  });
});

describe('AudioPlayer 続きから再生(playbackMemory 連携)', () => {
  beforeEach(setupPlayingState);

  it('保存済み位置(30秒以上)があるとき、loadedmetadata で currentTime が復元される', () => {
    localStorage.setItem(
      PLAYBACK_POSITIONS_KEY,
      JSON.stringify({ '42:track02.mp3': { position: 500, duration: 700, updatedAt: Date.now() } }),
    );
    const { container } = renderPlayer();
    const audio = container.querySelector('audio')!;
    expect(audio).not.toBeNull();

    Object.defineProperty(audio, 'duration', { value: 700, configurable: true });
    fireEvent(audio, new Event('loadedmetadata'));

    expect(audio.currentTime).toBe(500);
    expect(usePlayerStore.getState().currentTime).toBe(500);
  });

  it('保存済み位置が無いとき、loadedmetadata が発火しても currentTime は復元されない', () => {
    const { container } = renderPlayer();
    const audio = container.querySelector('audio')!;

    Object.defineProperty(audio, 'duration', { value: 700, configurable: true });
    fireEvent(audio, new Event('loadedmetadata'));

    expect(audio.currentTime).toBe(0);
  });

  it('位置が末尾30秒以内に食い込む場合は再開しない', () => {
    localStorage.setItem(
      PLAYBACK_POSITIONS_KEY,
      JSON.stringify({ '42:track02.mp3': { position: 690, duration: 700, updatedAt: Date.now() } }),
    );
    const { container } = renderPlayer();
    const audio = container.querySelector('audio')!;

    // 実際にロードされた duration は 700 → 700 - 690 = 10 秒 < 30 秒なので再開しない
    Object.defineProperty(audio, 'duration', { value: 700, configurable: true });
    fireEvent(audio, new Event('loadedmetadata'));

    expect(audio.currentTime).toBe(0);
  });

  it('onEnded で clearProgress が呼ばれ、保存済み位置が消える', () => {
    localStorage.setItem(
      PLAYBACK_POSITIONS_KEY,
      JSON.stringify({ '42:track02.mp3': { position: 500, duration: 700, updatedAt: Date.now() } }),
    );
    const { container } = renderPlayer();
    const audio = container.querySelector('audio')!;

    fireEvent(audio, new Event('ended'));

    expect(loadResumePosition(42, 'track02.mp3')).toBeNull();
  });

  it('onPause(ended でない)で flushProgress により位置が即座に保存される', () => {
    const { container } = renderPlayer();
    const audio = container.querySelector('audio')!;
    Object.defineProperty(audio, 'duration', { value: 700, configurable: true });
    Object.defineProperty(audio, 'currentTime', { value: 200, configurable: true, writable: true });

    fireEvent(audio, new Event('pause'));

    expect(loadResumePosition(42, 'track02.mp3')).toBe(200);
  });

  it('onTimeUpdate で recordProgress により位置が保存される(初回は throttle されない)', () => {
    const { container } = renderPlayer();
    const audio = container.querySelector('audio')!;
    Object.defineProperty(audio, 'duration', { value: 700, configurable: true });
    Object.defineProperty(audio, 'currentTime', { value: 150, configurable: true, writable: true });

    fireEvent.timeUpdate(audio);

    expect(loadResumePosition(42, 'track02.mp3')).toBe(150);
  });
});

describe('AudioPlayer トラック操作ボタン', () => {
  it('「前のトラック」ボタンは queue が 1 件以下で disabled', () => {
    usePlayerStore.setState({
      queue: [{ uid: 1, workId: 1, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' }],
      index: 0,
      nextUid: 100,
    });
    renderPlayer();
    const prevBtn = screen.getByRole('button', { name: '前のトラック' });
    expect(prevBtn).toBeDisabled();
  });

  it('「次のトラック」ボタンは末尾 index で disabled', () => {
    usePlayerStore.setState({
      queue: [
        { uid: 1, workId: 1, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' },
        { uid: 2, workId: 1, workTitle: 'テスト', name: 'b.mp3', path: 'b.mp3' },
      ],
      index: 1, // 末尾
      nextUid: 100,
    });
    renderPlayer();
    const nextBtn = screen.getByRole('button', { name: '次のトラック' });
    expect(nextBtn).toBeDisabled();
  });
});
