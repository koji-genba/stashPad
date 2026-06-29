import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import type { HistoryItem, HistorySort, HistoryOrder } from '@/api/types';
import { fetchHistory } from '@/api/client';
import { basename, formatDateTime } from '@/utils/format';
import styles from './HistoryPage.module.css';

export default function HistoryPage() {
  const [items, setItems] = useState<HistoryItem[]>([]);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);

  // 検索・ソート用 state
  const [qInput, setQInput] = useState('');         // 入力欄の生値
  const [q, setQ] = useState('');                   // デバウンス後のキーワード
  const [sort, setSort] = useState<HistorySort>('last_played');
  const [order, setOrder] = useState<HistoryOrder>('desc');

  // qInput を 300ms デバウンスして q に反映。フィルタ変更時はページを先頭に戻す。
  // データ取得 effect は [page, q, ...] を見るので、ここで page も一緒に確定させて
  // 二重 fetch(古い page での取得 → page=1 での再取得)を防ぐ。
  useEffect(() => {
    const t = setTimeout(() => {
      setQ(qInput);
      setPage(1);
    }, 300);
    return () => clearTimeout(t);
  }, [qInput]);

  // データ取得
  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    fetchHistory({ page, q, sort, order }, ac.signal)
      .then((d) => {
        setItems(d.items);
        setHasMore(d.items.length >= d.limit);
        setLoading(false);
      })
      .catch((e: unknown) => {
        if (ac.signal.aborted) return;
        setError(e instanceof Error ? e.message : '読み込み失敗');
        setLoading(false);
      });
    return () => ac.abort();
  }, [page, q, sort, order]);

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>再生履歴</h1>

      {/* 検索・ソートツールバー(loading/error/empty に関わらず常に表示) */}
      <div className={styles.toolbar}>
        <input
          className={styles.search}
          type="search"
          value={qInput}
          onChange={(e) => setQInput(e.target.value)}
          placeholder="作品名で絞り込み"
        />
        <select
          className={styles.select}
          value={sort}
          onChange={(e) => {
            setSort(e.target.value as HistorySort);
            setPage(1);
          }}
        >
          <option value="last_played">最終再生日</option>
          <option value="play_count">再生回数</option>
        </select>
        <button
          className="btn"
          aria-label={order === 'desc' ? '昇順に切り替え' : '降順に切り替え'}
          onClick={() => {
            setOrder((o) => (o === 'desc' ? 'asc' : 'desc'));
            setPage(1);
          }}
        >
          {order === 'desc' ? '降順 ↓' : '昇順 ↑'}
        </button>
      </div>

      {loading ? (
        <div className={styles.center}>
          <div className="spinner" />
        </div>
      ) : error ? (
        <p className="muted">{error}</p>
      ) : items.length === 0 ? (
        <p className={styles.empty}>{q ? '該当する履歴がありません' : 'まだ再生履歴がありません'}</p>
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
