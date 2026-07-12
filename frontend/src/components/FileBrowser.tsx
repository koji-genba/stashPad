// 作品詳細下部のファイルブラウザ。
// GET entries?path= の結果をそのまま表示(サーバが自然順ソート済みなので順序を保つ)。
// ファイルタップで media_kind に応じて プレイヤー / 画像ビューア / 動画 / テキスト を起動。
// audio 行の「⋮」ボタンからボトムシートでキュー操作が可能。
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router';
import { useStore } from 'zustand';
import type { EntriesResponse, Entry } from '@/api/types';
import { fetchEntries, fileUrl } from '@/api/client';
import { currentTrack, usePlayerStore } from '@/store/playerStore';
import { useOverlayStore } from '@/store/overlayStore';
import { formatBytes, joinPath, pathCrumbs } from '@/utils/format';
import FetchError from './FetchError';
import QueueActionSheet from './QueueActionSheet';
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
  const [searchParams, setSearchParams] = useSearchParams();
  const path = searchParams.get('path') ?? '';
  const [data, setData] = useState<EntriesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // fetch 失敗時の再試行用。increment するとデータ取得 effect が再実行される(issue #70)
  const [retryNonce, setRetryNonce] = useState(0);
  // ⋮ タップで開くシートの対象エントリ(null = 閉じている)
  const [sheetEntry, setSheetEntry] = useState<Entry | null>(null);

  // 「今このファイルを開いている」インジケータ用に再生 / オーバーレイ状態を購読。
  // audio はキューの現在トラック、video / text はオーバーレイの開いているファイル。
  // image はページ列で構造が異なるため対象外(issue #31)
  const playingTrack = useStore(usePlayerStore, currentTrack);
  const openVideo = useStore(useOverlayStore, (s) => s.video);
  const openText = useStore(useOverlayStore, (s) => s.text);

  const isCurrentMedia = (entry: Entry): boolean => {
    if (entry.is_dir) return false;
    const entryPath = joinPath(path, entry.name);
    switch (entry.media_kind) {
      case 'audio':
        return playingTrack?.workId === workId && playingTrack?.path === entryPath;
      case 'video':
        return openVideo?.workId === workId && openVideo?.path === entryPath;
      case 'text':
        return openText?.workId === workId && openText?.path === entryPath;
      default:
        return false;
    }
  };

  useEffect(() => {
    const ac = new AbortController();
    // fetch 開始前にローディング表示へ切り替える意図的な setState(データ取得 effect の定型)
    // eslint-disable-next-line react-hooks/set-state-in-effect
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
  }, [workId, path, retryNonce]);

  // ディレクトリ移動 / パンくず操作の共通入口。
  // useEffect 内の setLoading(true) より前にスピナーへ切替えておくことで、
  // 「新 path × 旧 entries」の組合せが 1 フレーム描画される stale render を防ぐ。
  // (旧 entries のまま isCurrentMedia が走ると、たまたまパスが一致したファイルに
  //  誤って強調が一瞬付いてしまうのを回避する)
  const navigateTo = (newPath: string) => {
    setLoading(true);
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (newPath) next.set('path', newPath);
      else next.delete('path');
      return next;
    });
  };

  const openEntry = (entry: Entry) => {
    if (entry.is_dir) {
      navigateTo(joinPath(path, entry.name));
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
        // 再生非対応(other / PDF 等)はダウンロード。
        // backend が Content-Disposition: attachment を返すので、
        // 一時的な <a> をクリックして SPA 状態を壊さずに保存させる。
        downloadFile(fileUrl(workId, joinPath(path, entry.name)), entry.name);
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
          onClick={() => navigateTo('')}
          disabled={path === ''}
        >
          ホーム
        </button>
        {crumbs.map((c) => (
          <span key={c.path} className={styles.crumbWrap}>
            <span className={styles.sep}>/</span>
            <button
              className={styles.crumb}
              onClick={() => navigateTo(c.path)}
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
        <FetchError message={error} onRetry={() => setRetryNonce((n) => n + 1)} />
      ) : !data || data.entries.length === 0 ? (
        <p className="faint">(空のフォルダ)</p>
      ) : (
        <ul className={styles.list}>
          {data.entries.map((entry) => {
            const isCurrent = isCurrentMedia(entry);
            return (
              <li key={entry.name} className={styles.row}>
                {/* 行本体ボタン(openEntry): ディレクトリ・全ファイル種別で従来通り */}
                <button
                  className={`${styles.entry} ${isCurrent ? styles.entryCurrent : ''}`}
                  onClick={() => openEntry(entry)}
                  aria-current={isCurrent ? 'true' : undefined}
                >
                  <span className={`${styles.icon} ${entry.is_dir ? styles.dir : ''}`}>
                    {entry.is_dir ? '📁' : (KIND_ICON[entry.media_kind] ?? '·')}
                  </span>
                  <span className={styles.name}>{entry.name}</span>
                  {!entry.is_dir && entry.size > 0 && (
                    <span className={styles.size}>{formatBytes(entry.size)}</span>
                  )}
                </button>
                {/* audio 行のみ ⋮ ボタンを表示(button ネスト回避のため兄弟 button) */}
                {!entry.is_dir && entry.media_kind === 'audio' && (
                  <button
                    type="button"
                    className={styles.menuBtn}
                    aria-label={`${entry.name} のキュー操作`}
                    onClick={(e) => {
                      e.stopPropagation();
                      setSheetEntry(entry);
                    }}
                  >
                    ⋮
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {/* キュー操作ボトムシート */}
      {sheetEntry && (
        <QueueActionSheet
          name={sheetEntry.name}
          input={{
            workId,
            workTitle,
            path: joinPath(path, sheetEntry.name),
            name: sheetEntry.name,
          }}
          onClose={() => setSheetEntry(null)}
        />
      )}
    </section>
  );
}

/** SPA の状態を壊さずにファイルを保存させる(一時 <a download> をクリック)。 */
function downloadFile(url: string, name: string) {
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
}
