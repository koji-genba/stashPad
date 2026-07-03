import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type {
  DeleteHistoryResult,
  ImportResult,
  ScanResult,
  TagCleanupResult,
  ThumbnailRebuildStatus,
  WorkListItem,
} from '@/api/types';
import {
  cleanupTags,
  deleteHistory,
  fetchThumbnailRebuildStatus,
  fetchWorks,
  importCsv,
  rebuildThumbnails,
  runScan,
  setWorkHidden,
} from '@/api/client';
import { useTagStore } from '@/store/tagStore';
import styles from './SettingsPage.module.css';

/** サムネイル一括再生成の進捗ポーリング間隔(ミリ秒) */
const REBUILD_POLL_INTERVAL_MS = 1000;

export default function SettingsPage() {
  const fileInput = useRef<HTMLInputElement>(null);
  const [csvFile, setCsvFile] = useState<File | null>(null);
  const [importing, setImporting] = useState(false);
  const [importResult, setImportResult] = useState<ImportResult | null>(null);
  const [importError, setImportError] = useState<string | null>(null);

  const [scanning, setScanning] = useState(false);
  const [scanResult, setScanResult] = useState<ScanResult | null>(null);
  const [scanError, setScanError] = useState<string | null>(null);

  const [cleaning, setCleaning] = useState(false);
  const [cleanupResult, setCleanupResult] = useState<TagCleanupResult | null>(null);
  const [cleanupError, setCleanupError] = useState<string | null>(null);

  const [clearingHistory, setClearingHistory] = useState(false);
  const [clearHistoryResult, setClearHistoryResult] = useState<DeleteHistoryResult | null>(
    null,
  );
  const [clearHistoryError, setClearHistoryError] = useState<string | null>(null);

  const [rebuilding, setRebuilding] = useState(false);
  const [rebuildStatus, setRebuildStatus] = useState<ThumbnailRebuildStatus | null>(
    null,
  );
  const [rebuildError, setRebuildError] = useState<string | null>(null);
  // ポーリング用の setInterval ハンドル。アンマウント時・完了時に必ずクリアする
  const rebuildPollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // 非表示作品一覧
  const [hiddenWorks, setHiddenWorks] = useState<WorkListItem[]>([]);
  const [hiddenTotal, setHiddenTotal] = useState(0);
  const [hiddenLoading, setHiddenLoading] = useState(true);
  const [hiddenError, setHiddenError] = useState<string | null>(null);
  // unhide 中の作品 ID セット
  const [unhidingIds, setUnhidingIds] = useState<Set<number>>(new Set());

  const onImport = async () => {
    if (!csvFile) return;
    setImporting(true);
    setImportResult(null);
    setImportError(null);
    try {
      const result = await importCsv(csvFile);
      setImportResult(result);
      // タグキャッシュが古くなるので強制再取得
      useTagStore.getState().refresh();
    } catch (e) {
      setImportError(e instanceof Error ? e.message : 'インポートに失敗しました');
    } finally {
      setImporting(false);
    }
  };

  const onScan = async () => {
    setScanning(true);
    setScanResult(null);
    setScanError(null);
    try {
      const result = await runScan();
      setScanResult(result);
      // スキャンで作品/タグの構成が変わりうるのでキャッシュ更新
      useTagStore.getState().refresh();
    } catch (e) {
      setScanError(e instanceof Error ? e.message : 'スキャンに失敗しました');
    } finally {
      setScanning(false);
    }
  };

  const onCleanupTags = async () => {
    setCleaning(true);
    setCleanupResult(null);
    setCleanupError(null);
    try {
      const result = await cleanupTags();
      setCleanupResult(result);
      // 未使用タグが消えるのでキャッシュ更新
      useTagStore.getState().refresh();
    } catch (e) {
      setCleanupError(e instanceof Error ? e.message : 'タグ削除に失敗しました');
    } finally {
      setCleaning(false);
    }
  };

  const onClearHistory = async () => {
    if (!window.confirm('再生履歴を全て削除しますか?この操作は取り消せません')) return;
    setClearingHistory(true);
    setClearHistoryResult(null);
    setClearHistoryError(null);
    try {
      const result = await deleteHistory();
      setClearHistoryResult(result);
    } catch (e) {
      setClearHistoryError(e instanceof Error ? e.message : '履歴削除に失敗しました');
    } finally {
      setClearingHistory(false);
    }
  };

  // ポーリングを止める(アンマウント時・完了時・エラー時に呼ぶ)
  const stopRebuildPolling = () => {
    if (rebuildPollRef.current !== null) {
      clearInterval(rebuildPollRef.current);
      rebuildPollRef.current = null;
    }
  };

  // running=false になるまで status を 1 秒間隔でポーリングする
  const startRebuildPolling = () => {
    stopRebuildPolling();
    rebuildPollRef.current = setInterval(async () => {
      try {
        const status = await fetchThumbnailRebuildStatus();
        setRebuildStatus(status);
        if (!status.running) {
          stopRebuildPolling();
          setRebuilding(false);
        }
      } catch (e) {
        stopRebuildPolling();
        setRebuilding(false);
        setRebuildError(e instanceof Error ? e.message : '進捗の取得に失敗しました');
      }
    }, REBUILD_POLL_INTERVAL_MS);
  };

  const onRebuildThumbnails = async () => {
    setRebuilding(true);
    setRebuildStatus(null);
    setRebuildError(null);
    try {
      const status = await rebuildThumbnails();
      setRebuildStatus(status);
      if (status.running) {
        startRebuildPolling();
      } else {
        setRebuilding(false);
      }
    } catch (e) {
      setRebuildError(
        e instanceof Error ? e.message : 'サムネイル再生成に失敗しました',
      );
      setRebuilding(false);
    }
  };

  // マウント時に既にジョブが実行中なら(他タブでの起動等)ポーリングを再開する
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const status = await fetchThumbnailRebuildStatus();
        if (cancelled || !status.running) return;
        setRebuilding(true);
        setRebuildStatus(status);
        startRebuildPolling();
      } catch {
        // 初回チェックの失敗は無視する(通常のボタン操作には影響しない)
      }
    })();
    return () => {
      cancelled = true;
      stopRebuildPolling();
    };
    // マウント時のみ実行する
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 非表示作品一覧を取得する
  // 設定画面では上限 200 件まで表示する。それを超える場合は注記でユーザに知らせる
  const HIDDEN_LIMIT = 200;
  const loadHiddenWorks = async () => {
    setHiddenLoading(true);
    setHiddenError(null);
    try {
      const result = await fetchWorks({ hidden: true, limit: HIDDEN_LIMIT });
      setHiddenWorks(result.items);
      setHiddenTotal(result.total);
    } catch (e) {
      setHiddenError(e instanceof Error ? e.message : '一覧の取得に失敗しました');
    } finally {
      setHiddenLoading(false);
    }
  };

  // マウント時に非表示作品一覧をロード
  useEffect(() => {
    void loadHiddenWorks();
  }, []);

  // 非表示を解除してリスト再取得
  const onUnhide = async (work: WorkListItem) => {
    setUnhidingIds((prev) => new Set(prev).add(work.id));
    try {
      await setWorkHidden(work.id, false);
      await loadHiddenWorks();
    } catch (e) {
      setHiddenError(e instanceof Error ? e.message : '非表示解除に失敗しました');
    } finally {
      setUnhidingIds((prev) => {
        const next = new Set(prev);
        next.delete(work.id);
        return next;
      });
    }
  };

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>設定</h1>

      {/* CSV インポート */}
      <section className={styles.section}>
        <h2 className={styles.heading}>CSV インポート</h2>
        <p className="muted">
          DLsite 作品情報 CSV をアップロードして作品メタデータ・タグを取り込みます(RJ
          番号で upsert)。
        </p>

        <div className={styles.fileRow}>
          <input
            ref={fileInput}
            type="file"
            accept=".csv,text/csv"
            className={styles.fileInput}
            onChange={(e) => {
              setCsvFile(e.target.files?.[0] ?? null);
              setImportResult(null);
              setImportError(null);
            }}
          />
          <button
            type="button"
            className="btn"
            onClick={() => fileInput.current?.click()}
            disabled={importing}
          >
            CSV を選択
          </button>
          <span className={styles.fileName}>
            {csvFile ? csvFile.name : '(未選択)'}
          </span>
        </div>

        <button
          type="button"
          className="btn btn-primary"
          onClick={onImport}
          disabled={!csvFile || importing}
        >
          {importing ? 'インポート中…' : 'インポート実行'}
        </button>

        {importError && <p className={styles.error}>{importError}</p>}
        {importResult && (
          <div className={styles.result}>
            <div className={styles.summary}>
              <span>
                新規 <b>{importResult.created}</b>
              </span>
              <span>
                更新 <b>{importResult.updated}</b>
              </span>
              <span>
                リンク <b>{importResult.linked}</b>
              </span>
            </div>
            {(importResult.errors ?? []).length > 0 && (
              <details className={styles.errors}>
                <summary>エラー {(importResult.errors ?? []).length} 件</summary>
                <ul>
                  {(importResult.errors ?? []).map((err, i) => (
                    <li key={i}>{err}</li>
                  ))}
                </ul>
              </details>
            )}
          </div>
        )}
      </section>

      {/* ライブラリスキャン */}
      <section className={styles.section}>
        <h2 className={styles.heading}>ライブラリスキャン</h2>
        <p className="muted">
          ライブラリルート直下のフォルダを走査し、作品の登録・リンク・サムネイル生成を行います。
        </p>
        <button
          type="button"
          className="btn btn-primary"
          onClick={onScan}
          disabled={scanning}
        >
          {scanning ? 'スキャン中…' : 'スキャン実行'}
        </button>

        {scanError && <p className={styles.error}>{scanError}</p>}
        {scanResult && (
          <div className={styles.result}>
            <div className={styles.summary}>
              <span>
                検出 <b>{scanResult.works_found}</b>
              </span>
              <span>
                新規登録 <b>{scanResult.newly_registered}</b>
              </span>
              <span>
                CSVリンク <b>{scanResult.linked_to_csv}</b>
              </span>
              <span>
                欠落 <b>{scanResult.missing_marked}</b>
              </span>
            </div>
          </div>
        )}
      </section>

      {/* メンテナンス */}
      <section className={styles.section}>
        <h2 className={styles.heading}>メンテナンス</h2>
        <p className="muted">
          ライブラリの不要データの整理やサムネイルの更新を行います。
        </p>

        <div className={styles.maintRow}>
          <button
            type="button"
            className="btn"
            onClick={onCleanupTags}
            disabled={cleaning}
          >
            {cleaning ? '削除中…' : '未使用タグを削除'}
          </button>
          {cleanupResult && (
            <span className={styles.maintResult}>
              削除 <b>{cleanupResult.deleted}</b> 件
            </span>
          )}
        </div>
        {cleanupError && <p className={styles.error}>{cleanupError}</p>}

        <div className={styles.maintRow}>
          <button
            type="button"
            className="btn"
            onClick={onRebuildThumbnails}
            disabled={rebuilding}
          >
            {rebuilding ? 'サムネイル再生成中…' : 'サムネイル再生成'}
          </button>
          {rebuilding && <span className="spinner" />}
          {rebuildStatus && (
            <span className={styles.maintResult}>
              {rebuildStatus.running ? (
                <>
                  確認 <b>{rebuildStatus.checked}</b>/<b>{rebuildStatus.total}</b> / 再生成{' '}
                  <b>{rebuildStatus.regenerated}</b>
                </>
              ) : (
                <>
                  確認 <b>{rebuildStatus.checked}</b> / 再生成{' '}
                  <b>{rebuildStatus.regenerated}</b>
                </>
              )}
            </span>
          )}
        </div>
        {rebuilding && (
          <p className="faint" style={{ marginTop: 4 }}>
            作品数によっては時間がかかります。
          </p>
        )}
        {rebuildError && <p className={styles.error}>{rebuildError}</p>}

        <div className={styles.maintRow}>
          <button
            type="button"
            className="btn"
            onClick={() => void onClearHistory()}
            disabled={clearingHistory}
          >
            {clearingHistory ? '削除中…' : '再生履歴を全削除'}
          </button>
          {clearHistoryResult && (
            <span className={styles.maintResult}>
              削除 <b>{clearHistoryResult.deleted}</b> 件
            </span>
          )}
        </div>
        {clearHistoryError && <p className={styles.error}>{clearHistoryError}</p>}
      </section>

      {/* 非表示の作品 */}
      <section className={styles.section}>
        <h2 className={styles.heading}>非表示の作品</h2>
        <p className="muted">
          非表示に設定した作品の一覧です。「解除」ボタンで通常表示に戻せます。
        </p>

        {hiddenLoading && <p className="muted">読み込み中…</p>}
        {hiddenError && <p className={styles.error}>{hiddenError}</p>}

        {!hiddenLoading && !hiddenError && hiddenWorks.length === 0 && (
          <p className="faint">非表示の作品はありません</p>
        )}

        {!hiddenLoading && hiddenWorks.length > 0 && (
          <ul className={styles.hiddenList}>
            {hiddenWorks.map((work) => (
              <li key={work.id} className={styles.hiddenItem}>
                <Link to={`/works/${work.id}`} className={styles.hiddenTitle}>
                  {work.title}
                  {work.rj_number && (
                    <span className={styles.hiddenRj}>{work.rj_number}</span>
                  )}
                </Link>
                <button
                  type="button"
                  className="btn"
                  onClick={() => void onUnhide(work)}
                  disabled={unhidingIds.has(work.id)}
                >
                  {unhidingIds.has(work.id) ? '解除中…' : '解除'}
                </button>
              </li>
            ))}
          </ul>
        )}
        {!hiddenLoading && hiddenTotal > hiddenWorks.length && (
          <p className="faint">
            {hiddenTotal} 件中 {hiddenWorks.length} 件を表示しています(上限 {HIDDEN_LIMIT} 件)
          </p>
        )}
      </section>
    </div>
  );
}
