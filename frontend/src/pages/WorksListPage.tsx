import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { SortKey, SortOrder, WorksResponse } from '@/api/types';
import { fetchWorks } from '@/api/client';
import { useTagStore, useTagNameMap } from '@/store/tagStore';
import WorkCard from '@/components/WorkCard';
import TagFacetPanel from '@/components/TagFacetPanel';
import CircleFacetPanel from '@/components/CircleFacetPanel';
import FetchError from '@/components/FetchError';
import { saveListSearch } from '@/lib/listSearchMemory';
import { useBodyScrollLock } from '@/hooks/useBodyScrollLock';
import { parseSearchTerms, splitQuery } from '@/utils/searchTerms';
import styles from './WorksListPage.module.css';

const LIMIT = 40;

const SORT_LABELS: Record<SortKey, string> = {
  purchase_date: '購入日',
  rj_number: 'RJ番号',
  title: 'タイトル',
  created_at: '登録日',
  circle: 'サークル名',
  rating: '評価',
  favorited_at: 'お気に入り登録',
  last_played: '最近聴いた',
  play_count: 'よく聴く',
};

const WORK_TYPE_OPTIONS = ['ボイス・ASMR', '動画', 'マンガ'];
const AGE_RATING_OPTIONS = ['全年齢', 'R-15', 'R18'];
const RATING_FILTER_OPTIONS = ['5', '4', '3', '2', '1', 'none'];
const PRIMARY_TAG_CATEGORIES = ['custom', 'genre', 'detail_genre'];
const DEFAULT_EXPANDED_TAG_CATEGORIES = ['custom', 'detail_genre'];

interface SidebarSectionProps {
  title: string;
  children: ReactNode;
  defaultOpen?: boolean;
}

function SidebarSection({ title, children, defaultOpen = true }: SidebarSectionProps) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <section className={styles.sidebarSection}>
      <div className={styles.sidebarSectionHead}>
        <h2 className={styles.sidebarTitle}>{title}</h2>
        <button
          type="button"
          className={styles.sectionToggle}
          onClick={() => setOpen((v) => !v)}
          aria-label={open ? `${title}を折りたたむ` : `${title}を展開`}
          aria-expanded={open}
        >
          {open ? '▼' : '▶'}
        </button>
      </div>
      {open && children}
    </section>
  );
}

function parseTags(value: string | null): number[] {
  if (!value) return [];
  return value
    .split(',')
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0);
}

function normalizeRatingFilter(value: string | null): '' | '1' | '2' | '3' | '4' | '5' | 'none' {
  if (value === 'none') return 'none';
  if (value && /^[1-5]$/.test(value)) return value as '1' | '2' | '3' | '4' | '5';
  return '';
}

function formatRatingFilter(value: string): string {
  if (value === 'none') return '未評価';
  const rating = Number(value);
  if (!Number.isInteger(rating) || rating < 1 || rating > 5) return value;
  return '★'.repeat(rating);
}

// フィルタ変更が結果セット全体に影響するとき、ユーザーが先頭に戻れるよう
// スクロールトップをオプションで指示できる型。
// replace: true のときは history を積まずに現在のエントリを置き換える
// (#58: デバウンス由来の連続更新で history が汚染されるのを防ぐ)
type UpdateOptions = { scrollToTop?: boolean; replace?: boolean };

export default function WorksListPage() {
  const [params, setParams] = useSearchParams();
  const q = params.get('q') ?? '';
  const tags = useMemo(() => parseTags(params.get('tags')), [params]);
  // 除外タグ ID リスト
  const excludeTags = useMemo(() => parseTags(params.get('exclude_tags')), [params]);
  const circle = params.get('circle') ?? '';
  const series = params.get('series') ?? '';
  const workType = params.get('work_type') ?? '';
  const ageRating = params.get('age_rating') ?? '';
  const ratingParam = normalizeRatingFilter(params.get('rating'));
  const ratingFilter = ratingParam === '' ? undefined : ratingParam === 'none' ? 'none' : Number(ratingParam);
  const favorite = params.get('favorite') === '1';
  const sort = (params.get('sort') as SortKey) || 'purchase_date';
  // デフォルトは降順。降順のときは URL にパラメータを付けない(#59)
  const order = (params.get('order') as SortOrder) || 'desc';
  const page = Number(params.get('page') ?? '1') || 1;
  // 検索キーワードの include/exclude チップ表示用(#29)
  const searchTerms = useMemo(() => parseSearchTerms(q), [q]);

  const [qInput, setQInput] = useState(q);
  // 毎レンダーで最新の q を ref に書き込む。デバウンスエフェクトの deps に
  // q を入れると URL 更新直後に無駄な再実行が生じるため、ref 経由で参照する。
  const qRef = useRef(q);
  // eslint-disable-next-line react-hooks/refs -- 上のコメントの通り、最新の q を常に反映させるための意図的なレンダー中書き込み
  qRef.current = q;
  const [data, setData] = useState<WorksResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // fetch 失敗時の再試行用。increment するとデータ取得 effect が再実行される(issue #70)
  const [retryNonce, setRetryNonce] = useState(0);
  const [drawerOpen, setDrawerOpen] = useState(false);
  // ページ番号入力欄(#35)。入力中の生値を保持し、Enter/blur で確定する
  const [pageInput, setPageInput] = useState(String(page));
  // 全タグ一覧を共有ストアから取得し、id→name の Map として参照する。
  // TagFacetPanel でも同じストアを使うため HTTP リクエストは 1 回だけ発火する。
  const tagNames = useTagNameMap();

  // 検索ボックスは URL と同期(戻る等で反映)
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 上のコメントの通り、URL 変化を入力欄に反映する意図的な setState
    setQInput(q);
  }, [q]);

  // ページ番号入力欄も URL と同期(前へ/次へ・戻る等で反映)
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 上のコメントの通り、URL 変化を入力欄に反映する意図的な setState
    setPageInput(String(page));
  }, [page]);

  // setParams の関数形式を使い、タイマー発火時に最新の params を参照する。
  // scrollToTop が true のとき先頭に戻す(結果セットが大きく変わる操作で指定)。
  const update = useCallback(
    (mut: (p: URLSearchParams) => void, opts?: UpdateOptions) => {
      setParams((prev) => {
        const next = new URLSearchParams(prev);
        mut(next);
        return next;
      }, { replace: opts?.replace ?? false });
      if (opts?.scrollToTop) window.scrollTo(0, 0);
    },
    [setParams],
  );

  // 入力を 300ms デバウンスして URL に反映(リアルタイム検索)
  useEffect(() => {
    if (qInput === qRef.current) return;
    const t = setTimeout(() => {
      update((p) => {
        if (qInput) p.set('q', qInput);
        else p.delete('q');
        p.delete('page');
      }, { scrollToTop: true, replace: true });
    }, 300);
    return () => clearTimeout(t);
  }, [qInput, update]);

  // マウント時にタグ一覧をプリフェッチ(ストアがキャッシュするので 1 回のみ発火)
  useEffect(() => {
    useTagStore.getState().ensureLoaded();
  }, []);

  useEffect(() => {
    const ac = new AbortController();
    // fetch 開始前にローディング表示へ切り替える意図的な setState(データ取得 effect の定型)
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    setError(null);
    fetchWorks(
      {
        q,
        tags,
        excludeTags,
        circle: circle || undefined,
        series: series || undefined,
        workType: workType || undefined,
        ageRating: ageRating || undefined,
        rating: ratingFilter,
        favorite: favorite || undefined,
        sort,
        order,
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
  }, [q, tags, excludeTags, circle, series, workType, ageRating, ratingFilter, favorite, sort, order, page, retryNonce]);

  // URL が変わるたびに検索クエリを sessionStorage に保存(詳細→一覧の戻り先に使う)
  useEffect(() => {
    saveListSearch(params.toString());
  }, [params]);

  useBodyScrollLock(drawerOpen);

  const submitSearch = (e: React.FormEvent) => {
    e.preventDefault();
    update((p) => {
      if (qInput) p.set('q', qInput);
      else p.delete('q');
      p.delete('page');
    }, { scrollToTop: true });
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
    }, { scrollToTop: true });
  };

  // チップの ✕ クリック: 3 状態サイクルを経由せず、含む/除外を直接解除する
  const clearTagParam = (key: 'tags' | 'exclude_tags', tagId: number) => {
    update((p) => {
      const set = new Set(parseTags(p.get(key)));
      set.delete(tagId);
      if (set.size > 0) p.set(key, [...set].join(','));
      else p.delete(key);
      p.delete('page');
    }, { scrollToTop: true });
  };

  const setCircle = (name: string) => {
    update((p) => {
      if (name) p.set('circle', name);
      else p.delete('circle');
      p.delete('page');
    }, { scrollToTop: true });
  };

  const setWorkType = (value: string) => {
    update((p) => {
      if (value) p.set('work_type', value);
      else p.delete('work_type');
      p.delete('page');
    }, { scrollToTop: true });
  };

  const setAgeRating = (value: string) => {
    update((p) => {
      if (value) p.set('age_rating', value);
      else p.delete('age_rating');
      p.delete('page');
    }, { scrollToTop: true });
  };

  const setRatingFilter = (value: string) => {
    update((p) => {
      if (value) p.set('rating', value);
      else p.delete('rating');
      p.delete('page');
    }, { scrollToTop: true });
  };

  const renderOptionFilter = (
    label: string,
    value: string,
    options: string[],
    onChange: (value: string) => void,
  ) => (
    <select
      className={styles.filterSelect}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      aria-label={label}
    >
      <option value="">{label}: すべて</option>
      {options.map((option) => (
        <option key={option} value={option}>
          {option}
        </option>
      ))}
    </select>
  );

  const renderFacetSections = (onPicked?: () => void) => (
    <>
      <SidebarSection title="種別">
        {renderOptionFilter('種別', workType, WORK_TYPE_OPTIONS, (value) => {
          setWorkType(value);
          onPicked?.();
        })}
      </SidebarSection>
      <SidebarSection title="年齢指定">
        {renderOptionFilter('年齢指定', ageRating, AGE_RATING_OPTIONS, (value) => {
          setAgeRating(value);
          onPicked?.();
        })}
      </SidebarSection>
      <SidebarSection title="評価">
        <select
          className={styles.filterSelect}
          value={ratingParam}
          onChange={(e) => {
            setRatingFilter(e.target.value);
            onPicked?.();
          }}
          aria-label="評価"
        >
          <option value="">評価: すべて</option>
          {RATING_FILTER_OPTIONS.map((option) => (
            <option key={option} value={option}>
              {formatRatingFilter(option)}
            </option>
          ))}
        </select>
      </SidebarSection>
      <SidebarSection title="タグ">
        <TagFacetPanel
          selected={tags}
          excluded={excludeTags}
          onToggle={toggleTag}
          categories={PRIMARY_TAG_CATEGORIES}
          defaultExpandedCategories={DEFAULT_EXPANDED_TAG_CATEGORIES}
        />
      </SidebarSection>
      <SidebarSection title="サークル">
        <CircleFacetPanel
          selected={circle}
          onSelect={(name) => {
            setCircle(name);
            onPicked?.();
          }}
        />
      </SidebarSection>
      <SidebarSection title="声優">
        <TagFacetPanel
          selected={tags}
          excluded={excludeTags}
          onToggle={toggleTag}
          categories={['voice_actor']}
          defaultExpandedCategories={['voice_actor']}
          showSearch={false}
        />
      </SidebarSection>
      <SidebarSection title="シナリオ">
        <TagFacetPanel
          selected={tags}
          excluded={excludeTags}
          onToggle={toggleTag}
          categories={['scenario']}
          defaultExpandedCategories={['scenario']}
          showSearch={false}
        />
      </SidebarSection>
      <SidebarSection title="イラスト">
        <TagFacetPanel
          selected={tags}
          excluded={excludeTags}
          onToggle={toggleTag}
          categories={['illustration']}
          defaultExpandedCategories={['illustration']}
          showSearch={false}
        />
      </SidebarSection>
      <SidebarSection title="音楽">
        <TagFacetPanel
          selected={tags}
          excluded={excludeTags}
          onToggle={toggleTag}
          categories={['music']}
          defaultExpandedCategories={['music']}
          showSearch={false}
        />
      </SidebarSection>
    </>
  );

  const toggleFavorite = () => {
    update((p) => {
      if (p.get('favorite') === '1') p.delete('favorite');
      else p.set('favorite', '1');
      p.delete('page');
    }, { scrollToTop: true });
  };

  const setSort = (s: SortKey) => {
    update((p) => {
      p.set('sort', s);
      p.delete('page');
    }, { scrollToTop: true });
  };

  // 昇順/降順トグル(#59)。デフォルトの desc は URL に残さない
  const toggleOrder = () => {
    update((p) => {
      const next: SortOrder = order === 'desc' ? 'asc' : 'desc';
      if (next === 'desc') p.delete('order');
      else p.set('order', next);
      p.delete('page');
    }, { scrollToTop: true });
  };

  // 検索語チップの ✕ クリック: 該当語だけを q から除去する(#29)
  const removeSearchTerm = (term: string, exclude: boolean) => {
    update((p) => {
      const target = exclude ? `-${term}` : term;
      const remaining = splitQuery(p.get('q') ?? '').filter((t) => t !== target);
      if (remaining.length > 0) p.set('q', remaining.join(' '));
      else p.delete('q');
      p.delete('page');
    }, { scrollToTop: true });
  };

  const goPage = (n: number) => {
    // ページ送りでも先頭に戻す(update 経由に統一し直接呼び出しを排除)
    const clamped = Math.min(Math.max(1, n), totalPages);
    update((p) => p.set('page', String(clamped)), { scrollToTop: true });
  };

  // ページ番号入力欄の確定処理(#35)。範囲外はクランプし、変化がなければ
  // 表示だけを揃えて goPage は呼ばない(無駄な再フェッチを避ける)
  const commitPageInput = () => {
    const n = Math.round(Number(pageInput));
    const clamped = Number.isFinite(n) ? Math.min(Math.max(1, n), totalPages) : page;
    setPageInput(String(clamped));
    if (clamped !== page) goPage(clamped);
  };

  const clearParam = (key: 'circle' | 'series' | 'work_type' | 'age_rating' | 'rating') => {
    update((p) => {
      p.delete(key);
      p.delete('page');
    }, { scrollToTop: true });
  };

  // q を含む全フィルタを一括削除し、入力欄もリセットする
  const clearAllFilters = () => {
    update((p) => {
      p.delete('q');
      p.delete('tags');
      p.delete('exclude_tags');
      p.delete('circle');
      p.delete('series');
      p.delete('work_type');
      p.delete('age_rating');
      p.delete('rating');
      p.delete('favorite');
      p.delete('page');
    }, { scrollToTop: true });
    // URL→qInput 同期の useEffect は次レンダー待ちになるため、入力欄は即時クリア
    setQInput('');
  };

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.limit)) : 1;
  // タグ絞り込みボタンのバッジ: 含む + 除外の合計
  const tagFilterCount = tags.length + excludeTags.length;

  // chips セクションの表示条件。検索キーワードも include/exclude チップとして表示する(#29)。
  const hasChips =
    tags.length > 0 ||
    excludeTags.length > 0 ||
    !!circle ||
    !!series ||
    !!workType ||
    !!ageRating ||
    !!ratingParam ||
    searchTerms.include.length > 0 ||
    searchTerms.exclude.length > 0;

  return (
    <div className={styles.layout}>
      {/* PC: 左サイドパネル / スマホ: 非表示 */}
      <aside className={styles.sidebar}>
        {renderFacetSections()}
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
            <button
              type="button"
              className={`btn ${styles.favoriteToggle} ${favorite ? styles.favoriteToggleActive : ''}`}
              onClick={toggleFavorite}
              aria-pressed={favorite}
            >
              ★ お気に入りのみ
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
            <button
              type="button"
              className="btn"
              aria-label={order === 'desc' ? '昇順に切り替え' : '降順に切り替え'}
              onClick={toggleOrder}
            >
              {order === 'desc' ? '降順 ↓' : '昇順 ↑'}
            </button>
          </div>

          {hasChips && (
            <div className={styles.chips}>
              {searchTerms.include.map((term) => (
                <button
                  key={`kw-${term}`}
                  className={styles.chip}
                  onClick={() => removeSearchTerm(term, false)}
                  title="クリックで解除"
                >
                  <span className={styles.chipKind}>検索語</span>
                  {term}
                  <span className={styles.chipX}>✕</span>
                </button>
              ))}
              {searchTerms.exclude.map((term) => (
                <button
                  key={`kw-excl-${term}`}
                  className={`${styles.chip} ${styles.chipExcluded}`}
                  onClick={() => removeSearchTerm(term, true)}
                  title="クリックで解除"
                >
                  −{term}
                  <span className={styles.chipX}>✕</span>
                </button>
              ))}
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
              {workType && (
                <button
                  className={styles.chip}
                  onClick={() => clearParam('work_type')}
                  title="クリックで解除"
                >
                  <span className={styles.chipKind}>種別</span>
                  {workType}
                  <span className={styles.chipX}>✕</span>
                </button>
              )}
              {ageRating && (
                <button
                  className={styles.chip}
                  onClick={() => clearParam('age_rating')}
                  title="クリックで解除"
                >
                  <span className={styles.chipKind}>年齢指定</span>
                  {ageRating}
                  <span className={styles.chipX}>✕</span>
                </button>
              )}
              {ratingParam && (
                <button
                  className={styles.chip}
                  onClick={() => clearParam('rating')}
                  title="クリックで解除"
                >
                  <span className={styles.chipKind}>評価</span>
                  {formatRatingFilter(ratingParam)}
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
              <button className={styles.clearAll} onClick={clearAllFilters}>
                全てクリア
              </button>
            </div>
          )}
        </div>

        {loading ? (
          <div className={styles.center}>
            <div className="spinner" />
          </div>
        ) : error ? (
          <FetchError message={error} onRetry={() => setRetryNonce((n) => n + 1)} />
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
                  favorited={w.favorited}
                  rating={w.rating}
                />
              ))}
            </div>

            {totalPages > 1 && (
              <div className={styles.pager}>
                <button
                  className="btn"
                  onClick={() => goPage(1)}
                  disabled={page <= 1}
                >
                  ≪ 最初へ
                </button>
                <button
                  className="btn"
                  onClick={() => goPage(page - 1)}
                  disabled={page <= 1}
                >
                  前へ
                </button>
                <label className={styles.pageInfo}>
                  <input
                    type="number"
                    className={styles.pageInput}
                    min={1}
                    max={totalPages}
                    value={pageInput}
                    aria-label="ページ番号"
                    onChange={(e) => setPageInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        commitPageInput();
                      }
                    }}
                    onBlur={commitPageInput}
                  />
                  {` / ${totalPages} ページ`}
                </label>
                <button
                  className="btn"
                  onClick={() => goPage(page + 1)}
                  disabled={page >= totalPages}
                >
                  次へ
                </button>
                <button
                  className="btn"
                  onClick={() => goPage(totalPages)}
                  disabled={page >= totalPages}
                >
                  最後へ ≫
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
            {renderFacetSections(() => setDrawerOpen(false))}
          </div>
        </div>
      )}
    </div>
  );
}
