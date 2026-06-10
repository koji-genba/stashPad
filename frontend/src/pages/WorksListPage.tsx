import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { SortKey, WorksResponse } from '@/api/types';
import { fetchTags, fetchWorks } from '@/api/client';
import WorkCard from '@/components/WorkCard';
import TagFacetPanel from '@/components/TagFacetPanel';
import styles from './WorksListPage.module.css';

const LIMIT = 40;

const SORT_LABELS: Record<SortKey, string> = {
  purchase_date: '購入日',
  title: 'タイトル',
  created_at: '登録日',
};

function parseTags(value: string | null): number[] {
  if (!value) return [];
  return value
    .split(',')
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0);
}

export default function WorksListPage() {
  const [params, setParams] = useSearchParams();
  const q = params.get('q') ?? '';
  const tags = useMemo(() => parseTags(params.get('tags')), [params]);
  const sort = (params.get('sort') as SortKey) || 'purchase_date';
  const page = Number(params.get('page') ?? '1') || 1;

  const [qInput, setQInput] = useState(q);
  const [data, setData] = useState<WorksResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [tagNames, setTagNames] = useState<Map<number, string>>(new Map());

  // 検索ボックスは URL と同期(戻る等で反映)
  useEffect(() => {
    setQInput(q);
  }, [q]);

  // 選択タグのラベル表示用に、全タグ名を一度だけ取得してキャッシュ
  useEffect(() => {
    const ac = new AbortController();
    fetchTags({}, ac.signal)
      .then((d) => {
        setTagNames(new Map(d.items.map((t) => [t.id, t.name])));
      })
      .catch(() => {});
    return () => ac.abort();
  }, []);

  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    fetchWorks({ q, tags, sort, page, limit: LIMIT }, ac.signal)
      .then((d) => {
        setData(d);
        setLoading(false);
      })
      .catch((e: unknown) => {
        if (ac.signal.aborted) return;
        setError(e instanceof Error ? e.message : '読み込み失敗');
        setLoading(false);
      });
    return () => ac.abort();
  }, [q, tags, sort, page]);

  const update = (mut: (p: URLSearchParams) => void) => {
    const next = new URLSearchParams(params);
    mut(next);
    setParams(next, { replace: false });
  };

  const submitSearch = (e: React.FormEvent) => {
    e.preventDefault();
    update((p) => {
      if (qInput) p.set('q', qInput);
      else p.delete('q');
      p.delete('page');
    });
  };

  const toggleTag = (tagId: number) => {
    update((p) => {
      const set = new Set(parseTags(p.get('tags')));
      if (set.has(tagId)) set.delete(tagId);
      else set.add(tagId);
      if (set.size > 0) p.set('tags', [...set].join(','));
      else p.delete('tags');
      p.delete('page');
    });
  };

  const setSort = (s: SortKey) => {
    update((p) => {
      p.set('sort', s);
      p.delete('page');
    });
  };

  const goPage = (n: number) => {
    update((p) => p.set('page', String(n)));
    window.scrollTo(0, 0);
  };

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.limit)) : 1;

  return (
    <div className={styles.layout}>
      {/* PC: 左サイドパネル / スマホ: 非表示 */}
      <aside className={styles.sidebar}>
        <h2 className={styles.sidebarTitle}>タグ</h2>
        <TagFacetPanel selected={tags} onToggle={toggleTag} />
      </aside>

      <div className={styles.content}>
        <div className={styles.toolbar}>
          <form className={styles.searchForm} onSubmit={submitSearch}>
            <input
              className="input"
              type="search"
              placeholder="タイトル・サークル・RJ番号で検索"
              value={qInput}
              onChange={(e) => setQInput(e.target.value)}
            />
            <button type="submit" className="btn btn-primary">
              検索
            </button>
          </form>

          <div className={styles.controls}>
            <button
              className={`btn ${styles.filterBtn}`}
              onClick={() => setDrawerOpen(true)}
            >
              タグ絞り込み{tags.length > 0 ? ` (${tags.length})` : ''}
            </button>
            <select
              className={styles.sortSelect}
              value={sort}
              onChange={(e) => setSort(e.target.value as SortKey)}
              aria-label="並び替え"
            >
              {(Object.keys(SORT_LABELS) as SortKey[]).map((k) => (
                <option key={k} value={k}>
                  {SORT_LABELS[k]}順
                </option>
              ))}
            </select>
          </div>

          {tags.length > 0 && (
            <div className={styles.chips}>
              {tags.map((id) => (
                <button
                  key={id}
                  className={styles.chip}
                  onClick={() => toggleTag(id)}
                  title="クリックで解除"
                >
                  {tagNames.get(id) ?? `#${id}`}
                  <span className={styles.chipX}>✕</span>
                </button>
              ))}
              <span className={styles.chipNote}>(AND 条件)</span>
            </div>
          )}
        </div>

        {loading ? (
          <div className={styles.center}>
            <div className="spinner" />
          </div>
        ) : error ? (
          <p className="muted">{error}</p>
        ) : !data || data.items.length === 0 ? (
          <p className={styles.empty}>該当する作品がありません</p>
        ) : (
          <>
            <div className={styles.resultCount}>{data.total} 件</div>
            <div className={styles.grid}>
              {data.items.map((w) => (
                <WorkCard
                  key={w.id}
                  id={w.id}
                  title={w.title}
                  circle={w.circle}
                  ageRating={w.age_rating}
                  thumbnailUrl={w.thumbnail_url}
                  hasFolder={w.has_folder}
                />
              ))}
            </div>

            {totalPages > 1 && (
              <div className={styles.pager}>
                <button
                  className="btn"
                  onClick={() => goPage(page - 1)}
                  disabled={page <= 1}
                >
                  前へ
                </button>
                <span className={styles.pageInfo}>
                  {page} / {totalPages}
                </span>
                <button
                  className="btn"
                  onClick={() => goPage(page + 1)}
                  disabled={page >= totalPages}
                >
                  次へ
                </button>
              </div>
            )}
          </>
        )}
      </div>

      {/* スマホ用ドロワー */}
      {drawerOpen && (
        <div className={styles.drawerOverlay} onClick={() => setDrawerOpen(false)}>
          <div className={styles.drawer} onClick={(e) => e.stopPropagation()}>
            <div className={styles.drawerHead}>
              <h2 className={styles.sidebarTitle}>タグで絞り込み</h2>
              <button
                className={styles.drawerClose}
                onClick={() => setDrawerOpen(false)}
                aria-label="閉じる"
              >
                ✕
              </button>
            </div>
            <TagFacetPanel selected={tags} onToggle={toggleTag} />
          </div>
        </div>
      )}
    </div>
  );
}
