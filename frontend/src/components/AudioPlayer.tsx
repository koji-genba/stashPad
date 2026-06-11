// 画面下部固定のミニプレイヤー。<audio> 要素を ref で保持し、
// ページ遷移しても unmount されない(App ルート直下に常駐)。
// 再生状態は playerStore が単一の真実。ここは store の命令(loadNonce / seekRequest /
// isPlaying / playbackRate / volume)を <audio> に反映し、<audio> のイベントを store へ返す。
// ミニプレイヤーのサムネ/メタ領域タップ、または上方向スワイプで FullscreenPlayer を開く。
import { useEffect, useRef } from 'react';
import { useStore } from 'zustand';
import {
  currentSrc,
  currentTrack,
  PLAYBACK_RATES,
  playerThumbUrl,
  usePlayerStore,
} from '@/store/playerStore';
import { formatTime } from '@/utils/format';
import FullscreenPlayer from './FullscreenPlayer';
import styles from './AudioPlayer.module.css';

export default function AudioPlayer() {
  const audioRef = useRef<HTMLAudioElement>(null);
  // ミニバー上方向スワイプ検知用 ref
  const barTouchStart = useRef<{ x: number; y: number } | null>(null);

  const index = useStore(usePlayerStore, (s) => s.index);
  const queueLen = useStore(usePlayerStore, (s) => s.queue.length);
  const isPlaying = useStore(usePlayerStore, (s) => s.isPlaying);
  const currentTime = useStore(usePlayerStore, (s) => s.currentTime);
  const duration = useStore(usePlayerStore, (s) => s.duration);
  const playbackRate = useStore(usePlayerStore, (s) => s.playbackRate);
  const volume = useStore(usePlayerStore, (s) => s.volume);
  const loadNonce = useStore(usePlayerStore, (s) => s.loadNonce);
  const seekRequest = useStore(usePlayerStore, (s) => s.seekRequest);
  const src = useStore(usePlayerStore, currentSrc);
  const track = useStore(usePlayerStore, currentTrack);

  // ---- store -> <audio>: 新トラックのロード ----
  useEffect(() => {
    const el = audioRef.current;
    if (!el || !src) return;
    if (el.src !== absoluteUrl(src)) {
      el.src = src;
    }
    el.playbackRate = playbackRate;
    el.load();
    void el.play().catch(() => {
      // 自動再生がブロックされたら停止状態にする
      usePlayerStore.getState().setPlaying(false);
    });
    // loadNonce が変わるたびに発火
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loadNonce, src]);

  // ---- store -> <audio>: 再生 / 停止 ----
  useEffect(() => {
    const el = audioRef.current;
    if (!el || !src) return;
    if (isPlaying && el.paused) {
      void el.play().catch(() => usePlayerStore.getState().setPlaying(false));
    } else if (!isPlaying && !el.paused) {
      el.pause();
    }
  }, [isPlaying, src]);

  // ---- store -> <audio>: 再生速度 ----
  useEffect(() => {
    const el = audioRef.current;
    if (el) el.playbackRate = playbackRate;
  }, [playbackRate]);

  // ---- store -> <audio>: 音量 ----
  useEffect(() => {
    const el = audioRef.current;
    if (el) el.volume = volume;
  }, [volume]);

  // ---- store -> <audio>: シーク要求 ----
  useEffect(() => {
    const el = audioRef.current;
    if (el && seekRequest) {
      el.currentTime = seekRequest.time;
    }
  }, [seekRequest]);

  // ---- Media Session API ----
  useEffect(() => {
    if (!('mediaSession' in navigator) || !track) return;
    const ms = navigator.mediaSession;
    const thumb = playerThumbUrl(track);
    ms.metadata = new MediaMetadata({
      title: track.name,
      artist: track.workTitle,
      album: track.workTitle,
      artwork: thumb
        ? [{ src: absoluteUrl(thumb), sizes: '512x512', type: 'image/jpeg' }]
        : [],
    });
    const store = usePlayerStore.getState();
    const set = (action: MediaSessionAction, handler: (() => void) | null) => {
      try {
        ms.setActionHandler(action, handler);
      } catch {
        /* 未対応アクションは無視 */
      }
    };
    set('play', () => store.setPlaying(true));
    set('pause', () => store.setPlaying(false));
    set('seekbackward', () => store.seekBy(-10));
    set('seekforward', () => store.seekBy(10));
    set('previoustrack', () => usePlayerStore.getState().prev());
    set('nexttrack', () => usePlayerStore.getState().next());
    return () => {
      (
        [
          'play',
          'pause',
          'seekbackward',
          'seekforward',
          'previoustrack',
          'nexttrack',
        ] as MediaSessionAction[]
      ).forEach((a) => set(a, null));
    };
  }, [track]);

  useEffect(() => {
    if ('mediaSession' in navigator) {
      navigator.mediaSession.playbackState = isPlaying ? 'playing' : 'paused';
    }
  }, [isPlaying]);

  if (!track) return null;

  const store = usePlayerStore.getState();
  const onSeek = (e: React.ChangeEvent<HTMLInputElement>) => {
    store.seekTo(Number(e.target.value));
  };

  // ミニバー上方向スワイプでフルスクリーンを開く
  const onBarTouchStart = (e: React.TouchEvent) => {
    barTouchStart.current = { x: e.touches[0].clientX, y: e.touches[0].clientY };
  };
  const onBarTouchEnd = (e: React.TouchEvent) => {
    if (!barTouchStart.current) return;
    const dx = e.changedTouches[0].clientX - barTouchStart.current.x;
    const dy = e.changedTouches[0].clientY - barTouchStart.current.y;
    barTouchStart.current = null;
    // 縦移動 60px 超(上方向)かつ縦優位
    if (dy < -60 && Math.abs(dy) > Math.abs(dx)) {
      store.setExpanded(true);
    }
  };

  return (
    <>
      <audio
        ref={audioRef}
        onTimeUpdate={(e) =>
          usePlayerStore.getState().setCurrentTime(e.currentTarget.currentTime)
        }
        onLoadedMetadata={(e) =>
          usePlayerStore.getState().setDuration(e.currentTarget.duration)
        }
        onDurationChange={(e) =>
          usePlayerStore.getState().setDuration(e.currentTarget.duration)
        }
        onPlay={() => usePlayerStore.getState().setPlaying(true)}
        onPause={() => usePlayerStore.getState().setPlaying(false)}
        onEnded={() => usePlayerStore.getState().handleEnded()}
      />

      {/* フルスクリーンプレイヤー(expanded=true のときオーバーレイで表示) */}
      <FullscreenPlayer />

      <div
        className={styles.bar}
        onTouchStart={onBarTouchStart}
        onTouchEnd={onBarTouchEnd}
      >
        <div
          className={styles.seek}
          style={{
            background: `linear-gradient(to right, var(--accent) ${
              duration ? (currentTime / duration) * 100 : 0
            }%, var(--border) 0)`,
          }}
        >
          <input
            type="range"
            min={0}
            max={duration || 0}
            step={0.1}
            value={Math.min(currentTime, duration || 0)}
            onChange={onSeek}
            aria-label="再生位置"
          />
        </div>

        <div className={styles.body}>
          {/* サムネ/メタ領域タップでフルスクリーンを開く */}
          <button
            type="button"
            className={styles.nav}
            onClick={() => store.setExpanded(true)}
            aria-label="フルスクリーンプレイヤーを開く"
          >
            <img
              className={styles.thumb}
              src={playerThumbUrl(track) ?? ''}
              alt=""
              onError={(e) => {
                e.currentTarget.style.visibility = 'hidden';
              }}
            />
            <div className={styles.meta}>
              <div className={styles.trackName} title={track.name}>
                {track.name}
              </div>
              <div className={styles.workName} title={track.workTitle}>
                {track.workTitle}
              </div>
            </div>
          </button>

          <div className={styles.controls}>
            <button
              className={styles.ctrl}
              onClick={() => store.prev()}
              disabled={queueLen <= 1}
              aria-label="前のトラック"
            >
              ⏮
            </button>
            <button
              className={styles.ctrl}
              onClick={() => store.seekBy(-10)}
              aria-label="10秒戻る"
            >
              <span className={styles.skipLabel}>-10</span>
            </button>
            <button
              className={styles.play}
              onClick={() => store.togglePlay()}
              aria-label={isPlaying ? '一時停止' : '再生'}
            >
              {isPlaying ? '❚❚' : '▶'}
            </button>
            <button
              className={styles.ctrl}
              onClick={() => store.seekBy(10)}
              aria-label="10秒進む"
            >
              <span className={styles.skipLabel}>+10</span>
            </button>
            <button
              className={styles.ctrl}
              onClick={() => store.next()}
              disabled={index + 1 >= queueLen}
              aria-label="次のトラック"
            >
              ⏭
            </button>
          </div>

          <div className={styles.right}>
            <span className={styles.time}>
              {formatTime(currentTime)} / {formatTime(duration)}
            </span>
            <select
              className={styles.rate}
              value={playbackRate}
              onChange={(e) => store.setRate(Number(e.target.value))}
              aria-label="再生速度"
            >
              {PLAYBACK_RATES.map((r) => (
                <option key={r} value={r}>
                  {r}x
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>
    </>
  );
}

function absoluteUrl(path: string): string {
  if (/^https?:\/\//.test(path)) return path;
  return new URL(path, window.location.origin).href;
}
