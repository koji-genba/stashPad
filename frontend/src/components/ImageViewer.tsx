// フルスクリーン画像ビューア(マンガモード)。
// 同一ディレクトリの image を自然順(entries 順)でページ列化。
// スワイプ / ←→ キー / 画面端タップで送り戻し。ピンチ・ホイールズーム・ダブルタップ拡大・パンに対応(issue #28)。
// index+1〜+3 の 3 枚を先読みプリロードする。
import { useEffect, useRef, useState } from 'react';
import { useStore } from 'zustand';
import { TransformWrapper, TransformComponent } from 'react-zoom-pan-pinch';
import { fileUrl } from '@/api/client';
import { useOverlayStore } from '@/store/overlayStore';
import { useBodyScrollLock } from '@/hooks/useBodyScrollLock';
import styles from './ImageViewer.module.css';

/** ズーム判定のしきい値。浮動小数の誤差やわずかなドラッグ揺れを等倍扱いにする */
const ZOOM_THRESHOLD = 1.01;
/** ダブルクリック/タップで toggle した際の目標倍率(約 2.5x)。
 * react-zoom-pan-pinch は smooth モード既定(scale = e^step)なので ln(2.5) を渡す。 */
const DOUBLE_CLICK_ZOOM_STEP = Math.log(2.5);
/** 先読みするページ数(index+1 〜 index+PRELOAD_AHEAD) */
const PRELOAD_AHEAD = 3;

/** スケール値からズーム中かどうかを判定する(テスト容易化のため純関数として切り出し) */
// eslint-disable-next-line react-refresh/only-export-components -- テスト用の純関数エクスポート。コンポーネントと分離するリファクタは対象外
export function isZoomed(scale: number): boolean {
  return scale > ZOOM_THRESHOLD;
}

export default function ImageViewer() {
  const image = useStore(useOverlayStore, (s) => s.image);
  const next = useStore(useOverlayStore, (s) => s.imageNext);
  const prev = useStore(useOverlayStore, (s) => s.imagePrev);
  const close = useStore(useOverlayStore, (s) => s.closeImage);

  useBodyScrollLock(image !== null);

  const touchStartX = useRef<number | null>(null);
  const touchStartY = useRef<number | null>(null);
  const [zoomed, setZoomed] = useState(false);

  const index = image?.index ?? -1;

  // ページ切替時にズーム状態を等倍に戻す。transform 自体のリセットは
  // <TransformWrapper key={idx}> によるページごとの再マウントに任せる
  // (resetTransform(0) だと前ページのズーム状態が新ページに1フレーム
  // 適用されてしまうちらつきが issue #85 で報告されたため)。
  // 再マウントでは onTransformed が発火せず親の zoomed が古いままになるので、
  // この setState は引き続き必要。
  useEffect(() => {
    // ページ切替時にズーム状態も等倍に戻す意図的な setState
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setZoomed(false);
  }, [index]);

  // キーボード操作(←→ でのページ送りはズーム中でも常に有効)
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
  const { workId, pages, index: idx } = image;
  const page = pages[idx];
  if (!page) return null;

  const preloadPages: { name: string; path: string }[] = [];
  for (let i = 1; i <= PRELOAD_AHEAD; i++) {
    const p = pages[idx + i];
    if (p) preloadPages.push(p);
  }

  const onTouchStart = (e: React.TouchEvent) => {
    if (zoomed) return; // ズーム中はパン優先で独自スワイプを無効化
    touchStartX.current = e.touches[0].clientX;
    touchStartY.current = e.touches[0].clientY;
  };
  const onTouchEnd = (e: React.TouchEvent) => {
    if (zoomed) return;
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
          {idx + 1} / {pages.length}
        </span>
        <button className={styles.close} onClick={close} aria-label="閉じる">
          ✕
        </button>
      </div>

      {/*
        key={idx} でページごとに TransformWrapper を再マウントする。プリロード用 img は
        TransformWrapper の外にあるため影響を受けず、新ページの img はプリロード済み
        キャッシュから即表示されるため、再マウントしても表示上の遅延は生じない。
      */}
      <TransformWrapper
        key={idx}
        minScale={1}
        maxScale={5}
        limitToBounds
        centerOnInit
        doubleClick={{ mode: 'toggle', step: DOUBLE_CLICK_ZOOM_STEP }}
        onTransformed={(_ref, state) => setZoomed(isZoomed(state.scale))}
      >
        <TransformComponent wrapperClass={styles.tpWrapper} contentClass={styles.tpContent}>
          <img className={styles.image} src={fileUrl(workId, page.path)} alt={page.name} />
        </TransformComponent>
      </TransformWrapper>

      {/* 画面端タップ領域(PC・スマホ共通)。ズーム中はパンを妨げないよう非表示 */}
      {!zoomed && (
        <>
          <button className={styles.tapPrev} onClick={prev} aria-label="前のページ" />
          <button className={styles.tapNext} onClick={next} aria-label="次のページ" />
        </>
      )}

      {/* 先読みプリロード(非表示、index+1〜+3) */}
      {preloadPages.map((p) => (
        <img
          key={p.path}
          data-testid="preload-image"
          className={styles.preload}
          src={fileUrl(workId, p.path)}
          alt=""
        />
      ))}
    </div>
  );
}
