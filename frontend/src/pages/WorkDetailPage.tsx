import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { Tag, WorkDetail } from '@/api/types';
import {
  addCustomTag,
  fetchWork,
  removeTag,
  thumbnailUrl,
} from '@/api/client';
import FileBrowser from '@/components/FileBrowser';
import { formatDateTime } from '@/utils/format';
import styles from './WorkDetailPage.module.css';

const CATEGORY_LABELS: Record<string, string> = {
  genre: 'ジャンル',
  detail_genre: '詳細',
  voice_actor: '声優',
  scenario: 'シナリオ',
  illustration: 'イラスト',
  music: '音楽',
  custom: 'カスタム',
};

export default function WorkDetailPage() {
  const { id } = useParams();
  const workId = Number(id);
  const navigate = useNavigate();

  const [work, setWork] = useState<WorkDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [newTag, setNewTag] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!Number.isFinite(workId)) return;
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    fetchWork(workId, ac.signal)
      .then((d) => {
        setWork(d);
        setLoading(false);
      })
      .catch((e: unknown) => {
        if (ac.signal.aborted) return;
        setError(e instanceof Error ? e.message : '読み込み失敗');
        setLoading(false);
      });
    return () => ac.abort();
  }, [workId]);

  const onTagClick = (tag: Tag) => {
    navigate(`/?tags=${tag.id}`);
  };

  const onAddTag = async (e: React.FormEvent) => {
    e.preventDefault();
    const name = newTag.trim();
    if (!name || !work) return;
    setBusy(true);
    try {
      const created = await addCustomTag(work.id, name);
      // 重複追加を防ぎつつ反映
      setWork((w) =>
        w && !w.tags.some((t) => t.id === created.id)
          ? { ...w, tags: [...w.tags, created] }
          : w,
      );
      setNewTag('');
    } catch (err) {
      alert(err instanceof Error ? err.message : 'タグ追加に失敗しました');
    } finally {
      setBusy(false);
    }
  };

  const onRemoveTag = async (tag: Tag) => {
    if (!work) return;
    try {
      await removeTag(work.id, tag.id);
      setWork((w) => (w ? { ...w, tags: w.tags.filter((t) => t.id !== tag.id) } : w));
    } catch (err) {
      alert(err instanceof Error ? err.message : 'タグ削除に失敗しました');
    }
  };

  if (loading) {
    return (
      <div className={styles.center}>
        <div className="spinner" />
      </div>
    );
  }
  if (error || !work) {
    return (
      <div className={styles.page}>
        <Link to="/" className={styles.back}>
          ← ライブラリ
        </Link>
        <p className="muted">{error ?? '作品が見つかりません'}</p>
      </div>
    );
  }

  const meta: { label: string; value: string | null | undefined }[] = [
    { label: 'RJ番号', value: work.rj_number },
    { label: 'サークル', value: work.circle },
    { label: 'シリーズ', value: work.series_name },
    { label: '種別', value: work.work_type },
    { label: '年齢指定', value: work.age_rating },
    { label: '形式', value: work.file_format },
    { label: 'サイズ', value: work.file_size_text },
    { label: '購入日', value: formatDateTime(work.purchase_date) },
  ];

  return (
    <div className={styles.page}>
      <Link to="/" className={styles.back}>
        ← ライブラリ
      </Link>

      <div className={styles.header}>
        <img
          className={styles.thumb}
          src={thumbnailUrl(work.id)}
          alt=""
          onError={(e) => {
            e.currentTarget.style.visibility = 'hidden';
          }}
        />
        <div className={styles.headInfo}>
          <h1 className={styles.title}>{work.title}</h1>
          {!work.has_folder && <span className={styles.notImported}>未取込</span>}
          <dl className={styles.meta}>
            {meta
              .filter((m) => m.value)
              .map((m) => (
                <div key={m.label} className={styles.metaRow}>
                  <dt>{m.label}</dt>
                  <dd>{m.value}</dd>
                </div>
              ))}
          </dl>
        </div>
      </div>

      <section className={styles.tagsSection}>
        <h2 className={styles.heading}>タグ</h2>
        <div className={styles.tags}>
          {work.tags.length === 0 && <span className="faint">タグなし</span>}
          {work.tags.map((tag) => (
            <span
              key={tag.id}
              className={`${styles.tag} ${
                tag.category === 'custom' ? styles.tagCustom : ''
              }`}
            >
              <button className={styles.tagBtn} onClick={() => onTagClick(tag)}>
                <span className={styles.tagCat}>
                  {CATEGORY_LABELS[tag.category] ?? tag.category}
                </span>
                {tag.name}
              </button>
              {tag.category === 'custom' && (
                <button
                  className={styles.tagRemove}
                  onClick={() => onRemoveTag(tag)}
                  aria-label={`${tag.name} を削除`}
                >
                  ✕
                </button>
              )}
            </span>
          ))}
        </div>

        <form className={styles.addTag} onSubmit={onAddTag}>
          <input
            className="input"
            type="text"
            placeholder="カスタムタグを追加(例: 睡眠用)"
            value={newTag}
            onChange={(e) => setNewTag(e.target.value)}
          />
          <button type="submit" className="btn" disabled={busy || !newTag.trim()}>
            追加
          </button>
        </form>
      </section>

      {work.has_folder ? (
        <FileBrowser workId={work.id} workTitle={work.title} />
      ) : (
        <p className={styles.noFolder}>
          この作品はフォルダが未取込のため、ファイルを表示できません。
        </p>
      )}
    </div>
  );
}
