// 再生キュー画面。フルスクリーンプレイヤーの「キュー」ボタンから開く全画面オーバーレイ。
//
// - 表示モード: 番号付き 2 行リスト(上段ファイル名・下段作品名)。行タップでそのトラックを再生
// - 編集モード: 行ごとに削除ボタンとドラッグハンドル。操作は実キューへ即時反映される
//   (下書きを持たないため、「戻る」で閉じても編集結果はそのまま残る)
// - 閉じる操作は history を 1 段戻す(usePlayerOverlay.closeQueue)。Android の「戻る」も
//   同じ経路で、表示/編集どちらのモードからでもプレイヤーへ戻る
//
// ドラッグ並び替えは Pointer Events の自前実装(依存追加なし)。行高は一様である前提で
// ドラッグ開始時に実測し、ポインタ位置から挿入先スロットを算出して moveInQueue を逐次呼ぶ。
import { useEffect, useRef, useState } from 'react';
import { useStore } from 'zustand';
import { usePlayerStore } from '@/store/playerStore';
import { usePlayerOverlay } from '@/hooks/usePlayerOverlay';
import styles from './QueueScreen.module.css';

/** リスト端から何 px 以内で自動スクロールを始めるか */
const EDGE_SCROLL_ZONE = 48;
/** 自動スクロールの 1 イベントあたりの移動量(px) */
const EDGE_SCROLL_STEP = 8;

export default function QueueScreen() {
  const queue = useStore(usePlayerStore, (s) => s.queue);
  const index = useStore(usePlayerStore, (s) => s.index);
  const overlay = usePlayerOverlay();
  const [editing, setEditing] = useState(false);

  // ドラッグ状態。uid で追うことで並び替え後も同じトラックをつまみ続けられる
  const [dragUid, setDragUid] = useState<number | null>(null);
  const rowHeightRef = useRef(0);
  const listRef = useRef<HTMLOListElement | null>(null);

  // 表示モードでは現在再生行を見える位置へ自動スクロール
  const currentRowRef = useRef<HTMLLIElement | null>(null);
  useEffect(() => {
    if (editing) return;
    const el = currentRowRef.current;
    // scrollIntoView が未対応の環境(テスト環境)ではガード
    if (el && typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ block: 'nearest' });
    }
  }, [editing, index]);

  const onHandlePointerDown = (uid: number) => (e: React.PointerEvent<HTMLButtonElement>) => {
    const row = e.currentTarget.closest('li');
    const rowHeight = row?.getBoundingClientRect().height ?? 0;
    if (rowHeight <= 0) return; // 行高を計測できなければドラッグしない
    rowHeightRef.current = rowHeight;
    setDragUid(uid);
    // 以降の pointermove をハンドルで受け続ける(指が行の外へ出ても追従)
    e.currentTarget.setPointerCapture?.(e.pointerId);
    e.preventDefault();
  };

  const onHandlePointerMove = (e: React.PointerEvent<HTMLButtonElement>) => {
    if (dragUid === null) return;
    const list = listRef.current;
    const rowHeight = rowHeightRef.current;
    if (!list || rowHeight <= 0) return;
    const rect = list.getBoundingClientRect();
    // 連続する pointermove が再レンダーより先に届いても正しく動くよう、
    // キューは描画時のものではなく store から都度読む
    const q = usePlayerStore.getState().queue;
    const y = e.clientY - rect.top + list.scrollTop;
    const slot = Math.max(0, Math.min(q.length - 1, Math.floor(y / rowHeight)));
    const from = q.findIndex((t) => t.uid === dragUid);
    if (from >= 0 && slot !== from) {
      usePlayerStore.getState().moveInQueue(from, slot);
    }
    // リスト端に近づいたら自動スクロール(画面外の行へも運べるように)
    if (e.clientY < rect.top + EDGE_SCROLL_ZONE) {
      list.scrollTop = Math.max(0, list.scrollTop - EDGE_SCROLL_STEP);
    } else if (e.clientY > rect.bottom - EDGE_SCROLL_ZONE) {
      list.scrollTop += EDGE_SCROLL_STEP;
    }
  };

  const onHandlePointerEnd = () => setDragUid(null);

  const store = usePlayerStore.getState();

  return (
    <div className={styles.overlay} role="dialog" aria-label="再生キュー">
      {/* ヘッダ: 閉じる + タイトル + 編集/完了 */}
      <div className={styles.header}>
        <button
          type="button"
          className={styles.closeBtn}
          onClick={() => overlay.closeQueue()}
          aria-label="プレイヤーに戻る"
        >
          ⌄
        </button>
        <div className={styles.title}>
          再生キュー({index + 1}/{queue.length})
        </div>
        <button
          type="button"
          className={styles.editBtn}
          onClick={() => setEditing((v) => !v)}
          aria-label={editing ? 'キューの編集を終了' : 'キューを編集'}
        >
          {editing ? '完了' : '編集'}
        </button>
      </div>

      <ol className={styles.list} ref={listRef}>
        {queue.map((t, i) => {
          const isCurrent = i === index;
          const rowClass = [
            styles.row,
            isCurrent ? styles.rowCurrent : '',
            t.uid === dragUid ? styles.rowDragging : '',
          ].join(' ');
          const texts = (
            <>
              <span className={styles.num} aria-hidden>
                {i + 1}
              </span>
              <span className={styles.texts}>
                <span className={styles.trackName}>{t.name}</span>
                <span className={styles.workTitle}>{t.workTitle}</span>
              </span>
            </>
          );
          return (
            <li key={t.uid} className={rowClass} ref={isCurrent ? currentRowRef : null}>
              {editing ? (
                <div className={styles.rowBody}>
                  {texts}
                  <button
                    type="button"
                    className={styles.removeBtn}
                    onClick={() => store.removeFromQueue(i)}
                    aria-label={`${t.name} をキューから削除`}
                  >
                    {/* 「車両進入禁止」風アイコン */}
                    <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden focusable="false">
                      <circle cx="12" cy="12" r="10" fill="var(--danger)" />
                      <rect x="6" y="10.75" width="12" height="2.5" rx="1.25" fill="#fff" />
                    </svg>
                  </button>
                  <button
                    type="button"
                    className={styles.dragHandle}
                    onPointerDown={onHandlePointerDown(t.uid)}
                    onPointerMove={onHandlePointerMove}
                    onPointerUp={onHandlePointerEnd}
                    onPointerCancel={onHandlePointerEnd}
                    aria-label={`${t.name} を並び替え`}
                  >
                    ⇅
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  className={styles.rowBody}
                  onClick={() => store.playIndex(i)}
                  aria-label={`${t.name} を再生`}
                  aria-current={isCurrent ? 'true' : undefined}
                >
                  {texts}
                  {isCurrent && (
                    <span className={styles.nowPlaying} aria-hidden>
                      ♪
                    </span>
                  )}
                </button>
              )}
            </li>
          );
        })}
      </ol>
    </div>
  );
}
