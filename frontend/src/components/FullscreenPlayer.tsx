// フルスクリーンオーディオプレイヤー。
// ミニプレイヤーバーをタップ(usePlayerOverlay.openPlayer)したときに表示される
// 全画面オーバーレイ。表示状態は history(location.state)が持ち、Android の
// 「戻る」やスワイプ・Escape で 1 段ずつ閉じる。
// 大きなアートワーク・トランスポート・スキップ・速度/音量スライダーを持つ。
import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useStore } from 'zustand';
import {
  currentTrack,
  PLAYBACK_RATES,
  playerThumbUrl,
  SLEEP_PRESETS_MIN,
  usePlayerStore,
} from '@/store/playerStore';
import { usePlayerOverlay } from '@/hooks/usePlayerOverlay';
import { useBodyScrollLock } from '@/hooks/useBodyScrollLock';
import { formatTime } from '@/utils/format';
import QueueScreen from './QueueScreen';
import Thumbnail from './Thumbnail';
import styles from './FullscreenPlayer.module.css';

export default function FullscreenPlayer() {
  const navigate = useNavigate();
  const overlay = usePlayerOverlay();

  const queue = useStore(usePlayerStore, (s) => s.queue);
  const index = useStore(usePlayerStore, (s) => s.index);
  const isPlaying = useStore(usePlayerStore, (s) => s.isPlaying);
  const currentTime = useStore(usePlayerStore, (s) => s.currentTime);
  const duration = useStore(usePlayerStore, (s) => s.duration);
  const playbackRate = useStore(usePlayerStore, (s) => s.playbackRate);
  const volume = useStore(usePlayerStore, (s) => s.volume);
  const sleepMode = useStore(usePlayerStore, (s) => s.sleepMode);
  const sleepEndsAt = useStore(usePlayerStore, (s) => s.sleepEndsAt);
  const track = useStore(usePlayerStore, currentTrack);

  // スリープタイマー「N 分後」の残り時間表示を毎秒更新する。
  // 現在時刻を state に持ち、有効化直後(seed)と毎秒(interval)で更新する。
  // 実際の停止判定は AudioPlayer 側(絶対時刻)が担い、ここは表示だけを持つ。
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!overlay.playerOpen || sleepMode !== 'duration') return;
    const tick = () => setNow(Date.now());
    const seed = setTimeout(tick, 0); // 有効化直後に現在時刻へ追いつく
    const id = setInterval(tick, 1000);
    return () => {
      clearTimeout(seed);
      clearInterval(id);
    };
  }, [overlay.playerOpen, sleepMode]);

  // Escape キーで 1 段閉じる(キュー画面 → プレイヤー → ミニプレイヤーの順)
  useEffect(() => {
    if (!overlay.playerOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      if (overlay.queueOpen) overlay.closeQueue();
      else overlay.closePlayer();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [overlay.playerOpen, overlay.queueOpen]);

  useBodyScrollLock(overlay.playerOpen);

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
      overlay.closePlayer();
    }
  };

  if (!overlay.playerOpen || !track) return null;

  const store = usePlayerStore.getState();
  const thumbUrl = playerThumbUrl(track);
  // シークバーの進捗表示用パーセント
  const progressPct = duration ? (currentTime / duration) * 100 : 0;
  // スリープタイマー「N 分後」の残り秒数(絶対時刻 - 現在時刻)。0 未満にはしない
  const sleepRemainingSec =
    sleepEndsAt !== null ? Math.max(0, (sleepEndsAt - now) / 1000) : 0;

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
          onClick={() => overlay.closePlayer()}
          aria-label="ミニプレイヤーに戻る"
        >
          ⌄
        </button>
        <button
          type="button"
          className={styles.titleBtn}
          onClick={() => {
            // 新しいエントリにはオーバーレイのフラグが無いため、プレイヤーは自然に閉じる
            // (「戻る」で作品ページからプレイヤーへ戻れる)
            navigate(`/works/${track.workId}`);
          }}
          aria-label={`${track.workTitle} の作品ページを開く`}
        >
          {track.workTitle}
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
          <Thumbnail className={styles.artwork} src={thumbUrl} />
        )}
      </div>

      {/* トラック名 + 作品タイトル */}
      <div className={styles.trackInfo}>
        <div className={styles.trackName} title={track.name}>
          {track.name}
        </div>
        <div className={styles.workTitle} title={track.workTitle}>
          {track.workTitle}
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

      {/* スリープタイマー(N 分後 / このトラックの終わりで停止) */}
      <div className={styles.sleepRow}>
        {sleepMode === 'off' ? (
          <>
            <span className={styles.sleepLabel} aria-hidden>
              💤
            </span>
            {SLEEP_PRESETS_MIN.map((min) => (
              <button
                key={min}
                type="button"
                className={styles.sleepBtn}
                onClick={() => store.setSleepAfter(min)}
                aria-label={`${min}分後に停止`}
              >
                {min}分
              </button>
            ))}
            <button
              type="button"
              className={styles.sleepBtn}
              onClick={() => store.setSleepEndOfTrack()}
              aria-label="このトラックの終わりで停止"
            >
              曲終わり
            </button>
          </>
        ) : (
          <>
            <span className={styles.sleepActive} role="status" aria-live="polite">
              💤{' '}
              {sleepMode === 'duration'
                ? `停止まで ${formatTime(sleepRemainingSec)}`
                : 'このトラックの終わりで停止'}
            </span>
            <button
              type="button"
              className={styles.sleepCancelBtn}
              onClick={() => store.clearSleepTimer()}
              aria-label="スリープタイマーを解除"
            >
              解除
            </button>
          </>
        )}
      </div>

      {/* キュー画面を開く */}
      <div className={styles.queueBtnRow}>
        <button
          type="button"
          className={styles.queueOpenBtn}
          onClick={() => overlay.openQueue()}
          aria-label="再生キューを表示"
        >
          <span aria-hidden>☰</span> キュー({index + 1}/{queue.length})
        </button>
      </div>

      {/* 再生キュー画面(このオーバーレイのさらに上に重なる) */}
      {overlay.queueOpen && <QueueScreen />}
    </div>
  );
}
