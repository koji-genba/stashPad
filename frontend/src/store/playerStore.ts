// オーディオプレイヤーのグローバル状態(zustand)。
// <audio> 要素自体はルート直下の <AudioPlayer> が ref で保持し続け、
// ページ遷移しても unmount しない。本ストアは「何を・どこまで再生しているか」を持つ。
import { create } from 'zustand';
import type { Entry } from '@/api/types';
import { fileUrl, recordPlay, thumbnailUrl } from '@/api/client';
import { joinPath } from '@/utils/format';

/** 再生速度の選択肢。AudioPlayer / FullscreenPlayer で共用 */
export const PLAYBACK_RATES = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2] as const;

export interface QueueTrack {
  /** キュー内アイテムの一意 ID(重複追加・並び替えに耐える React key / ドラッグ識別用) */
  uid: number;
  workId: number;
  workTitle: string;
  /** 作品ルートからの相対パス(file API / plays API に渡す) */
  path: string;
  name: string;
}

/** キューに積む際の入力。uid は store が採番する */
export type EnqueueInput = Omit<QueueTrack, 'uid'>;

interface PlayerState {
  queue: QueueTrack[];
  index: number;
  isPlaying: boolean;
  currentTime: number;
  duration: number;
  playbackRate: number;
  /** 音量 0..1。初期値 1 */
  volume: number;
  /** 次に採番する uid。トラックを積むたびに割り当てて加算する(キュー置換をまたいでもリセットしない) */
  nextUid: number;

  /** ディレクトリの entries から audio キューを構築し、指定ファイルから再生開始 */
  startFromEntries: (
    args: {
      workId: number;
      workTitle: string;
      dir: string;
      entries: Entry[];
      startName: string;
    },
  ) => void;

  /** 「今の曲が終わったら再生」: 現在トラックの直後に挿入。空なら再生開始 */
  playTrackNext: (input: EnqueueInput) => void;
  /** 「キューを置き換えて再生」: 常にその 1 曲だけにして再生開始 */
  replaceQueueWith: (input: EnqueueInput) => void;
  /** 「キューの最後に追加」: 末尾に push。空なら再生開始 */
  appendToQueue: (input: EnqueueInput) => void;
  /** キュー内のアイテムを from から to へ移動(再生は中断しない) */
  moveInQueue: (from: number, to: number) => void;
  /** キューから i 番目を除去 */
  removeFromQueue: (i: number) => void;

  /** index のトラックを再生(キュー内移動・自動送りで使用) */
  playIndex: (index: number, opts?: { record?: boolean }) => void;
  next: () => void;
  prev: () => void;
  togglePlay: () => void;
  setPlaying: (playing: boolean) => void;
  seekBy: (delta: number) => void;
  seekTo: (time: number) => void;
  setRate: (rate: number) => void;
  setCurrentTime: (t: number) => void;
  setDuration: (d: number) => void;
  /** トラック終了時の自動送り。次が無ければ停止 */
  handleEnded: () => void;
  /** 音量を [0, 1] にクランプして設定 */
  setVolume: (v: number) => void;

  // ---- 以下は AudioPlayer コンポーネントが <audio> を操作するための命令キュー ----
  // 数値を増やすことで「シーク/レート反映/ロードして再生」を要求する。
  seekRequest: { time: number; nonce: number } | null;
  loadNonce: number; // 増えたら現在トラックを load して再生せよ
}

export const currentTrack = (s: PlayerState): QueueTrack | null =>
  s.index >= 0 && s.index < s.queue.length ? s.queue[s.index] : null;

export const currentSrc = (s: PlayerState): string | null => {
  const t = currentTrack(s);
  return t ? fileUrl(t.workId, t.path) : null;
};

export const usePlayerStore = create<PlayerState>((set, get) => ({
  queue: [],
  index: -1,
  isPlaying: false,
  currentTime: 0,
  duration: 0,
  playbackRate: 1,
  volume: 1,
  nextUid: 1,
  seekRequest: null,
  loadNonce: 0,

  startFromEntries: ({ workId, workTitle, dir, entries, startName }) => {
    const audio = entries.filter((e) => !e.is_dir && e.media_kind === 'audio');
    const startIdx = audio.findIndex((e) => e.name === startName);
    if (startIdx < 0) return;
    let uid = get().nextUid;
    const queue: QueueTrack[] = audio.map((e) => ({
      uid: uid++,
      workId,
      workTitle,
      name: e.name,
      path: joinPath(dir, e.name),
    }));
    set({
      queue,
      index: startIdx,
      nextUid: uid,
      currentTime: 0,
      duration: 0,
      isPlaying: true,
      loadNonce: get().loadNonce + 1,
    });
    void recordPlayFor(get(), startIdx);
  },

  playTrackNext: (input) => {
    const { queue, index, nextUid } = get();
    const track: QueueTrack = { uid: nextUid, ...input };
    if (queue.length === 0) {
      startSingle(set, get, track);
      return;
    }
    // 現在トラックの直後に挿入するのみ(再生状態は不変・履歴記録なし)
    const next = queue.slice();
    next.splice(index + 1, 0, track);
    set({ queue: next, nextUid: nextUid + 1 });
  },

  replaceQueueWith: (input) => {
    const { nextUid } = get();
    // 既存キュー・再生状態によらず常に置き換えて再生開始扱い
    startSingle(set, get, { uid: nextUid, ...input });
  },

  appendToQueue: (input) => {
    const { queue, nextUid } = get();
    const track: QueueTrack = { uid: nextUid, ...input };
    if (queue.length === 0) {
      startSingle(set, get, track);
      return;
    }
    // 末尾に push のみ(再生状態は不変・履歴記録なし)
    set({ queue: [...queue, track], nextUid: nextUid + 1 });
  },

  moveInQueue: (from, to) => {
    const { queue, index } = get();
    const len = queue.length;
    if (from < 0 || from >= len || to < 0 || to >= len || from === to) return;
    const next = queue.slice();
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    // 再生中トラックの index 追従(再生は中断しない)
    let nextIndex = index;
    if (from === index) nextIndex = to;
    else if (from < index && to >= index) nextIndex = index - 1;
    else if (from > index && to <= index) nextIndex = index + 1;
    set({ queue: next, index: nextIndex });
  },

  removeFromQueue: (i) => {
    const { queue, index } = get();
    if (i < 0 || i >= queue.length) return;
    const next = queue.slice();
    next.splice(i, 1);

    if (i < index) {
      // 再生中より前を除去 → index を 1 詰めるだけ(再生継続)
      set({ queue: next, index: index - 1 });
      return;
    }
    if (i > index) {
      // 再生中より後を除去 → 除去のみ
      set({ queue: next });
      return;
    }
    // i === index(再生中トラックを除去)
    if (next.length === 0) {
      // 全クリア(フルスクリーン表示の後始末は history 側 = usePlayerOverlay の unwind が担う)
      set({
        queue: [],
        index: -1,
        isPlaying: false,
        currentTime: 0,
        duration: 0,
      });
      return;
    }
    // 他にトラックあり → handleEnded の自動送りと同等(次トラックをロード)
    const newIndex = Math.min(i, next.length - 1);
    set({
      queue: next,
      index: newIndex,
      currentTime: 0,
      duration: 0,
      loadNonce: get().loadNonce + 1,
    });
    void recordPlayFor(get(), newIndex);
  },

  playIndex: (index, opts) => {
    const { queue } = get();
    if (index < 0 || index >= queue.length) return;
    set({
      index,
      currentTime: 0,
      duration: 0,
      isPlaying: true,
      loadNonce: get().loadNonce + 1,
    });
    if (opts?.record !== false) void recordPlayFor(get(), index);
  },

  next: () => {
    const { index, queue } = get();
    if (index + 1 < queue.length) get().playIndex(index + 1);
  },

  prev: () => {
    const { index, currentTime } = get();
    // 3 秒以上経過していたら先頭に戻す、それ以外は前トラックへ
    if (currentTime > 3) {
      get().seekTo(0);
      return;
    }
    if (index - 1 >= 0) get().playIndex(index - 1);
  },

  togglePlay: () => set((s) => ({ isPlaying: !s.isPlaying })),
  setPlaying: (playing) => set({ isPlaying: playing }),

  seekBy: (delta) => {
    const { currentTime, duration } = get();
    const target = Math.max(0, Math.min(duration || Infinity, currentTime + delta));
    get().seekTo(target);
  },

  seekTo: (time) => {
    // nonce はシーク要求ごとの単調増加カウンタ。同じ time への連続シークでも
    // AudioPlayer 側が新しい要求として区別できるようにする
    set((s) => ({
      currentTime: time,
      seekRequest: { time, nonce: (s.seekRequest?.nonce ?? 0) + 1 },
    }));
  },

  setRate: (rate) => set({ playbackRate: rate }),
  setCurrentTime: (t) => set({ currentTime: t }),
  setDuration: (d) => set({ duration: d }),
  setVolume: (v) => set({ volume: Math.max(0, Math.min(1, v)) }),

  handleEnded: () => {
    const { index, queue } = get();
    if (index + 1 < queue.length) {
      get().playIndex(index + 1); // 自動送りでも履歴記録
    } else {
      set({ isPlaying: false });
    }
  },
}));

// zustand の set / get の型エイリアス(ヘルパで使う)
type SetState = (partial: Partial<PlayerState>) => void;
type GetState = () => PlayerState;

/**
 * 共通「再生開始扱い」: キューを単一トラックに置き換えて先頭から再生開始する。
 * queue=[track], index=0, isPlaying=true, currentTime=0, duration=0, loadNonce+1。
 * uid 採番のため nextUid を加算し、recordPlay を記録する(startFromEntries と同じ扱い)。
 */
function startSingle(set: SetState, get: GetState, track: QueueTrack) {
  set({
    queue: [track],
    index: 0,
    nextUid: get().nextUid + 1,
    currentTime: 0,
    duration: 0,
    isPlaying: true,
    loadNonce: get().loadNonce + 1,
  });
  void recordPlayFor(get(), 0);
}

async function recordPlayFor(state: PlayerState, index: number) {
  const t = state.queue[index];
  if (!t) return;
  try {
    await recordPlay(t.workId, t.path);
  } catch {
    // 履歴記録の失敗は再生を止めない
  }
}

/** Media Session のサムネ URL を組み立てるためのヘルパ */
export function playerThumbUrl(track: QueueTrack | null): string | null {
  return track ? thumbnailUrl(track.workId) : null;
}
