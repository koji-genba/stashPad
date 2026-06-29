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
import type { EnqueueInput } from './playerStore';
import { recordPlay } from '@/api/client';
import type { Entry } from '@/api/types';

// モック化された recordPlay。呼ばれ方(回数・引数)の検証に使う
const recordPlayMock = vi.mocked(recordPlay);

// テスト前にストアを初期状態にリセット
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
    expect(s.queue).toEqual([]);
    expect(s.index).toBe(-1);
    expect(s.isPlaying).toBe(false);
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.playbackRate).toBe(1);
    expect(s.seekRequest).toBeNull();
    expect(s.loadNonce).toBe(0);
    expect(s.nextUid).toBe(1);
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
      queue: [{ uid: 1, workId: 1, workTitle: 'テスト作品', name: 'track1.mp3', path: 'track1.mp3' }],
      index: 0,
      nextUid: 100,
    });
    const track = currentTrack(usePlayerStore.getState());
    expect(track).toEqual({ uid: 1, workId: 1, workTitle: 'テスト作品', name: 'track1.mp3', path: 'track1.mp3' });

    const src = currentSrc(usePlayerStore.getState());
    expect(src).toBe('/api/works/1/file?path=track1.mp3');
  });

  it('index が範囲外の場合は null を返す', () => {
    usePlayerStore.setState({
      queue: [{ uid: 1, workId: 1, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' }],
      index: 5, // 範囲外
      nextUid: 100,
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
    expect(after.queue[after.index].workId).toBe(42);
    expect(after.queue[after.index].workTitle).toBe('音声作品');
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

// ---- キュー操作アクション(作品横断キュー)----

// EnqueueInput を作るヘルパ(uid は store が採番するので含めない)
function makeInput(name: string, workId = 9, workTitle = '追加元作品'): EnqueueInput {
  return { workId, workTitle, name, path: name };
}

// 非空キューの共通フィクスチャ。手書き uid(1..3)と衝突しないよう nextUid=100
function setupQueueOf3() {
  usePlayerStore.setState({
    queue: [
      { uid: 1, workId: 1, workTitle: '作品A', name: 'a.mp3', path: 'a.mp3' },
      { uid: 2, workId: 1, workTitle: '作品A', name: 'b.mp3', path: 'b.mp3' },
      { uid: 3, workId: 1, workTitle: '作品A', name: 'c.mp3', path: 'c.mp3' },
    ],
    index: 0,
    isPlaying: true,
    currentTime: 12,
    duration: 100,
    loadNonce: 5,
    nextUid: 100,
  });
}

describe('playTrackNext(「今の曲が終わったら再生」)', () => {
  beforeEach(() => {
    resetStore();
    recordPlayMock.mockClear();
  });

  it('キューが空のとき: 再生開始扱い(queue=[track], index=0, isPlaying, loadNonce+1)', () => {
    const before = usePlayerStore.getState().loadNonce;
    usePlayerStore.getState().playTrackNext(makeInput('x.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue).toHaveLength(1);
    expect(s.queue[0]).toMatchObject({ workId: 9, workTitle: '追加元作品', name: 'x.mp3', path: 'x.mp3' });
    expect(s.index).toBe(0);
    expect(s.isPlaying).toBe(true);
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.loadNonce).toBe(before + 1);
  });

  it('キューが空のとき: recordPlay が現在トラックの workId/path で記録される', () => {
    usePlayerStore.getState().playTrackNext(makeInput('x.mp3'));
    expect(recordPlayMock).toHaveBeenCalledTimes(1);
    expect(recordPlayMock).toHaveBeenCalledWith(9, 'x.mp3');
  });

  it('キューが空のとき: uid は nextUid から採番され nextUid が加算される', () => {
    usePlayerStore.setState({ nextUid: 42 });
    usePlayerStore.getState().playTrackNext(makeInput('x.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue[0].uid).toBe(42);
    expect(s.nextUid).toBe(43);
  });

  it('キューが非空のとき: index+1 の位置に挿入するのみ(queue=[a,b,c], index=0 → [a,d,b,c])', () => {
    setupQueueOf3();
    usePlayerStore.getState().playTrackNext(makeInput('d.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue.map((t) => t.name)).toEqual(['a.mp3', 'd.mp3', 'b.mp3', 'c.mp3']);
  });

  it('キューが非空のとき: index / isPlaying / loadNonce / currentTime / duration は不変', () => {
    setupQueueOf3();
    usePlayerStore.getState().playTrackNext(makeInput('d.mp3'));
    const s = usePlayerStore.getState();
    expect(s.index).toBe(0);
    expect(s.isPlaying).toBe(true);
    expect(s.loadNonce).toBe(5);
    expect(s.currentTime).toBe(12);
    expect(s.duration).toBe(100);
  });

  it('キューが非空のとき: 履歴は記録されない', () => {
    setupQueueOf3();
    usePlayerStore.getState().playTrackNext(makeInput('d.mp3'));
    expect(recordPlayMock).not.toHaveBeenCalled();
  });

  it('続けて呼ぶと常に「現在の曲の直後」に積まれる([a,d,b,c] → [a,e,d,b,c])', () => {
    setupQueueOf3();
    usePlayerStore.getState().playTrackNext(makeInput('d.mp3'));
    usePlayerStore.getState().playTrackNext(makeInput('e.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue.map((t) => t.name)).toEqual(['a.mp3', 'e.mp3', 'd.mp3', 'b.mp3', 'c.mp3']);
  });

  it('挿入トラックの uid は採番され nextUid が加算される', () => {
    setupQueueOf3(); // nextUid=100
    usePlayerStore.getState().playTrackNext(makeInput('d.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue[1].uid).toBe(100);
    expect(s.nextUid).toBe(101);
  });
});

describe('replaceQueueWith(「キューを置き換えて再生」)', () => {
  beforeEach(() => {
    resetStore();
    recordPlayMock.mockClear();
  });

  it('キューが空のとき: その 1 曲だけにして再生開始扱い', () => {
    const before = usePlayerStore.getState().loadNonce;
    usePlayerStore.getState().replaceQueueWith(makeInput('x.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue).toHaveLength(1);
    expect(s.queue[0]).toMatchObject({ name: 'x.mp3', path: 'x.mp3', workId: 9 });
    expect(s.index).toBe(0);
    expect(s.isPlaying).toBe(true);
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.loadNonce).toBe(before + 1);
  });

  it('キューが非空でも内容・再生状態によらず常に置き換えて再生開始扱い', () => {
    setupQueueOf3(); // index=0, isPlaying=true, currentTime=12, loadNonce=5
    usePlayerStore.getState().replaceQueueWith(makeInput('x.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue.map((t) => t.name)).toEqual(['x.mp3']);
    expect(s.index).toBe(0);
    expect(s.isPlaying).toBe(true);
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.loadNonce).toBe(6); // 5 + 1
  });

  it('recordPlay が新トラックの workId/path で記録される', () => {
    setupQueueOf3();
    usePlayerStore.getState().replaceQueueWith(makeInput('x.mp3'));
    expect(recordPlayMock).toHaveBeenCalledTimes(1);
    expect(recordPlayMock).toHaveBeenCalledWith(9, 'x.mp3');
  });

  it('uid は nextUid から採番され、置換をまたいでも単調増加する', () => {
    setupQueueOf3(); // nextUid=100
    usePlayerStore.getState().replaceQueueWith(makeInput('x.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue[0].uid).toBe(100);
    expect(s.nextUid).toBe(101);
  });
});

describe('appendToQueue(「キューの最後に追加」)', () => {
  beforeEach(() => {
    resetStore();
    recordPlayMock.mockClear();
  });

  it('キューが空のとき: 再生開始扱い', () => {
    const before = usePlayerStore.getState().loadNonce;
    usePlayerStore.getState().appendToQueue(makeInput('x.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue).toHaveLength(1);
    expect(s.queue[0]).toMatchObject({ name: 'x.mp3', path: 'x.mp3', workId: 9 });
    expect(s.index).toBe(0);
    expect(s.isPlaying).toBe(true);
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.loadNonce).toBe(before + 1);
  });

  it('キューが空のとき: recordPlay が記録される', () => {
    usePlayerStore.getState().appendToQueue(makeInput('x.mp3'));
    expect(recordPlayMock).toHaveBeenCalledTimes(1);
    expect(recordPlayMock).toHaveBeenCalledWith(9, 'x.mp3');
  });

  it('キューが非空のとき: 末尾に push のみ', () => {
    setupQueueOf3();
    usePlayerStore.getState().appendToQueue(makeInput('d.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue.map((t) => t.name)).toEqual(['a.mp3', 'b.mp3', 'c.mp3', 'd.mp3']);
  });

  it('キューが非空のとき: index / isPlaying / loadNonce / currentTime / duration は不変', () => {
    setupQueueOf3();
    usePlayerStore.getState().appendToQueue(makeInput('d.mp3'));
    const s = usePlayerStore.getState();
    expect(s.index).toBe(0);
    expect(s.isPlaying).toBe(true);
    expect(s.loadNonce).toBe(5);
    expect(s.currentTime).toBe(12);
    expect(s.duration).toBe(100);
  });

  it('キューが非空のとき: 履歴は記録されない', () => {
    setupQueueOf3();
    usePlayerStore.getState().appendToQueue(makeInput('d.mp3'));
    expect(recordPlayMock).not.toHaveBeenCalled();
  });

  it('末尾トラックの uid は採番され nextUid が加算される', () => {
    setupQueueOf3(); // nextUid=100
    usePlayerStore.getState().appendToQueue(makeInput('d.mp3'));
    const s = usePlayerStore.getState();
    expect(s.queue[3].uid).toBe(100);
    expect(s.nextUid).toBe(101);
  });
});

describe('moveInQueue', () => {
  beforeEach(() => {
    resetStore();
    setupQueueOf3();
    recordPlayMock.mockClear();
  });

  it('from が範囲外のとき: 何もしない', () => {
    usePlayerStore.getState().moveInQueue(-1, 1);
    expect(usePlayerStore.getState().queue.map((t) => t.name)).toEqual(['a.mp3', 'b.mp3', 'c.mp3']);
    usePlayerStore.getState().moveInQueue(3, 1);
    expect(usePlayerStore.getState().queue.map((t) => t.name)).toEqual(['a.mp3', 'b.mp3', 'c.mp3']);
  });

  it('to が範囲外のとき: 何もしない', () => {
    usePlayerStore.getState().moveInQueue(0, -1);
    expect(usePlayerStore.getState().queue.map((t) => t.name)).toEqual(['a.mp3', 'b.mp3', 'c.mp3']);
    usePlayerStore.getState().moveInQueue(0, 3);
    expect(usePlayerStore.getState().queue.map((t) => t.name)).toEqual(['a.mp3', 'b.mp3', 'c.mp3']);
  });

  it('from===to のとき: 何もしない', () => {
    usePlayerStore.getState().moveInQueue(1, 1);
    expect(usePlayerStore.getState().queue.map((t) => t.name)).toEqual(['a.mp3', 'b.mp3', 'c.mp3']);
  });

  it('splice で移動する([a,b,c] の 0→2 → [b,c,a])', () => {
    usePlayerStore.getState().moveInQueue(0, 2);
    expect(usePlayerStore.getState().queue.map((t) => t.name)).toEqual(['b.mp3', 'c.mp3', 'a.mp3']);
  });

  it('from===index のとき: index=to に追従する', () => {
    // index=0(a 再生中)。a を末尾へ
    usePlayerStore.getState().moveInQueue(0, 2);
    expect(usePlayerStore.getState().index).toBe(2);
  });

  it('from<index かつ to>=index のとき: index-1', () => {
    usePlayerStore.setState({ index: 1 }); // b 再生中
    usePlayerStore.getState().moveInQueue(0, 2); // a を b より後ろへ
    expect(usePlayerStore.getState().index).toBe(0);
  });

  it('from>index かつ to<=index のとき: index+1', () => {
    usePlayerStore.setState({ index: 1 }); // b 再生中
    usePlayerStore.getState().moveInQueue(2, 0); // c を先頭へ
    expect(usePlayerStore.getState().index).toBe(2);
  });

  it('上記いずれにも当てはまらない移動では index は変わらない', () => {
    usePlayerStore.setState({ index: 0 }); // a 再生中
    usePlayerStore.getState().moveInQueue(1, 2); // b と c の入れ替え(index 0 に無関係)
    expect(usePlayerStore.getState().index).toBe(0);
  });

  it('再生は中断しない(loadNonce / currentTime / duration / isPlaying 不変)', () => {
    usePlayerStore.getState().moveInQueue(0, 2);
    const s = usePlayerStore.getState();
    expect(s.loadNonce).toBe(5);
    expect(s.currentTime).toBe(12);
    expect(s.duration).toBe(100);
    expect(s.isPlaying).toBe(true);
  });

  it('履歴は記録されない', () => {
    usePlayerStore.getState().moveInQueue(0, 2);
    expect(recordPlayMock).not.toHaveBeenCalled();
  });
});

describe('removeFromQueue', () => {
  beforeEach(() => {
    resetStore();
    setupQueueOf3();
    recordPlayMock.mockClear();
  });

  it('範囲外のとき: 何もしない', () => {
    usePlayerStore.getState().removeFromQueue(-1);
    expect(usePlayerStore.getState().queue).toHaveLength(3);
    usePlayerStore.getState().removeFromQueue(3);
    expect(usePlayerStore.getState().queue).toHaveLength(3);
  });

  it('i < index のとき: 除去して index-1(再生継続、他は不変)', () => {
    usePlayerStore.setState({ index: 2 }); // c 再生中
    usePlayerStore.getState().removeFromQueue(0); // a を除去
    const s = usePlayerStore.getState();
    expect(s.queue.map((t) => t.name)).toEqual(['b.mp3', 'c.mp3']);
    expect(s.index).toBe(1); // 2 - 1
    expect(s.isPlaying).toBe(true);
    expect(s.loadNonce).toBe(5); // 不変(再生継続)
    expect(s.currentTime).toBe(12);
    expect(s.duration).toBe(100);
  });

  it('i < index のとき: 履歴は記録されない', () => {
    usePlayerStore.setState({ index: 2 });
    usePlayerStore.getState().removeFromQueue(0);
    expect(recordPlayMock).not.toHaveBeenCalled();
  });

  it('i > index のとき: 除去のみ(index・再生状態は不変)', () => {
    usePlayerStore.setState({ index: 0 }); // a 再生中
    usePlayerStore.getState().removeFromQueue(2); // c を除去
    const s = usePlayerStore.getState();
    expect(s.queue.map((t) => t.name)).toEqual(['a.mp3', 'b.mp3']);
    expect(s.index).toBe(0);
    expect(s.isPlaying).toBe(true);
    expect(s.loadNonce).toBe(5);
    expect(s.currentTime).toBe(12);
    expect(s.duration).toBe(100);
    expect(recordPlayMock).not.toHaveBeenCalled();
  });

  it('i===index で除去後 0 件: 全クリアして停止する', () => {
    usePlayerStore.setState({ queue: [{ uid: 1, workId: 1, workTitle: '作品A', name: 'only.mp3', path: 'only.mp3' }], index: 0, loadNonce: 7 });
    usePlayerStore.getState().removeFromQueue(0);
    const s = usePlayerStore.getState();
    expect(s.queue).toEqual([]);
    expect(s.index).toBe(-1);
    expect(s.isPlaying).toBe(false);
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.loadNonce).toBe(7); // loadNonce は不変
  });

  it('i===index で除去後 0 件: 履歴は記録されない', () => {
    usePlayerStore.setState({ queue: [{ uid: 1, workId: 1, workTitle: '作品A', name: 'only.mp3', path: 'only.mp3' }], index: 0 });
    usePlayerStore.getState().removeFromQueue(0);
    expect(recordPlayMock).not.toHaveBeenCalled();
  });

  it('i===index で他にあり(中間を除去): index 据え置きで次トラックをロード', () => {
    usePlayerStore.setState({ index: 1 }); // b 再生中
    usePlayerStore.getState().removeFromQueue(1); // b を除去 → [a,c]
    const s = usePlayerStore.getState();
    expect(s.queue.map((t) => t.name)).toEqual(['a.mp3', 'c.mp3']);
    // index = Math.min(1, 2-1) = 1 → c
    expect(s.index).toBe(1);
    expect(s.queue[s.index].name).toBe('c.mp3');
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.loadNonce).toBe(6); // 5 + 1
    expect(s.isPlaying).toBe(true); // 維持
  });

  it('i===index で末尾を除去: 新しい末尾へ index が下がる', () => {
    usePlayerStore.setState({ index: 2 }); // c(末尾)再生中
    usePlayerStore.getState().removeFromQueue(2); // c を除去 → [a,b]
    const s = usePlayerStore.getState();
    expect(s.queue.map((t) => t.name)).toEqual(['a.mp3', 'b.mp3']);
    // index = Math.min(2, 2-1) = 1 → b
    expect(s.index).toBe(1);
    expect(s.queue[s.index].name).toBe('b.mp3');
    expect(s.currentTime).toBe(0);
    expect(s.duration).toBe(0);
    expect(s.loadNonce).toBe(6);
    expect(s.isPlaying).toBe(true);
  });

  it('i===index で他にあり: isPlaying=false なら停止状態のまま次をロード', () => {
    usePlayerStore.setState({ index: 1, isPlaying: false });
    usePlayerStore.getState().removeFromQueue(1);
    expect(usePlayerStore.getState().isPlaying).toBe(false);
  });

  it('i===index で他にあり: recordPlay が新トラックの workId/path で記録される', () => {
    usePlayerStore.setState({ index: 1 }); // b 再生中 → 除去後 c
    usePlayerStore.getState().removeFromQueue(1);
    expect(recordPlayMock).toHaveBeenCalledTimes(1);
    expect(recordPlayMock).toHaveBeenCalledWith(1, 'c.mp3');
  });

  it('同一トラック(同 workId/path)の重複は uid で区別され、指定 index のみ除去される', () => {
    usePlayerStore.setState({
      queue: [
        { uid: 1, workId: 1, workTitle: '作品A', name: 'dup.mp3', path: 'dup.mp3' },
        { uid: 2, workId: 1, workTitle: '作品A', name: 'dup.mp3', path: 'dup.mp3' },
      ],
      index: 0,
      isPlaying: true,
    });
    usePlayerStore.getState().removeFromQueue(1); // i > index
    const s = usePlayerStore.getState();
    expect(s.queue).toHaveLength(1);
    expect(s.queue[0].uid).toBe(1);
  });
});

describe('playIndex', () => {
  beforeEach(() => {
    resetStore();
    usePlayerStore.setState({
      queue: [
        { uid: 1, workId: 1, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' },
        { uid: 2, workId: 1, workTitle: 'テスト', name: 'b.mp3', path: 'b.mp3' },
        { uid: 3, workId: 1, workTitle: 'テスト', name: 'c.mp3', path: 'c.mp3' },
      ],
      index: 0,
      nextUid: 100,
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
      queue: [
        { uid: 1, workId: 1, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' },
        { uid: 2, workId: 1, workTitle: 'テスト', name: 'b.mp3', path: 'b.mp3' },
        { uid: 3, workId: 1, workTitle: 'テスト', name: 'c.mp3', path: 'c.mp3' },
      ],
      index: 1,
      currentTime: 0,
      nextUid: 100,
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
      queue: [
        { uid: 1, workId: 1, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' },
        { uid: 2, workId: 1, workTitle: 'テスト', name: 'b.mp3', path: 'b.mp3' },
      ],
      index: 0,
      isPlaying: true,
      nextUid: 100,
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
  it('track が null の場合は null を返す', () => {
    expect(playerThumbUrl(null)).toBeNull();
  });

  it('track がある場合はサムネ URL を返す', () => {
    const track = { uid: 1, workId: 5, workTitle: 'テスト', name: 'a.mp3', path: 'a.mp3' };
    expect(playerThumbUrl(track)).toBe('/api/works/5/thumbnail');
  });
});

// フルスクリーン表示の開閉は store ではなく history(usePlayerOverlay)が担う。
// 開閉まわりのテストは usePlayerOverlay.test.tsx を参照。

describe('volume / 音量', () => {
  beforeEach(resetStore);

  it('初期値は 1', () => {
    expect(usePlayerStore.getState().volume).toBe(1);
  });

  it('setVolume で通常値を設定できる', () => {
    usePlayerStore.getState().setVolume(0.5);
    expect(usePlayerStore.getState().volume).toBe(0.5);
  });

  it('setVolume(-0.5) → 0 にクランプされる', () => {
    usePlayerStore.getState().setVolume(-0.5);
    expect(usePlayerStore.getState().volume).toBe(0);
  });

  it('setVolume(1.5) → 1 にクランプされる', () => {
    usePlayerStore.getState().setVolume(1.5);
    expect(usePlayerStore.getState().volume).toBe(1);
  });

  it('setVolume(0) → 0 が設定できる(ミュート)', () => {
    usePlayerStore.getState().setVolume(0);
    expect(usePlayerStore.getState().volume).toBe(0);
  });

  it('setVolume(1) → 1 が設定できる(最大)', () => {
    usePlayerStore.getState().setVolume(1);
    expect(usePlayerStore.getState().volume).toBe(1);
  });
});

describe('永続化(persist middleware)', () => {
  // このブロック専用で localStorage をリセット。他テストへの汚染を防ぐ。
  beforeEach(() => {
    localStorage.clear();
    resetStore();
  });

  it('setRate を呼ぶと localStorage の stashpad-player に playbackRate が反映される', () => {
    usePlayerStore.getState().setRate(1.5);
    const stored = JSON.parse(localStorage.getItem('stashpad-player')!);
    expect(stored.state.playbackRate).toBe(1.5);
  });

  it('setVolume を呼ぶと localStorage の stashpad-player に volume が反映される', () => {
    usePlayerStore.getState().setVolume(0.7);
    const stored = JSON.parse(localStorage.getItem('stashpad-player')!);
    expect(stored.state.volume).toBe(0.7);
  });

  it('queue / currentTime / isPlaying / index などの揮発 state は localStorage に書き込まれない(partialize の検証)', () => {
    usePlayerStore.getState().setRate(1.25);
    const stored = JSON.parse(localStorage.getItem('stashpad-player')!);
    // partialize で絞り込んだ state には playbackRate と volume のみ存在する
    expect(Object.keys(stored.state)).toEqual(['playbackRate', 'volume']);
  });
});
