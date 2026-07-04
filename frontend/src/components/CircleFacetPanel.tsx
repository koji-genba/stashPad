// サークルファセット。絞り込み入力付き。選択は単一(完全一致)。
// 選択中サークルを再クリックすると解除される。
//
// #27: パネル初回マウント時に q なしで全件を 1 回だけ fetch し、以降の入力による
// 絞り込みはクライアントサイド(name.toLowerCase().includes)で行う。サーバへの
// デバウンス再フェッチは行わない。
import { useEffect, useMemo, useState } from 'react';
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
  const [failed, setFailed] = useState(false);

  // 初回マウント時に全件を 1 回だけ fetch する。以降の絞り込みはクライアントサイドで行うため
  // q には依存しない。
  useEffect(() => {
    const ac = new AbortController();
    // fetch 開始前にローディング表示へ切り替える意図的な setState(データ取得 effect の定型)
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    setFailed(false);
    fetchCircles({}, ac.signal)
      .then((d) => {
        setCircles(d.items);
        setLoading(false);
      })
      .catch(() => {
        if (ac.signal.aborted) return;
        setFailed(true);
        setLoading(false);
      });
    return () => {
      ac.abort();
    };
  }, []);

  const filtered = useMemo(() => {
    if (!q) return circles;
    const needle = q.toLowerCase();
    return circles.filter((c) => c.name.toLowerCase().includes(needle));
  }, [circles, q]);

  const displayed = filtered.slice(0, MAX_DISPLAY);
  const hasMore = filtered.length > MAX_DISPLAY;

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
      ) : failed ? (
        <p className="error">サークル一覧の読み込みに失敗しました</p>
      ) : filtered.length === 0 ? (
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
