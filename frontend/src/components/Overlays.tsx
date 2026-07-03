// ルート直下に常駐する全画面オーバーレイ群(画像ビューア / 動画 / テキスト)。
// useOverlayHistorySync により表示状態を history と同期し、
// Android の「戻る」で 1 段閉じられるようにする(issue #52)。
import { useEffect, useState } from 'react';
import { useStore } from 'zustand';
import { fetchTextFile, fileUrl, recordPlay } from '@/api/client';
import { useOverlayStore } from '@/store/overlayStore';
import { useBodyScrollLock } from '@/hooks/useBodyScrollLock';
import { useOverlayHistorySync } from '@/hooks/useOverlayHistorySync';
import ImageViewer from './ImageViewer';
import styles from './Overlays.module.css';

// Escape キーでオーバーレイを閉じる(active の間だけ購読する)
function useEscapeToClose(active: boolean, close: () => void): void {
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [active, close]);
}

function VideoModal() {
  const video = useStore(useOverlayStore, (s) => s.video);
  const close = useStore(useOverlayStore, (s) => s.closeVideo);

  useBodyScrollLock(video !== null);
  useEscapeToClose(video !== null, close);

  useEffect(() => {
    if (video) {
      // 動画も再生開始を履歴記録する
      void recordPlay(video.workId, video.path).catch(() => {});
    }
  }, [video]);

  if (!video) return null;
  return (
    <div className={styles.overlay}>
      <div className={styles.topbar}>
        <span className={styles.title}>{video.name}</span>
        <button className={styles.close} onClick={close} aria-label="閉じる">
          ✕
        </button>
      </div>
      <video
        className={styles.video}
        src={fileUrl(video.workId, video.path)}
        controls
        autoPlay
        playsInline
      />
    </div>
  );
}

function TextModal() {
  const text = useStore(useOverlayStore, (s) => s.text);
  const close = useStore(useOverlayStore, (s) => s.closeText);
  const [content, setContent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useBodyScrollLock(text !== null);
  useEscapeToClose(text !== null, close);

  useEffect(() => {
    setContent(null);
    setError(null);
    if (!text) return;
    const ac = new AbortController();
    fetchTextFile(text.workId, text.path, ac.signal)
      .then(setContent)
      .catch((e: unknown) => {
        if (!ac.signal.aborted) setError(e instanceof Error ? e.message : '読み込み失敗');
      });
    return () => ac.abort();
  }, [text]);

  if (!text) return null;
  return (
    <div className={styles.overlay}>
      <div className={styles.topbar}>
        <span className={styles.title}>{text.name}</span>
        <button className={styles.close} onClick={close} aria-label="閉じる">
          ✕
        </button>
      </div>
      <div className={styles.textScroll}>
        {error ? (
          <p className="muted">読み込みに失敗しました: {error}</p>
        ) : content === null ? (
          <div className="spinner" />
        ) : (
          <pre className={styles.pre}>{content}</pre>
        )}
      </div>
    </div>
  );
}

export default function Overlays() {
  useOverlayHistorySync();
  return (
    <>
      <ImageViewer />
      <VideoModal />
      <TextModal />
    </>
  );
}
