// playerStore の状態遷移テスト。
// zustand ストアはモジュールレベルでシングルトンなので、
// 各テスト前に setState で初期状態にリセットする。
import { beforeEach, describe, expect, it, vi } from 'vitest';

// recordPlay / thumbnailUrl / fileUrl をモック化し、副作用を切り離す
vi.mock('@/api/client', () => ({
  recordPlay: vi.fn().mockResolvedValue(undefined),
  fileUrl: (workId: number, path: string) => `/api/works/${workId}/file?path=${encodeURIComponent(path)}`,
  thumbnailUrl: (workId: number) => `/api/works/${workId}/thumbnail`,
}));

import { currentSrc, currentTrack, playerThumbUrl, usePlayerStore } from './playerStore';
import type { Entry } from '@/api/types';

// テスト前にストアを初期状態にリセット
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
};

function resetStore() {
  // replace=false でマージ: メソッドを保持しつつデータプロパティのみリセット
  usePlayerStore.setState(initialState, false);
}

// テスト用エントリ生成ヘルパ
function makeEntry(name: string, media_kind: Entry['media_kind'] = 'audio'): Entry {
  return { name, is_dir: false, size: 0, media_kind };
}

describe('playerStore 初期状態', () => {
  beforeEach(resetStore);

  it('初期値が正しい', () => {
    const s = usePlayerStore.getState();
    expect(s.ctx).toBeNull();
    expect(s.queue).toEqual([]);
    expect(s.index).toBe(-1);
    expect(s.isPlaying).toBe(false);
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.playbackRate).toBe(1);
    expect(s.seekRequest).toBeNull();
    expect(s.loadNonce).toBe(0);
  });
});

describe('currentTrack / currentSrc セレクタ', () => {
  beforeEach(resetStore);

  it('キューが空の場合は null を返す', () => {
    expect(currentTrack(usePlayerStore.getState())).toBeNull();
    expect(currentSrc(usePlayerStore.getState())).toBeNull();
  });

  it('キューに要素があり index が有効なら正しいトラックを返す', () => {
    usePlayerStore.setState({
      ctx: { workId: 1, workTitle: 'テスト作品', dir: '' },
      queue: [{ name: 'track1.mp3', path: 'track1.mp3' }],
      index: 0,
    });
    const track = currentTrack(usePlayerStore.getState());
    expect(track).toEqual({ name: 'track1.mp3', path: 'track1.mp3' });

    const src = currentSrc(usePlayerStore.getState());
    expect(src).toBe('/api/works/1/file?path=track1.mp3');
  });

  it('index が範囲外の場合は null を返す', () => {
    usePlayerStore.setState({
      ctx: { workId: 1, workTitle: 'テスト', dir: '' },
      queue: [{ name: 'a.mp3', path: 'a.mp3' }],
      index: 5, // 範囲外
    });
    expect(currentTrack(usePlayerStore.getState())).toBeNull();
  });
});

describe('startFromEntries', () => {
  beforeEach(resetStore);

  const audioEntries: Entry[] = [
    makeEntry('track1.mp3', 'audio'),
    makeEntry('track2.mp3', 'audio'),
    makeEntry('image.jpg', 'image'), // audio 以外はキューに含まれない
    // is_dir=true のエントリを作るにはオブジェクト直書きが必要(makeEntry は is_dir=false 固定)
    { name: 'subdir', is_dir: true, size: 0, media_kind: 'audio' },
  ];

  it('startName が見つかると再生が開始される', () => {
    const s = usePlayerStore.getState();
    s.startFromEntries({
      workId: 42,
      workTitle: '音声作品',
      dir: '',
      entries: audioEntries,
      startName: 'track2.mp3',
    });
    const after = usePlayerStore.getState();
    expect(after.isPlaying).toBe(true);
    expect(after.index).toBe(1); // track2.mp3 は audio キューの 2 番目
    expect(after.ctx?.workId).toBe(42);
    expect(after.ctx?.workTitle).toBe('音声作品');
    // image.jpg と is_dir=true のサブディレクトリは除外、audio 2 件のみ
    expect(after.queue).toHaveLength(2);
    expect(after.queue[0].name).toBe('track1.mp3');
    expect(after.queue[1].name).toBe('track2.mp3');
    expect(after.loadNonce).toBeGreaterThan(0);
  });

  it('dir が指定された場合、path に dir が付く', () => {
    const s = usePlayerStore.getState();
    s.startFromEntries({
      workId: 1,
      workTitle: 'テスト',
      dir: 'subdir',
      entries: [makeEntry('a.mp3', 'audio')],
      startName: 'a.mp3',
    });
    const after = usePlayerStore.getState();
    expect(after.queue[0].path).toBe('subdir/a.mp3');
  });

  it('startName が見つからない場合は何もしない', () => {
    const s = usePlayerStore.getState();
    s.startFromEntries({
      workId: 1,
      workTitle: 'テスト',
      dir: '',
      entries: audioEntries,
      startName: 'nonexistent.mp3',
    });
    const after = usePlayerStore.getState();
    expect(after.isPlaying).toBe(false);
    expect(after.queue).toEqual([]);
  });

  it('audio エントリが空の場合は何もしない', () => {
    const s = usePlayerStore.getState();
    s.startFromEntries({
      workId: 1,
      workTitle: 'テスト',
      dir: '',
      entries: [makeEntry('image.jpg', 'image')],
      startName: 'image.jpg',
    });
    const after = usePlayerStore.getState();
    expect(after.isPlaying).toBe(false);
  });

  it('currentTime と duration がリセットされる', () => {
    usePlayerStore.setState({ currentTime: 99, duration: 999 });
    const s = usePlayerStore.getState();
    s.startFromEntries({
      workId: 1,
      workTitle: 'テスト',
      dir: '',
      entries: [makeEntry('a.mp3', 'audio')],
      startName: 'a.mp3',
    });
    const after = usePlayerStore.getState();
    expect(after.currentTime).toBe(0);
    expect(after.duration).toBe(0);
  });
});

describe('playIndex', () => {
  beforeEach(() => {
    resetStore();
    usePlayerStore.setState({
      ctx: { workId: 1, workTitle: 'テスト', dir: '' },
      queue: [
        { name: 'a.mp3', path: 'a.mp3' },
        { name: 'b.mp3', path: 'b.mp3' },
        { name: 'c.mp3', path: 'c.mp3' },
      ],
      index: 0,
    });
  });

  it('有効な index に移動する', () => {
    usePlayerStore.getState().playIndex(2);
    expect(usePlayerStore.getState().index).toBe(2);
    expect(usePlayerStore.getState().isPlaying).toBe(true);
  });

  it('負の index は無視される', () => {
    usePlayerStore.getState().playIndex(-1);
    expect(usePlayerStore.getState().index).toBe(0); // 変化なし
  });

  it('範囲外の index は無視される', () => {
    usePlayerStore.getState().playIndex(10);
    expect(usePlayerStore.getState().index).toBe(0); // 変化なし
  });

  it('playIndex で currentTime と duration がリセットされる', () => {
    usePlayerStore.setState({ currentTime: 50, duration: 200 });
    usePlayerStore.getState().playIndex(1);
    const s = usePlayerStore.getState();
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
  });

  it('loadNonce が増加する', () => {
    const before = usePlayerStore.getState().loadNonce;
    usePlayerStore.getState().playIndex(1);
    expect(usePlayerStore.getState().loadNonce).toBe(before + 1);
  });
});

describe('next / prev', () => {
  beforeEach(() => {
    resetStore();
    usePlayerStore.setState({
      ctx: { workId: 1, workTitle: 'テスト', dir: '' },
      queue: [
        { name: 'a.mp3', path: 'a.mp3' },
        { name: 'b.mp3', path: 'b.mp3' },
        { name: 'c.mp3', path: 'c.mp3' },
      ],
      index: 1,
      currentTime: 0,
    });
  });

  it('next で次のトラックに移動する', () => {
    usePlayerStore.getState().next();
    expect(usePlayerStore.getState().index).toBe(2);
  });

  it('最終トラックで next しても移動しない', () => {
    usePlayerStore.setState({ index: 2 });
    usePlayerStore.getState().next();
    expect(usePlayerStore.getState().index).toBe(2);
  });

  it('prev で前のトラックに移動する(currentTime <= 3)', () => {
    usePlayerStore.setState({ currentTime: 1 });
    usePlayerStore.getState().prev();
    expect(usePlayerStore.getState().index).toBe(0);
  });

  it('currentTime > 3 のとき prev は先頭(0秒)にシークする', () => {
    usePlayerStore.setState({ currentTime: 5 });
    usePlayerStore.getState().prev();
    // index は変化しない(シークのみ)
    expect(usePlayerStore.getState().index).toBe(1);
    // seekRequest が設定される
    expect(usePlayerStore.getState().seekRequest?.time).toBe(0);
    expect(usePlayerStore.getState().currentTime).toBe(0);
  });

  it('先頭トラック(index=0)で currentTime <= 3 のとき prev は何もしない', () => {
    usePlayerStore.setState({ index: 0, currentTime: 1 });
    usePlayerStore.getState().prev();
    expect(usePlayerStore.getState().index).toBe(0);
  });

  it('currentTime がぴったり 3 秒のとき prev は前トラックへ移動する', () => {
    // currentTime > 3 の条件: 3 秒ちょうどは前トラックへ移動
    usePlayerStore.setState({ currentTime: 3 });
    usePlayerStore.getState().prev();
    expect(usePlayerStore.getState().index).toBe(0);
  });
});

describe('togglePlay / setPlaying', () => {
  beforeEach(resetStore);

  it('togglePlay で再生/停止を切り替える', () => {
    expect(usePlayerStore.getState().isPlaying).toBe(false);
    usePlayerStore.getState().togglePlay();
    expect(usePlayerStore.getState().isPlaying).toBe(true);
    usePlayerStore.getState().togglePlay();
    expect(usePlayerStore.getState().isPlaying).toBe(false);
  });

  it('setPlaying で任意の値を設定できる', () => {
    usePlayerStore.getState().setPlaying(true);
    expect(usePlayerStore.getState().isPlaying).toBe(true);
    usePlayerStore.getState().setPlaying(false);
    expect(usePlayerStore.getState().isPlaying).toBe(false);
  });
});

describe('seekBy / seekTo', () => {
  beforeEach(() => {
    resetStore();
    usePlayerStore.setState({ currentTime: 30, duration: 120, loadNonce: 5 });
  });

  it('seekTo で currentTime が変わり seekRequest が更新される', () => {
    usePlayerStore.getState().seekTo(60);
    const s = usePlayerStore.getState();
    expect(s.currentTime).toBe(60);
    expect(s.seekRequest?.time).toBe(60);
  });

  it('seekTo を連続で呼ぶと nonce が単調増加する(同じ time でも新しい要求として区別)', () => {
    usePlayerStore.getState().seekTo(60);
    const n1 = usePlayerStore.getState().seekRequest!.nonce;
    usePlayerStore.getState().seekTo(60);
    const n2 = usePlayerStore.getState().seekRequest!.nonce;
    expect(n2).toBe(n1 + 1);
  });

  it('seekBy で currentTime をデルタ分変化させる', () => {
    usePlayerStore.getState().seekBy(10);
    expect(usePlayerStore.getState().currentTime).toBe(40);
  });

  it('seekBy で負のデルタを適用できる', () => {
    usePlayerStore.getState().seekBy(-10);
    expect(usePlayerStore.getState().currentTime).toBe(20);
  });

  it('seekBy の結果が 0 未満にならない', () => {
    usePlayerStore.getState().seekBy(-9999);
    expect(usePlayerStore.getState().currentTime).toBe(0);
  });

  it('seekBy の結果が duration を超えない', () => {
    usePlayerStore.getState().seekBy(9999);
    expect(usePlayerStore.getState().currentTime).toBe(120);
  });

  it('duration が 0 の場合 seekBy は currentTime を 0 以上 Infinity 未満にクランプ', () => {
    usePlayerStore.setState({ currentTime: 10, duration: 0 });
    usePlayerStore.getState().seekBy(9999);
    // duration=0 → Math.min(Infinity, 10+9999) = 10009
    expect(usePlayerStore.getState().currentTime).toBe(10009);
  });
});

describe('setRate / setCurrentTime / setDuration', () => {
  beforeEach(resetStore);

  it('setRate で playbackRate が更新される', () => {
    usePlayerStore.getState().setRate(1.5);
    expect(usePlayerStore.getState().playbackRate).toBe(1.5);
  });

  it('setCurrentTime で currentTime が更新される', () => {
    usePlayerStore.getState().setCurrentTime(42);
    expect(usePlayerStore.getState().currentTime).toBe(42);
  });

  it('setDuration で duration が更新される', () => {
    usePlayerStore.getState().setDuration(300);
    expect(usePlayerStore.getState().duration).toBe(300);
  });
});

describe('handleEnded', () => {
  beforeEach(() => {
    resetStore();
    usePlayerStore.setState({
      ctx: { workId: 1, workTitle: 'テスト', dir: '' },
      queue: [
        { name: 'a.mp3', path: 'a.mp3' },
        { name: 'b.mp3', path: 'b.mp3' },
      ],
      index: 0,
      isPlaying: true,
    });
  });

  it('次のトラックがある場合は次に進む', () => {
    usePlayerStore.getState().handleEnded();
    expect(usePlayerStore.getState().index).toBe(1);
    expect(usePlayerStore.getState().isPlaying).toBe(true);
  });

  it('最後のトラックが終了した場合は停止する', () => {
    usePlayerStore.setState({ index: 1 });
    usePlayerStore.getState().handleEnded();
    expect(usePlayerStore.getState().index).toBe(1); // 変化なし
    expect(usePlayerStore.getState().isPlaying).toBe(false);
  });
});

describe('playerThumbUrl', () => {
  it('ctx が null の場合は null を返す', () => {
    expect(playerThumbUrl(null)).toBeNull();
  });

  it('ctx がある場合はサムネ URL を返す', () => {
    const ctx = { workId: 5, workTitle: 'テスト', dir: '' };
    expect(playerThumbUrl(ctx)).toBe('/api/works/5/thumbnail');
  });
});
