// ルート直下に常駐する全画面オーバーレイ群(画像ビューア / 動画 / テキスト)。
import { useEffect, useState } from 'react';
import { useStore } from 'zustand';
import { fetchTextFile, fileUrl, recordPlay } from '@/api/client';
import { useOverlayStore } from '@/store/overlayStore';
import ImageViewer from './ImageViewer';
import styles from './Overlays.module.css';

function VideoModal() {
  const video = useStore(useOverlayStore, (s) => s.video);
  const close = useStore(useOverlayStore, (s) => s.closeVideo);

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
  return (
    <>
      <ImageViewer />
      <VideoModal />
      <TextModal />
    </>
  );
}
