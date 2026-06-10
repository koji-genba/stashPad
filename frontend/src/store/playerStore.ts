// オーディオプレイヤーのグローバル状態(zustand)。
// <audio> 要素自体はルート直下の <AudioPlayer> が ref で保持し続け、
// ページ遷移しても unmount しない。本ストアは「何を・どこまで再生しているか」を持つ。
import { create } from 'zustand';
import type { Entry } from '@/api/types';
import { fileUrl, recordPlay, thumbnailUrl } from '@/api/client';
import { joinPath } from '@/utils/format';

export interface QueueTrack {
  /** 作品ルートからの相対パス(file API / plays API に渡す) */
  path: string;
  name: string;
}

export interface PlayerContext {
  workId: number;
  workTitle: string;
  /** キュー元ディレクトリ(相対パス。空文字=ルート) */
  dir: string;
}

interface PlayerState {
  ctx: PlayerContext | null;
  queue: QueueTrack[];
  index: number;
  isPlaying: boolean;
  currentTime: number;
  duration: number;
  playbackRate: number;

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

  // ---- 以下は AudioPlayer コンポーネントが <audio> を操作するための命令キュー ----
  // 数値を増やすことで「シーク/レート反映/ロードして再生」を要求する。
  seekRequest: { time: number; nonce: number } | null;
  loadNonce: number; // 増えたら現在トラックを load して再生せよ
}

export const currentTrack = (s: PlayerState): QueueTrack | null =>
  s.index >= 0 && s.index < s.queue.length ? s.queue[s.index] : null;

export const currentSrc = (s: PlayerState): string | null => {
  const t = currentTrack(s);
  return s.ctx && t ? fileUrl(s.ctx.workId, t.path) : null;
};

export const usePlayerStore = create<PlayerState>((set, get) => ({
  ctx: null,
  queue: [],
  index: -1,
  isPlaying: false,
  currentTime: 0,
  duration: 0,
  playbackRate: 1,
  seekRequest: null,
  loadNonce: 0,

  startFromEntries: ({ workId, workTitle, dir, entries, startName }) => {
    const queue: QueueTrack[] = entries
      .filter((e) => !e.is_dir && e.media_kind === 'audio')
      .map((e) => ({ name: e.name, path: joinPath(dir, e.name) }));
    const index = queue.findIndex((t) => t.name === startName);
    if (index < 0) return;
    set({
      ctx: { workId, workTitle, dir },
      queue,
      index,
      currentTime: 0,
      duration: 0,
      isPlaying: true,
      loadNonce: get().loadNonce + 1,
    });
    void recordPlayFor(get(), index);
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
    set({ currentTime: time, seekRequest: { time, nonce: get().loadNonce + Date.now() } });
  },

  setRate: (rate) => set({ playbackRate: rate }),
  setCurrentTime: (t) => set({ currentTime: t }),
  setDuration: (d) => set({ duration: d }),

  handleEnded: () => {
    const { index, queue } = get();
    if (index + 1 < queue.length) {
      get().playIndex(index + 1); // 自動送りでも履歴記録
    } else {
      set({ isPlaying: false });
    }
  },
}));

async function recordPlayFor(state: PlayerState, index: number) {
  const t = state.queue[index];
  if (!state.ctx || !t) return;
  try {
    await recordPlay(state.ctx.workId, t.path);
  } catch {
    // 履歴記録の失敗は再生を止めない
  }
}

/** Media Session のサムネ URL を組み立てるためのヘルパ */
export function playerThumbUrl(ctx: PlayerContext | null): string | null {
  return ctx ? thumbnailUrl(ctx.workId) : null;
}
