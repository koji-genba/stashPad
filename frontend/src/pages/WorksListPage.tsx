import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { SortKey, WorksResponse } from '@/api/types';
import { fetchWorks } from '@/api/client';
import { useTagStore, useTagNameMap } from '@/store/tagStore';
import WorkCard from '@/components/WorkCard';
import TagFacetPanel from '@/components/TagFacetPanel';
import CircleFacetPanel from '@/components/CircleFacetPanel';
import { saveListSearch } from '@/lib/listSearchMemory';
import styles from './WorksListPage.module.css';

const LIMIT = 40;

const SORT_LABELS: Record<SortKey, string> = {
  purchase_date: '購入日',
  title: 'タイトル',
  created_at: '登録日',
  circle: 'サークル名',
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
  // 除外タグ ID リスト
  const excludeTags = useMemo(() => parseTags(params.get('exclude_tags')), [params]);
  const circle = params.get('circle') ?? '';
  const series = params.get('series') ?? '';
  const sort = (params.get('sort') as SortKey) || 'purchase_date';
  const page = Number(params.get('page') ?? '1') || 1;

  const [qInput, setQInput] = useState(q);
  // 毎レンダーで最新の q を ref に書き込む。デバウンスエフェクトの deps に
  // q を入れると URL 更新直後に無駄な再実行が生じるため、ref 経由で参照する。
  const qRef = useRef(q);
  qRef.current = q;
  const [data, setData] = useState<WorksResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  // 全タグ一覧を共有ストアから取得し、id→name の Map として参照する。
  // TagFacetPanel でも同じストアを使うため HTTP リクエストは 1 回だけ発火する。
  const tagNames = useTagNameMap();

  // 検索ボックスは URL と同期(戻る等で反映)
  useEffect(() => {
    setQInput(q);
  }, [q]);

  // setParams の関数形式を使い、タイマー発火時に最新の params を参照する
  const update = useCallback((mut: (p: URLSearchParams) => void) => {
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      mut(next);
      return next;
    }, { replace: false });
  }, [setParams]);

  // 入力を 300ms デバウンスして URL に反映(リアルタイム検索)
  useEffect(() => {
    if (qInput === qRef.current) return;
    const t = setTimeout(() => {
      update((p) => {
        if (qInput) p.set('q', qInput);
        else p.delete('q');
        p.delete('page');
      });
    }, 300);
    return () => clearTimeout(t);
  }, [qInput, update]);

  // マウント時にタグ一覧をプリフェッチ(ストアがキャッシュするので 1 回のみ発火)
  useEffect(() => {
    useTagStore.getState().ensureLoaded();
  }, []);

  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    fetchWorks(
      {
        q,
        tags,
        excludeTags,
        circle: circle || undefined,
        series: series || undefined,
        sort,
        page,
        limit: LIMIT,
      },
      ac.signal,
    )
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
  }, [q, tags, excludeTags, circle, series, sort, page]);

  // URL が変わるたびに検索クエリを sessionStorage に保存(詳細→一覧の戻り先に使う)
  useEffect(() => {
    saveListSearch(params.toString());
  }, [params]);

  const submitSearch = (e: React.FormEvent) => {
    e.preventDefault();
    update((p) => {
      if (qInput) p.set('q', qInput);
      else p.delete('q');
      p.delete('page');
    });
  };

  // タグの 3 状態サイクル: 未選択 → 含む → 除外 → 未選択
  const toggleTag = (tagId: number) => {
    update((p) => {
      const selSet = new Set(parseTags(p.get('tags')));
      const exclSet = new Set(parseTags(p.get('exclude_tags')));
      if (selSet.has(tagId)) {
        // 含む → 除外
        selSet.delete(tagId);
        exclSet.add(tagId);
      } else if (exclSet.has(tagId)) {
        // 除外 → 未選択
        exclSet.delete(tagId);
      } else {
        // 未選択 → 含む
        selSet.add(tagId);
        // 念のため除外側にあれば削除
        exclSet.delete(tagId);
      }
      if (selSet.size > 0) p.set('tags', [...selSet].join(','));
      else p.delete('tags');
      if (exclSet.size > 0) p.set('exclude_tags', [...exclSet].join(','));
      else p.delete('exclude_tags');
      p.delete('page');
    });
  };

  // チップの ✕ クリック: 3 状態サイクルを経由せず、含む/除外を直接解除する
  const clearTagParam = (key: 'tags' | 'exclude_tags', tagId: number) => {
    update((p) => {
      const set = new Set(parseTags(p.get(key)));
      set.delete(tagId);
      if (set.size > 0) p.set(key, [...set].join(','));
      else p.delete(key);
      p.delete('page');
    });
  };

  const setCircle = (name: string) => {
    update((p) => {
      if (name) p.set('circle', name);
      else p.delete('circle');
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

  const clearParam = (key: 'circle' | 'series') => {
    update((p) => {
      p.delete(key);
      p.delete('page');
    });
  };

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.limit)) : 1;
  // タグ絞り込みボタンのバッジ: 含む + 除外の合計
  const tagFilterCount = tags.length + excludeTags.length;

  return (
    <div className={styles.layout}>
      {/* PC: 左サイドパネル / スマホ: 非表示 */}
      <aside className={styles.sidebar}>
        <h2 className={styles.sidebarTitle}>サークル</h2>
        <CircleFacetPanel selected={circle} onSelect={setCircle} />
        <h2 className={`${styles.sidebarTitle} ${styles.sidebarTitleSpaced}`}>タグ</h2>
        <TagFacetPanel selected={tags} excluded={excludeTags} onToggle={toggleTag} />
      </aside>

      <div className={styles.content}>
        <div className={styles.toolbar}>
          <form className={styles.searchForm} onSubmit={submitSearch}>
            <input
              className="input"
              type="search"
              placeholder="タイトル・サークル・RJ番号(スペース区切り、-語 で除外)"
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
              タグ絞り込み{tagFilterCount > 0 ? ` (${tagFilterCount})` : ''}
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

          {(tags.length > 0 || excludeTags.length > 0 || circle || series) && (
            <div className={styles.chips}>
              {circle && (
                <button
                  className={styles.chip}
                  onClick={() => clearParam('circle')}
                  title="クリックで解除"
                >
                  <span className={styles.chipKind}>サークル</span>
                  {circle}
                  <span className={styles.chipX}>✕</span>
                </button>
              )}
              {series && (
                <button
                  className={styles.chip}
                  onClick={() => clearParam('series')}
                  title="クリックで解除"
                >
                  <span className={styles.chipKind}>シリーズ</span>
                  {series}
                  <span className={styles.chipX}>✕</span>
                </button>
              )}
              {tags.map((id) => (
                <button
                  key={id}
                  className={styles.chip}
                  onClick={() => clearTagParam('tags', id)}
                  title="クリックで解除"
                >
                  {tagNames.get(id) ?? `#${id}`}
                  <span className={styles.chipX}>✕</span>
                </button>
              ))}
              {excludeTags.map((id) => (
                <button
                  key={`excl-${id}`}
                  className={`${styles.chip} ${styles.chipExcluded}`}
                  onClick={() => clearTagParam('exclude_tags', id)}
                  title="クリックで解除"
                >
                  −{tagNames.get(id) ?? `#${id}`}
                  <span className={styles.chipX}>✕</span>
                </button>
              ))}
              {tags.length > 1 && <span className={styles.chipNote}>(AND 条件)</span>}
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
                <label className={styles.pageInfo}>
                  <select
                    className={styles.pageSelect}
                    value={page}
                    onChange={(e) => goPage(Number(e.target.value))}
                    aria-label="ページを選択"
                  >
                    {Array.from({ length: totalPages }, (_, i) => i + 1).map((n) => (
                      <option key={n} value={n}>
                        {n}
                      </option>
                    ))}
                  </select>
                  {' / '}
                  {totalPages}
                </label>
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
              <h2 className={styles.sidebarTitle}>絞り込み</h2>
              <button
                className={styles.drawerClose}
                onClick={() => setDrawerOpen(false)}
                aria-label="閉じる"
              >
                ✕
              </button>
            </div>
            <h3 className={styles.sidebarTitle}>サークル</h3>
            <CircleFacetPanel selected={circle} onSelect={(name) => { setCircle(name); setDrawerOpen(false); }} />
            <h3 className={`${styles.sidebarTitle} ${styles.sidebarTitleSpaced}`}>タグ</h3>
            <TagFacetPanel selected={tags} excluded={excludeTags} onToggle={toggleTag} />
          </div>
        </div>
      )}
    </div>
  );
}
