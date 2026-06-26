// タグファセット。カテゴリ別にグルーピングし、作品数付きで表示。
// 選択中タグはトグルできる。スマホはドロワー、PC はサイドパネルとして使われる。
//
// q が空の場合 → 共有 tagStore から取得(WorksListPage の ensureLoaded と重複なし)
// q が非空の場合 → サーバ側 LIKE フィルタを利用するため直接 fetchTags({q}) を呼ぶ
import { useEffect, useMemo, useState } from 'react';
import type { TagFacet } from '@/api/types';
import { fetchTags } from '@/api/client';
import { useTagStore } from '@/store/tagStore';
import styles from './TagFacetPanel.module.css';

interface Props {
  selected: number[];
  /** 除外中のタグ ID リスト */
  excluded: number[];
  onToggle: (tagId: number) => void;
}

const CATEGORY_LABELS: Record<string, string> = {
  genre: 'ジャンル',
  detail_genre: '詳細ジャンル',
  voice_actor: '声優',
  scenario: 'シナリオ',
  illustration: 'イラスト',
  music: '音楽',
  custom: 'カスタム',
};

const CATEGORY_ORDER = [
  'custom',
  'genre',
  'detail_genre',
  'voice_actor',
  'scenario',
  'illustration',
  'music',
];

export default function TagFacetPanel({ selected, excluded, onToggle }: Props) {
  const [searchQ, setSearchQ] = useState('');
  // q が非空のとき用のローカル検索結果
  const [localTags, setLocalTags] = useState<TagFacet[]>([]);
  const [localLoading, setLocalLoading] = useState(false);
  const [localFailed, setLocalFailed] = useState(false);

  // q が空のとき → 共有ストアから参照
  const storeItems = useTagStore((s) => s.items);
  const storeLoading = useTagStore((s) => s.loading);
  const storeFailed = useTagStore((s) => s.error !== null);

  // 折りたたみ中のカテゴリ。デフォルトは全て開。
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  const toggleCategory = (cat: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(cat)) next.delete(cat);
      else next.add(cat);
      return next;
    });
  };

  // q が空のとき → ストアに取得を委ねる(WorksListPage が ensureLoaded 済みのケースが多い)
  useEffect(() => {
    if (searchQ) return;
    const ac = new AbortController();
    useTagStore.getState().ensureLoaded(ac.signal);
    return () => ac.abort();
  }, [searchQ]);

  // q が非空のとき → サーバ側 LIKE フィルタで絞り込む(250ms デバウンス)
  useEffect(() => {
    if (!searchQ) return;
    const ac = new AbortController();
    const t = setTimeout(() => {
      setLocalLoading(true);
      setLocalFailed(false);
      fetchTags({ q: searchQ }, ac.signal)
        .then((d) => {
          setLocalTags(d.items);
          setLocalLoading(false);
        })
        .catch(() => {
          if (ac.signal.aborted) return;
          setLocalFailed(true);
          setLocalLoading(false);
        });
    }, 250);
    return () => {
      clearTimeout(t);
      ac.abort();
    };
  }, [searchQ]);

  // 表示に使う実際の tags/loading/failed を q の有無で切り替える
  const tags = searchQ ? localTags : storeItems;
  const loading = searchQ ? localLoading : storeLoading;
  const failed = searchQ ? localFailed : storeFailed;

  // #22: selected/excluded を Set に変換してレンダー内の includes(O(n²)) を排除する
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const excludedSet = useMemo(() => new Set(excluded), [excluded]);

  const grouped = useMemo(() => {
    const map = new Map<string, TagFacet[]>();
    for (const tag of tags) {
      const arr = map.get(tag.category) ?? [];
      arr.push(tag);
      map.set(tag.category, arr);
    }
    const cats = [...map.keys()].sort((a, b) => {
      const ia = CATEGORY_ORDER.indexOf(a);
      const ib = CATEGORY_ORDER.indexOf(b);
      return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
    });
    return cats.map((cat) => ({ category: cat, tags: map.get(cat)! }));
  }, [tags]);

  return (
    <div className={styles.panel}>
      <input
        className="input"
        type="search"
        placeholder="タグを絞り込み"
        value={searchQ}
        onChange={(e) => setSearchQ(e.target.value)}
      />
      {loading ? (
        <div className={styles.center}>
          <div className="spinner" />
        </div>
      ) : failed ? (
        <p className="error">タグ一覧の読み込みに失敗しました</p>
      ) : grouped.length === 0 ? (
        <p className="faint">タグがありません</p>
      ) : (
        grouped.map((g) => {
          const isCollapsed = collapsed.has(g.category);
          const selCount = g.tags.filter((t) => selectedSet.has(t.id)).length;
          const exclCount = g.tags.filter((t) => excludedSet.has(t.id)).length;
          const activeCount = selCount + exclCount;
          return (
            <div key={g.category} className={styles.group}>
              <button
                type="button"
                className={styles.groupTitle}
                onClick={() => toggleCategory(g.category)}
                aria-expanded={!isCollapsed}
              >
                <span className={styles.caret}>{isCollapsed ? '▶' : '▼'}</span>
                <span>{CATEGORY_LABELS[g.category] ?? g.category}</span>
                <span className={styles.groupCount}>
                  {activeCount > 0 ? `${activeCount}/${g.tags.length}` : g.tags.length}
                </span>
              </button>
              {!isCollapsed && (
                <ul className={styles.tagList}>
                  {g.tags.map((tag) => {
                    const isSel = selectedSet.has(tag.id);
                    const isExcl = excludedSet.has(tag.id);
                    return (
                      <li key={tag.id}>
                        <button
                          className={`${styles.tag} ${isSel ? styles.tagSel : ''} ${isExcl ? styles.tagExcluded : ''}`}
                          onClick={() => onToggle(tag.id)}
                          title={isSel ? '含む(クリックで除外)' : isExcl ? '除外中(クリックで解除)' : 'クリックで絞り込み'}
                        >
                          {isExcl && <span className={styles.excludePrefix}>−</span>}
                          <span className={styles.tagName}>{tag.name}</span>
                          <span className={styles.count}>{tag.work_count}</span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          );
        })
      )}
    </div>
  );
}
