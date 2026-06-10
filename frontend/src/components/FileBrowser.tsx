// 作品詳細下部のファイルブラウザ。
// GET entries?path= の結果をそのまま表示(サーバが自然順ソート済みなので順序を保つ)。
// ファイルタップで media_kind に応じて プレイヤー / 画像ビューア / 動画 / テキスト を起動。
import { useEffect, useState } from 'react';
import type { EntriesResponse, Entry } from '@/api/types';
import { fetchEntries } from '@/api/client';
import { usePlayerStore } from '@/store/playerStore';
import { useOverlayStore } from '@/store/overlayStore';
import { formatBytes, joinPath, pathCrumbs } from '@/utils/format';
import styles from './FileBrowser.module.css';

interface Props {
  workId: number;
  workTitle: string;
}

const KIND_ICON: Record<string, string> = {
  audio: '♪',
  video: '▶',
  image: '🖼',
  text: '𝐓',
  other: '·',
  '': '·',
};

export default function FileBrowser({ workId, workTitle }: Props) {
  const [path, setPath] = useState('');
  const [data, setData] = useState<EntriesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    fetchEntries(workId, path, ac.signal)
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
  }, [workId, path]);

  const openEntry = (entry: Entry) => {
    if (entry.is_dir) {
      setPath(joinPath(path, entry.name));
      return;
    }
    const entries = data?.entries ?? [];
    switch (entry.media_kind) {
      case 'audio':
        usePlayerStore.getState().startFromEntries({
          workId,
          workTitle,
          dir: path,
          entries,
          startName: entry.name,
        });
        break;
      case 'image':
        useOverlayStore.getState().openImage({
          workId,
          dir: path,
          entries,
          startName: entry.name,
        });
        break;
      case 'video':
        useOverlayStore.getState().openVideo({
          workId,
          workTitle,
          path: joinPath(path, entry.name),
          name: entry.name,
        });
        break;
      case 'text':
        useOverlayStore.getState().openText({
          workId,
          path: joinPath(path, entry.name),
          name: entry.name,
        });
        break;
      default:
        // other は何もしない(将来ダウンロードリンク等)
        break;
    }
  };

  const crumbs = pathCrumbs(path);

  return (
    <section className={styles.browser}>
      <h2 className={styles.heading}>ファイル</h2>

      <nav className={styles.crumbs} aria-label="パンくず">
        <button
          className={styles.crumb}
          onClick={() => setPath('')}
          disabled={path === ''}
        >
          ホーム
        </button>
        {crumbs.map((c) => (
          <span key={c.path} className={styles.crumbWrap}>
            <span className={styles.sep}>/</span>
            <button
              className={styles.crumb}
              onClick={() => setPath(c.path)}
              disabled={c.path === path}
            >
              {c.name}
            </button>
          </span>
        ))}
      </nav>

      {loading ? (
        <div className={styles.center}>
          <div className="spinner" />
        </div>
      ) : error ? (
        <p className="muted">{error}</p>
      ) : !data || data.entries.length === 0 ? (
        <p className="faint">(空のフォルダ)</p>
      ) : (
        <ul className={styles.list}>
          {data.entries.map((entry) => (
            <li key={entry.name}>
              <button className={styles.entry} onClick={() => openEntry(entry)}>
                <span className={`${styles.icon} ${entry.is_dir ? styles.dir : ''}`}>
                  {entry.is_dir ? '📁' : (KIND_ICON[entry.media_kind] ?? '·')}
                </span>
                <span className={styles.name}>{entry.name}</span>
                {!entry.is_dir && entry.size > 0 && (
                  <span className={styles.size}>{formatBytes(entry.size)}</span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
