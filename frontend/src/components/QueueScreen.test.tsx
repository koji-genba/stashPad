// QueueScreen(再生キュー画面)のテスト。
// 表示モード: 番号付き 2 行リスト(上段ファイル名・下段作品名)+行タップで再生。
// 編集モード: 行ごとの削除ボタンとドラッグハンドルで実キューへ即時反映。
// 「戻る」(history が 1 段戻る)でどちらのモードからもプレイヤーへ戻る。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { usePlayerStore } from '@/store/playerStore';

// API クライアントをモック化(playerStore.test.ts と同様)
vi.mock('@/api/client', () => ({
  recordPlay: vi.fn().mockResolvedValue(undefined),
  fileUrl: (workId: number, path: string) => `/api/works/${workId}/file?path=${encodeURIComponent(path)}`,
  thumbnailUrl: (workId: number) => `/api/works/${workId}/thumbnail`,
}));

import QueueScreen from './QueueScreen';

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

// 作品をまたいだ 3 トラックのキュー(a 再生中)
function setupQueue() {
  usePlayerStore.setState({
    queue: [
      { uid: 1, workId: 10, workTitle: '作品アルファ', name: 'a.mp3', path: 'a.mp3' },
      { uid: 2, workId: 10, workTitle: '作品アルファ', name: 'b.mp3', path: 'b.mp3' },
      { uid: 3, workId: 20, workTitle: '作品ベータ', name: 'c.mp3', path: 'CD2/c.mp3' },
    ],
    index: 0,
    isPlaying: true,
    loadNonce: 5,
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

/** キュー画面を開いた状態(プレイヤー+キューの 2 段を積んだ history)で描画する */
function renderQueueScreen() {
  return render(
    <MemoryRouter
      initialEntries={[
        { pathname: '/' },
        { pathname: '/', state: { fsPlayer: true } },
        { pathname: '/', state: { fsPlayer: true, fsQueue: true } },
      ]}
      initialIndex={2}
    >
      <QueueScreen />
      <LocationProbe />
    </MemoryRouter>,
  );
}

const queueNames = () => usePlayerStore.getState().queue.map((t) => t.name);

beforeEach(() => {
  resetStore();
  setupQueue();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('QueueScreen 表示モード', () => {
  it('全トラックが番号付きの 2 行(ファイル名+作品名)で表示される', () => {
    renderQueueScreen();
    const rows = screen.getAllByRole('listitem');
    expect(rows).toHaveLength(3);
    expect(within(rows[0]).getByText('1')).toBeInTheDocument();
    expect(within(rows[0]).getByText('a.mp3')).toBeInTheDocument();
    expect(within(rows[0]).getByText('作品アルファ')).toBeInTheDocument();
    expect(within(rows[2]).getByText('3')).toBeInTheDocument();
    expect(within(rows[2]).getByText('c.mp3')).toBeInTheDocument();
    expect(within(rows[2]).getByText('作品ベータ')).toBeInTheDocument();
  });

  it('ヘッダに現在位置と曲数が表示される', () => {
    renderQueueScreen();
    expect(screen.getByText('再生キュー(1/3)')).toBeInTheDocument();
  });

  it('現在再生中の行に aria-current が付く', () => {
    renderQueueScreen();
    expect(screen.getByRole('button', { name: 'a.mp3 を再生' })).toHaveAttribute(
      'aria-current',
      'true',
    );
    expect(screen.getByRole('button', { name: 'b.mp3 を再生' })).not.toHaveAttribute(
      'aria-current',
    );
  });

  it('行タップでそのトラックから再生する', () => {
    renderQueueScreen();
    fireEvent.click(screen.getByRole('button', { name: 'c.mp3 を再生' }));
    const s = usePlayerStore.getState();
    expect(s.index).toBe(2);
    expect(s.isPlaying).toBe(true);
    expect(s.loadNonce).toBe(6); // 新トラックのロード要求
  });

  it('「プレイヤーに戻る」でキュー画面だけが閉じる(history が 1 段戻る)', () => {
    renderQueueScreen();
    fireEvent.click(screen.getByRole('button', { name: 'プレイヤーに戻る' }));
    expect(probeText()).toContain('player=true');
    expect(probeText()).toContain('queue=false');
  });
});

describe('QueueScreen 編集モード', () => {
  function enterEditMode() {
    fireEvent.click(screen.getByRole('button', { name: 'キューを編集' }));
  }

  it('編集モードに入ると削除ボタンとドラッグハンドルが現れ、「完了」で表示モードに戻る', () => {
    renderQueueScreen();
    enterEditMode();
    expect(screen.getAllByRole('button', { name: /をキューから削除$/ })).toHaveLength(3);
    expect(screen.getAllByRole('button', { name: /を並び替え$/ })).toHaveLength(3);
    // 表示モードの再生ボタンは無い
    expect(screen.queryByRole('button', { name: 'a.mp3 を再生' })).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'キューの編集を終了' }));
    expect(screen.queryAllByRole('button', { name: /をキューから削除$/ })).toHaveLength(0);
    expect(screen.getByRole('button', { name: 'a.mp3 を再生' })).toBeInTheDocument();
  });

  it('編集モードでは行タップで再生されない', () => {
    renderQueueScreen();
    enterEditMode();
    fireEvent.click(screen.getByText('b.mp3'));
    const s = usePlayerStore.getState();
    expect(s.index).toBe(0);
    expect(s.loadNonce).toBe(5); // ロード要求は発生しない
  });

  it('削除ボタンでそのトラックがキューから消える', () => {
    renderQueueScreen();
    enterEditMode();
    fireEvent.click(screen.getByRole('button', { name: 'b.mp3 をキューから削除' }));
    expect(queueNames()).toEqual(['a.mp3', 'c.mp3']);
    // 再生中(a)はそのまま
    expect(usePlayerStore.getState().index).toBe(0);
    expect(usePlayerStore.getState().loadNonce).toBe(5);
  });

  it('再生中トラックを削除すると次のトラックの再生に進む', () => {
    renderQueueScreen();
    enterEditMode();
    fireEvent.click(screen.getByRole('button', { name: 'a.mp3 をキューから削除' }));
    const s = usePlayerStore.getState();
    expect(queueNames()).toEqual(['b.mp3', 'c.mp3']);
    expect(s.index).toBe(0); // 次のトラック(b)
    expect(s.loadNonce).toBe(6); // 新トラックのロード要求
    expect(s.isPlaying).toBe(true);
  });

  it('編集モード中に「戻る」で閉じても編集結果はキューに残る(即時反映)', () => {
    renderQueueScreen();
    enterEditMode();
    fireEvent.click(screen.getByRole('button', { name: 'c.mp3 をキューから削除' }));
    fireEvent.click(screen.getByRole('button', { name: 'プレイヤーに戻る' }));
    expect(probeText()).toContain('queue=false');
    expect(queueNames()).toEqual(['a.mp3', 'b.mp3']);
  });
});

describe('QueueScreen ドラッグ並び替え', () => {
  // jsdom はレイアウトを持たないため、リスト(OL)と行(LI)の矩形をモックする。
  // 行高 50px: 行 0 = y[0,50), 行 1 = y[50,100), 行 2 = y[100,150)
  function mockListRects() {
    const orig = Element.prototype.getBoundingClientRect;
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (
      this: Element,
    ) {
      if (this.tagName === 'OL') {
        return { top: 0, bottom: 300, left: 0, right: 320, width: 320, height: 300, x: 0, y: 0, toJSON: () => ({}) } as DOMRect;
      }
      if (this.tagName === 'LI') {
        return { top: 0, bottom: 50, left: 0, right: 320, width: 320, height: 50, x: 0, y: 0, toJSON: () => ({}) } as DOMRect;
      }
      return orig.call(this);
    });
  }

  function handleOf(name: string) {
    return screen.getByRole('button', { name: `${name} を並び替え` });
  }

  beforeEach(() => {
    mockListRects();
    renderQueueScreen();
    fireEvent.click(screen.getByRole('button', { name: 'キューを編集' }));
  });

  it('ハンドルを下へドラッグすると行が移動し、再生中 index も追従する', () => {
    const handle = handleOf('a.mp3');
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 25 });
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 125 }); // 行 2 の位置
    fireEvent.pointerUp(handle, { pointerId: 1 });
    expect(queueNames()).toEqual(['b.mp3', 'c.mp3', 'a.mp3']);
    expect(usePlayerStore.getState().index).toBe(2); // a 再生中 → 末尾へ追従
  });

  it('ドラッグ中の往復にも追従する(下げてから戻す)', () => {
    const handle = handleOf('a.mp3');
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 25 });
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 125 });
    expect(queueNames()).toEqual(['b.mp3', 'c.mp3', 'a.mp3']);
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 25 }); // 行 0 へ戻す
    fireEvent.pointerUp(handle, { pointerId: 1 });
    expect(queueNames()).toEqual(['a.mp3', 'b.mp3', 'c.mp3']);
    expect(usePlayerStore.getState().index).toBe(0);
  });

  it('リスト範囲外(下)へ動かしても末尾でクランプされる', () => {
    const handle = handleOf('a.mp3');
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 25 });
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 9999 });
    fireEvent.pointerUp(handle, { pointerId: 1 });
    expect(queueNames()).toEqual(['b.mp3', 'c.mp3', 'a.mp3']);
  });

  it('pointerDown していなければ pointerMove は何もしない', () => {
    const handle = handleOf('b.mp3');
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 125 });
    expect(queueNames()).toEqual(['a.mp3', 'b.mp3', 'c.mp3']);
  });
});
