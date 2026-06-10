import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import type { HistoryItem } from '@/api/types';
import { fetchHistory } from '@/api/client';
import { basename, formatDateTime } from '@/utils/format';
import styles from './HistoryPage.module.css';

export default function HistoryPage() {
  const [items, setItems] = useState<HistoryItem[]>([]);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);

  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    fetchHistory(page, ac.signal)
      .then((d) => {
        setItems(d.items);
        // バックエンドは page のみ返すので、満杯なら次がある可能性ありと見なす
        setHasMore(d.items.length >= 40);
        setLoading(false);
      })
      .catch((e: unknown) => {
        if (ac.signal.aborted) return;
        setError(e instanceof Error ? e.message : '読み込み失敗');
        setLoading(false);
      });
    return () => ac.abort();
  }, [page]);

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>再生履歴</h1>

      {loading ? (
        <div className={styles.center}>
          <div className="spinner" />
        </div>
      ) : error ? (
        <p className="muted">{error}</p>
      ) : items.length === 0 ? (
        <p className={styles.empty}>まだ再生履歴がありません</p>
      ) : (
        <>
          <ul className={styles.list}>
            {items.map((item) => (
              <li key={item.work.id}>
                <Link to={`/works/${item.work.id}`} className={styles.row}>
                  <img
                    className={styles.thumb}
                    src={item.work.thumbnail_url}
                    alt=""
                    loading="lazy"
                    onError={(e) => {
                      e.currentTarget.style.visibility = 'hidden';
                    }}
                  />
                  <div className={styles.info}>
                    <div className={styles.workTitle}>{item.work.title}</div>
                    <div className={styles.sub}>
                      {basename(item.last_file_path)}
                    </div>
                    <div className={styles.meta}>
                      <span>{formatDateTime(item.last_played_at)}</span>
                      <span className={styles.count}>{item.play_count} 回</span>
                    </div>
                  </div>
                </Link>
              </li>
            ))}
          </ul>

          {(page > 1 || hasMore) && (
            <div className={styles.pager}>
              <button
                className="btn"
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
              >
                前へ
              </button>
              <span className={styles.pageInfo}>{page}</span>
              <button
                className="btn"
                onClick={() => setPage((p) => p + 1)}
                disabled={!hasMore}
              >
                次へ
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
