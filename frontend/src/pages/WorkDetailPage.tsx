import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { Tag, WorkDetail } from '@/api/types';
import {
  addCustomTag,
  fetchWork,
  refreshThumbnail,
  removeTag,
  setWorkHidden,
  thumbnailUrl,
} from '@/api/client';
import FileBrowser from '@/components/FileBrowser';
import { formatDateTime } from '@/utils/format';
import { listBackPath } from '@/lib/listSearchMemory';
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
  const [tagError, setTagError] = useState<string | null>(null);
  // 非表示操作用
  const [hideBusy, setHideBusy] = useState(false);
  const [hideError, setHideError] = useState<string | null>(null);
  // サムネ再生成が走ったら src にクエリを足して再読込させる
  const [thumbBust, setThumbBust] = useState<number | null>(null);

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

  // 詳細表示時にサムネ再生成チェックを裏で実行。差し替わったら img を再読込。
  useEffect(() => {
    if (!Number.isFinite(workId)) return;
    let active = true;
    setThumbBust(null);
    refreshThumbnail(workId)
      .then((r) => {
        if (active && r?.refreshed) setThumbBust(Date.now());
      })
      .catch(() => {
        // fire-and-forget。失敗しても何もしない
      });
    return () => {
      active = false;
    };
  }, [workId]);

  const onTagClick = (tag: Tag) => {
    navigate(`/?tags=${tag.id}`);
  };

  const onAddTag = async (e: React.FormEvent) => {
    e.preventDefault();
    const name = newTag.trim();
    if (!name || !work) return;
    setBusy(true);
    setTagError(null);
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
      setTagError(err instanceof Error ? err.message : 'タグ追加に失敗しました');
    } finally {
      setBusy(false);
    }
  };

  const onRemoveTag = async (tag: Tag) => {
    if (!work) return;
    setTagError(null);
    try {
      await removeTag(work.id, tag.id);
      setWork((w) => (w ? { ...w, tags: w.tags.filter((t) => t.id !== tag.id) } : w));
    } catch (err) {
      setTagError(err instanceof Error ? err.message : 'タグ削除に失敗しました');
    }
  };

  // この作品を非表示にして一覧へ戻る
  const onHide = async () => {
    if (!work) return;
    if (!window.confirm('この作品を非表示にしますか?')) return;
    setHideBusy(true);
    setHideError(null);
    try {
      await setWorkHidden(work.id, true);
      navigate(listBackPath());
    } catch (err) {
      setHideError(err instanceof Error ? err.message : '非表示設定に失敗しました');
      setHideBusy(false);
    }
  };

  // 非表示を解除してローカル state を更新
  const onUnhide = async () => {
    if (!work) return;
    setHideBusy(true);
    setHideError(null);
    try {
      await setWorkHidden(work.id, false);
      setWork((w) => (w ? { ...w, hidden: false } : w));
    } catch (err) {
      setHideError(err instanceof Error ? err.message : '非表示解除に失敗しました');
    } finally {
      setHideBusy(false);
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
        <Link to={listBackPath()} className={styles.back}>
          ← ライブラリ
        </Link>
        <p className="muted">{error ?? '作品が見つかりません'}</p>
      </div>
    );
  }

  const meta: {
    label: string;
    value: string | null | undefined;
    to?: string;
    href?: string;
  }[] = [
    {
      label: 'RJ番号',
      value: work.rj_number,
      href: work.rj_number
        ? `https://www.dlsite.com/maniax/work/=/product_id/${encodeURIComponent(work.rj_number)}.html`
        : undefined,
    },
    {
      label: 'サークル',
      value: work.circle,
      to: work.circle ? `/?circle=${encodeURIComponent(work.circle)}` : undefined,
    },
    {
      label: 'シリーズ',
      value: work.series_name,
      to: work.series_name
        ? `/?series=${encodeURIComponent(work.series_name)}`
        : undefined,
    },
    { label: '種別', value: work.work_type },
    { label: '年齢指定', value: work.age_rating },
    { label: '形式', value: work.file_format },
    { label: 'サイズ', value: work.file_size_text },
    { label: '購入日', value: formatDateTime(work.purchase_date) },
  ];

  return (
    <div className={styles.page}>
      <Link to={listBackPath()} className={styles.back}>
        ← ライブラリ
      </Link>

      <div className={styles.header}>
        <img
          className={styles.thumb}
          src={thumbBust ? `${thumbnailUrl(work.id)}?t=${thumbBust}` : thumbnailUrl(work.id)}
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
                  <dd>
                    {m.href ? (
                      <a
                        href={m.href}
                        target="_blank"
                        rel="noopener noreferrer"
                        className={styles.metaLink}
                      >
                        {m.value} ↗
                      </a>
                    ) : m.to ? (
                      <Link to={m.to} className={styles.metaLink}>
                        {m.value}
                      </Link>
                    ) : (
                      m.value
                    )}
                  </dd>
                </div>
              ))}
          </dl>
        </div>
      </div>

      {/* 非表示コントロール */}
      <div className={styles.hideControl}>
        {work.hidden ? (
          <>
            <span className={styles.hiddenBadge}>非表示</span>
            <button
              type="button"
              className="btn"
              onClick={onUnhide}
              disabled={hideBusy}
            >
              {hideBusy ? '解除中…' : '非表示を解除'}
            </button>
          </>
        ) : (
          <button
            type="button"
            className={`btn ${styles.hideBtn}`}
            onClick={onHide}
            disabled={hideBusy}
          >
            {hideBusy ? '処理中…' : 'この作品を非表示にする'}
          </button>
        )}
        {hideError && <p className="error">{hideError}</p>}
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
        {tagError && <p className="error">{tagError}</p>}
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
