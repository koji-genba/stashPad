// ファイル行の「⋮」ボタンをタップしたときに表示するボトムシート式メニュー。
// 「今の曲が終わったら再生」「キューを置き換えて再生」「キューの最後に追加」の
// 3 アクション + キャンセルを提供する。
// z-index: 90(ミニプレイヤー 45 / フルスクリーン 60 / キュー画面 70 より上)
import { useEffect } from 'react';
import type { EnqueueInput } from '@/store/playerStore';
import { usePlayerStore } from '@/store/playerStore';
import styles from './QueueActionSheet.module.css';

interface Props {
  /** 操作対象のファイル名(パネル先頭に表示) */
  name: string;
  /** キュー操作に渡す EnqueueInput */
  input: EnqueueInput;
  /** シートを閉じるコールバック */
  onClose: () => void;
}

export default function QueueActionSheet({ name, input, onClose }: Props) {
  // 表示中は body スクロールロック(FullscreenPlayer.tsx と同じイディオム)
  useEffect(() => {
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = prev;
    };
  }, []);

  // Escape キーで閉じる
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const handleAction = (action: (input: EnqueueInput) => void) => {
    action(input);
    onClose();
  };

  return (
    <div className={styles.root}>
      {/* バックドロップ: クリックで閉じる */}
      <div
        className={styles.backdrop}
        aria-hidden="true"
        onClick={onClose}
      />
      {/* パネル本体 */}
      <div
        role="dialog"
        aria-label={`${name} のキュー操作`}
        className={styles.panel}
      >
        <p className={styles.fileName}>{name}</p>

        <button
          type="button"
          className={styles.action}
          onClick={() => handleAction((i) => usePlayerStore.getState().playTrackNext(i))}
        >
          今の曲が終わったら再生
        </button>

        <button
          type="button"
          className={styles.action}
          onClick={() => handleAction((i) => usePlayerStore.getState().replaceQueueWith(i))}
        >
          キューを置き換えて再生
        </button>

        <button
          type="button"
          className={styles.action}
          onClick={() => handleAction((i) => usePlayerStore.getState().appendToQueue(i))}
        >
          キューの最後に追加
        </button>

        <button
          type="button"
          className={`${styles.action} ${styles.cancel}`}
          onClick={onClose}
        >
          キャンセル
        </button>
      </div>
    </div>
  );
}
