// サークルファセット。絞り込み入力付き。選択は単一(完全一致)。
// 選択中サークルを再クリックすると解除される。
import { useEffect, useState } from 'react';
import type { CircleFacet } from '@/api/types';
import { fetchCircles } from '@/api/client';
import styles from './CircleFacetPanel.module.css';

/** 上位表示件数の上限 */
const MAX_DISPLAY = 50;

interface Props {
  /** 現在選択中のサークル名(空文字列 or 未設定なら未選択) */
  selected: string;
  onSelect: (circleName: string) => void;
}

export default function CircleFacetPanel({ selected, onSelect }: Props) {
  const [circles, setCircles] = useState<CircleFacet[]>([]);
  const [q, setQ] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const ac = new AbortController();
    // デバウンス: 入力 250ms 後に fetch
    const t = setTimeout(() => {
      setLoading(true);
      fetchCircles(q ? { q } : {}, ac.signal)
        .then((d) => {
          setCircles(d.items);
          setLoading(false);
        })
        .catch(() => {
          if (!ac.signal.aborted) setLoading(false);
        });
    }, q ? 250 : 0);
    return () => {
      clearTimeout(t);
      ac.abort();
    };
  }, [q]);

  const displayed = circles.slice(0, MAX_DISPLAY);
  const hasMore = circles.length > MAX_DISPLAY;

  return (
    <div className={styles.panel}>
      <input
        className="input"
        type="search"
        placeholder="サークルを絞り込み"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      {loading ? (
        <div className={styles.center}>
          <div className="spinner" />
        </div>
      ) : circles.length === 0 ? (
        <p className="faint">サークルがありません</p>
      ) : (
        <>
          <ul className={styles.list}>
            {displayed.map((c) => {
              const isSel = selected === c.name;
              return (
                <li key={c.name}>
                  <button
                    type="button"
                    className={`${styles.item} ${isSel ? styles.itemSel : ''}`}
                    onClick={() => onSelect(isSel ? '' : c.name)}
                  >
                    <span className={styles.itemName}>{c.name}</span>
                    <span className={styles.itemCount}>{c.work_count}</span>
                  </button>
                </li>
              );
            })}
          </ul>
          {hasMore && (
            <p className={styles.note}>
              上位 {MAX_DISPLAY} 件を表示中。絞り込み入力でさらに検索してください。
            </p>
          )}
        </>
      )}
    </div>
  );
}
