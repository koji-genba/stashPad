// タグファセット。カテゴリ別にグルーピングし、作品数付きで表示。
// 選択中タグはトグルできる。スマホはドロワー、PC はサイドパネルとして使われる。
import { useEffect, useMemo, useState } from 'react';
import type { TagFacet } from '@/api/types';
import { fetchTags } from '@/api/client';
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
  const [tags, setTags] = useState<TagFacet[]>([]);
  const [q, setQ] = useState('');
  const [loading, setLoading] = useState(true);
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

  useEffect(() => {
    const ac = new AbortController();
    const t = setTimeout(() => {
      setLoading(true);
      fetchTags(q ? { q } : {}, ac.signal)
        .then((d) => {
          setTags(d.items);
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
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      {loading ? (
        <div className={styles.center}>
          <div className="spinner" />
        </div>
      ) : grouped.length === 0 ? (
        <p className="faint">タグがありません</p>
      ) : (
        grouped.map((g) => {
          const isCollapsed = collapsed.has(g.category);
          const selCount = g.tags.filter((t) => selected.includes(t.id)).length;
          const exclCount = g.tags.filter((t) => excluded.includes(t.id)).length;
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
                    const isSel = selected.includes(tag.id);
                    const isExcl = excluded.includes(tag.id);
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
