// フルスクリーンオーディオプレイヤー。
// ミニプレイヤーバーをタップ(setExpanded(true))したときに表示される全画面オーバーレイ。
// 大きなアートワーク・トランスポート・スキップ・速度/音量スライダー・キュー一覧を持つ。
import { useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useStore } from 'zustand';
import {
  currentTrack,
  PLAYBACK_RATES,
  playerThumbUrl,
  usePlayerStore,
} from '@/store/playerStore';
import { formatTime } from '@/utils/format';
import styles from './FullscreenPlayer.module.css';

export default function FullscreenPlayer() {
  const navigate = useNavigate();

  const ctx = useStore(usePlayerStore, (s) => s.ctx);
  const queue = useStore(usePlayerStore, (s) => s.queue);
  const index = useStore(usePlayerStore, (s) => s.index);
  const isPlaying = useStore(usePlayerStore, (s) => s.isPlaying);
  const currentTime = useStore(usePlayerStore, (s) => s.currentTime);
  const duration = useStore(usePlayerStore, (s) => s.duration);
  const playbackRate = useStore(usePlayerStore, (s) => s.playbackRate);
  const volume = useStore(usePlayerStore, (s) => s.volume);
  const expanded = useStore(usePlayerStore, (s) => s.expanded);
  const track = useStore(usePlayerStore, currentTrack);

  // キュー内の現在再生行を自動スクロール
  const currentQueueRef = useRef<HTMLButtonElement | null>(null);
  useEffect(() => {
    if (!expanded) return;
    // scrollIntoView が未対応の環境(テスト環境)ではガード
    if (currentQueueRef.current && typeof currentQueueRef.current.scrollIntoView === 'function') {
      currentQueueRef.current.scrollIntoView({ block: 'nearest' });
    }
  }, [expanded, index]);

  // Escape キーで閉じる
  useEffect(() => {
    if (!expanded) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        usePlayerStore.getState().setExpanded(false);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [expanded]);

  // body スクロールロック(ImageViewer と同じイディオム)
  useEffect(() => {
    if (!expanded) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = prev;
    };
  }, [expanded]);

  // スワイプで閉じる(ヘッダ〜アートワーク領域に touchstart/touchend を設置)
  const touchStartX = useRef<number | null>(null);
  const touchStartY = useRef<number | null>(null);

  const onTouchStart = (e: React.TouchEvent) => {
    touchStartX.current = e.touches[0].clientX;
    touchStartY.current = e.touches[0].clientY;
  };
  const onTouchEnd = (e: React.TouchEvent) => {
    if (touchStartX.current === null || touchStartY.current === null) return;
    const dx = e.changedTouches[0].clientX - touchStartX.current;
    const dy = e.changedTouches[0].clientY - touchStartY.current;
    touchStartX.current = null;
    touchStartY.current = null;
    // 縦移動 60px 超かつ縦優位の下方向スワイプで閉じる
    if (dy > 60 && Math.abs(dy) > Math.abs(dx)) {
      usePlayerStore.getState().setExpanded(false);
    }
  };

  if (!expanded || !ctx) return null;

  const store = usePlayerStore.getState();
  const thumbUrl = playerThumbUrl(ctx);
  // シークバーの進捗表示用パーセント
  const progressPct = duration ? (currentTime / duration) * 100 : 0;

  const onSeek = (e: React.ChangeEvent<HTMLInputElement>) => {
    store.seekTo(Number(e.target.value));
  };

  const onVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    store.setVolume(Number(e.target.value));
  };

  return (
    <div className={styles.overlay}>
      {/* ヘッダ: 閉じるボタン + 作品タイトルボタン */}
      <div
        className={styles.header}
        onTouchStart={onTouchStart}
        onTouchEnd={onTouchEnd}
      >
        <button
          type="button"
          className={styles.closeBtn}
          onClick={() => store.setExpanded(false)}
          aria-label="ミニプレイヤーに戻る"
        >
          ⌄
        </button>
        <button
          type="button"
          className={styles.titleBtn}
          onClick={() => {
            navigate(`/works/${ctx.workId}`);
            store.setExpanded(false);
          }}
          aria-label={`${ctx.workTitle} の作品ページを開く`}
        >
          {ctx.workTitle}
        </button>
        {/* ヘッダの右側の空白スペーサー(閉じるボタンと対称) */}
        <div className={styles.headerSpacer} aria-hidden />
      </div>

      {/* アートワーク */}
      <div
        className={styles.artworkWrap}
        onTouchStart={onTouchStart}
        onTouchEnd={onTouchEnd}
      >
        {thumbUrl && (
          <img
            className={styles.artwork}
            src={thumbUrl}
            alt=""
            onError={(e) => {
              e.currentTarget.style.visibility = 'hidden';
            }}
          />
        )}
      </div>

      {/* トラック名 + 作品タイトル */}
      <div className={styles.trackInfo}>
        <div className={styles.trackName} title={track?.name}>
          {track?.name}
        </div>
        <div className={styles.workTitle} title={ctx.workTitle}>
          {ctx.workTitle}
        </div>
      </div>

      {/* 大シークバー */}
      <div className={styles.seekWrap}>
        <div
          className={styles.seekTrack}
          style={{
            background: `linear-gradient(to right, var(--accent) ${progressPct}%, var(--border) 0)`,
          }}
        >
          {/* つまみ。進捗位置に追従させる(input 自体は opacity:0 で重ねている) */}
          <div
            className={styles.seekThumb}
            style={{ left: `${progressPct}%` }}
            aria-hidden
          />
          <input
            type="range"
            className={styles.seekInput}
            min={0}
            max={duration || 0}
            step={0.1}
            value={Math.min(currentTime, duration || 0)}
            onChange={onSeek}
            aria-label="再生位置"
          />
        </div>
        <div className={styles.timeRow}>
          <span className={styles.time}>{formatTime(currentTime)}</span>
          <span className={styles.time}>{formatTime(duration)}</span>
        </div>
      </div>

      {/* トランスポート行: ⏮ / −10 / 再生 / +10 / ⏭ */}
      <div className={styles.transport}>
        <button
          type="button"
          className={styles.transportBtn}
          onClick={() => store.prev()}
          disabled={queue.length <= 1}
          aria-label="前のトラック"
        >
          ⏮
        </button>
        <button
          type="button"
          className={styles.transportBtn}
          onClick={() => store.seekBy(-10)}
          aria-label="10秒戻る"
        >
          <span className={styles.skipLabel}>-10</span>
        </button>
        <button
          type="button"
          className={styles.playBtn}
          onClick={() => store.togglePlay()}
          aria-label={isPlaying ? '一時停止' : '再生'}
        >
          {isPlaying ? '❚❚' : '▶'}
        </button>
        <button
          type="button"
          className={styles.transportBtn}
          onClick={() => store.seekBy(10)}
          aria-label="10秒進む"
        >
          <span className={styles.skipLabel}>+10</span>
        </button>
        <button
          type="button"
          className={styles.transportBtn}
          onClick={() => store.next()}
          disabled={index + 1 >= queue.length}
          aria-label="次のトラック"
        >
          ⏭
        </button>
      </div>

      {/* スキップ行: −30 / −5 / +5 / +30 */}
      <div className={styles.skipRow}>
        <button
          type="button"
          className={styles.skipBtn}
          onClick={() => store.seekBy(-30)}
          aria-label="30秒戻る"
        >
          <span className={styles.skipLabel}>-30</span>
        </button>
        <button
          type="button"
          className={styles.skipBtn}
          onClick={() => store.seekBy(-5)}
          aria-label="5秒戻る"
        >
          <span className={styles.skipLabel}>-5</span>
        </button>
        <button
          type="button"
          className={styles.skipBtn}
          onClick={() => store.seekBy(5)}
          aria-label="5秒進む"
        >
          <span className={styles.skipLabel}>+5</span>
        </button>
        <button
          type="button"
          className={styles.skipBtn}
          onClick={() => store.seekBy(30)}
          aria-label="30秒進む"
        >
          <span className={styles.skipLabel}>+30</span>
        </button>
      </div>

      {/* 下部コントロール: 再生速度 + 音量 */}
      <div className={styles.bottomControls}>
        <label className={styles.controlLabel}>
          <span className={styles.controlLabelText}>速度</span>
          <select
            className={styles.rateSelect}
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
        </label>
        <label className={styles.controlLabel}>
          <span className={styles.controlLabelText}>音量</span>
          <input
            type="range"
            className={styles.volumeSlider}
            min={0}
            max={1}
            step={0.05}
            value={volume}
            onChange={onVolumeChange}
            aria-label="音量"
          />
        </label>
      </div>

      {/* キュー一覧 */}
      <div className={styles.queueSection}>
        <div className={styles.queueHeader}>
          キュー({index + 1}/{queue.length})
        </div>
        <div className={styles.queueList}>
          {queue.map((t, i) => {
            const isCurrent = i === index;
            return (
              <button
                key={t.path}
                type="button"
                ref={isCurrent ? currentQueueRef : null}
                className={`${styles.queueItem} ${isCurrent ? styles.queueItemCurrent : ''}`}
                onClick={() => store.playIndex(i)}
                aria-label={t.name}
              >
                {isCurrent && <span className={styles.queueNowPlaying} aria-hidden>♪</span>}
                <span className={styles.queueName}>{t.name}</span>
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
