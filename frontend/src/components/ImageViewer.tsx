// フルスクリーン画像ビューア(マンガモード)。
// 同一ディレクトリの image を自然順(entries 順)でページ列化。
// スワイプ / ←→ キー / 画面端タップで送り戻し。次ページ 1 枚プリロード。
import { useEffect, useRef } from 'react';
import { useStore } from 'zustand';
import { fileUrl } from '@/api/client';
import { useOverlayStore } from '@/store/overlayStore';
import styles from './ImageViewer.module.css';

export default function ImageViewer() {
  const image = useStore(useOverlayStore, (s) => s.image);
  const next = useStore(useOverlayStore, (s) => s.imageNext);
  const prev = useStore(useOverlayStore, (s) => s.imagePrev);
  const close = useStore(useOverlayStore, (s) => s.closeImage);

  const touchStartX = useRef<number | null>(null);
  const touchStartY = useRef<number | null>(null);

  // キーボード操作
  useEffect(() => {
    if (!image) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowRight') next();
      else if (e.key === 'ArrowLeft') prev();
      else if (e.key === 'Escape') close();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [image, next, prev, close]);

  if (!image) return null;
  const { workId, pages, index } = image;
  const page = pages[index];
  if (!page) return null;
  const nextPage = pages[index + 1];

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
    if (Math.abs(dx) > 50 && Math.abs(dx) > Math.abs(dy)) {
      // 左スワイプ=次へ / 右スワイプ=前へ
      if (dx < 0) next();
      else prev();
    }
  };

  return (
    <div className={styles.overlay} onTouchStart={onTouchStart} onTouchEnd={onTouchEnd}>
      <div className={styles.topbar}>
        <span className={styles.counter}>
          {index + 1} / {pages.length}
        </span>
        <button className={styles.close} onClick={close} aria-label="閉じる">
          ✕
        </button>
      </div>

      <img className={styles.image} src={fileUrl(workId, page.path)} alt={page.name} />

      {/* 画面端タップ領域(PC・スマホ共通) */}
      <button className={styles.tapPrev} onClick={prev} aria-label="前のページ" />
      <button className={styles.tapNext} onClick={next} aria-label="次のページ" />

      {/* 次ページのプリロード(非表示) */}
      {nextPage && (
        <img className={styles.preload} src={fileUrl(workId, nextPage.path)} alt="" />
      )}
    </div>
  );
}
