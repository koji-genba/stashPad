import { useRef, useState } from 'react';
import type {
  ImportResult,
  ScanResult,
  TagCleanupResult,
  ThumbnailRebuildResult,
} from '@/api/types';
import {
  cleanupTags,
  importCsv,
  rebuildThumbnails,
  runScan,
} from '@/api/client';
import styles from './SettingsPage.module.css';

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

  const [rebuilding, setRebuilding] = useState(false);
  const [rebuildResult, setRebuildResult] = useState<ThumbnailRebuildResult | null>(
    null,
  );
  const [rebuildError, setRebuildError] = useState<string | null>(null);

  const onImport = async () => {
    if (!csvFile) return;
    setImporting(true);
    setImportResult(null);
    setImportError(null);
    try {
      const result = await importCsv(csvFile);
      setImportResult(result);
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
    } catch (e) {
      setCleanupError(e instanceof Error ? e.message : 'タグ削除に失敗しました');
    } finally {
      setCleaning(false);
    }
  };

  const onRebuildThumbnails = async () => {
    setRebuilding(true);
    setRebuildResult(null);
    setRebuildError(null);
    try {
      const result = await rebuildThumbnails();
      setRebuildResult(result);
    } catch (e) {
      setRebuildError(
        e instanceof Error ? e.message : 'サムネイル再生成に失敗しました',
      );
    } finally {
      setRebuilding(false);
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
          {rebuildResult && (
            <span className={styles.maintResult}>
              確認 <b>{rebuildResult.checked}</b> / 再生成{' '}
              <b>{rebuildResult.regenerated}</b>
            </span>
          )}
        </div>
        {rebuilding && (
          <p className="faint" style={{ marginTop: 4 }}>
            作品数によっては時間がかかります。
          </p>
        )}
        {rebuildError && <p className={styles.error}>{rebuildError}</p>}
      </section>
    </div>
  );
}
