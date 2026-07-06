// API クライアント。全レスポンス型は ./types に定義。
// エラーは一律 {"error": "..."} なので ApiRequestError に正規化する。
import { decodeTextBuffer } from '@/utils/textDecode';
import type {
  CirclesResponse,
  DeleteHistoryResult,
  EntriesResponse,
  HistoryParams,
  HistoryResponse,
  ImportMetadataResult,
  ImportResult,
  ScanResult,
  SortKey,
  SortOrder,
  Tag,
  TagCleanupResult,
  TagsResponse,
  ThumbnailRebuildStatus,
  ThumbnailRefreshResult,
  WorkDetail,
  WorksResponse,
} from './types';

export const API_BASE = '/api';

export class ApiRequestError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiRequestError';
    this.status = status;
  }
}

async function parseError(res: Response): Promise<never> {
  let message = `${res.status} ${res.statusText}`;
  try {
    const body = (await res.json()) as { error?: string };
    if (body && typeof body.error === 'string') message = body.error;
  } catch {
    // JSON でないボディは無視
  }
  throw new ApiRequestError(res.status, message);
}

async function getJson<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, { signal });
  if (!res.ok) return parseError(res);
  return (await res.json()) as T;
}

async function sendJson<T>(
  method: string,
  url: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) return parseError(res);
  // 201/204 でボディが無い場合に備える
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

// ---- 作品 ----

export interface WorksQuery {
  q?: string;
  tags?: number[];
  /** 除外するタグ ID リスト。これらのタグを持つ作品を結果から除く */
  excludeTags?: number[];
  /** サークル名の完全一致フィルタ */
  circle?: string;
  /** シリーズ名の完全一致フィルタ */
  series?: string;
  /** 種別の完全一致フィルタ */
  workType?: string;
  /** 年齢指定の完全一致フィルタ */
  ageRating?: string;
  /** 評価の完全一致フィルタ。none は未評価のみ */
  rating?: number | 'none';
  /** true のとき非表示作品のみを返す */
  hidden?: boolean;
  /** true のときお気に入り作品のみを返す */
  favorite?: boolean;
  sort?: SortKey;
  order?: SortOrder;
  page?: number;
  limit?: number;
}

export function fetchWorks(query: WorksQuery, signal?: AbortSignal): Promise<WorksResponse> {
  const params = new URLSearchParams();
  if (query.q) params.set('q', query.q);
  if (query.tags && query.tags.length > 0) params.set('tags', query.tags.join(','));
  if (query.excludeTags && query.excludeTags.length > 0)
    params.set('exclude_tags', query.excludeTags.join(','));
  if (query.circle) params.set('circle', query.circle);
  if (query.series) params.set('series', query.series);
  if (query.workType) params.set('work_type', query.workType);
  if (query.ageRating) params.set('age_rating', query.ageRating);
  if (query.rating !== undefined) params.set('rating', String(query.rating));
  if (query.sort) params.set('sort', query.sort);
  if (query.order) params.set('order', query.order);
  if (query.page) params.set('page', String(query.page));
  if (query.limit) params.set('limit', String(query.limit));
  if (query.hidden) params.set('hidden', '1');
  if (query.favorite) params.set('favorite', '1');
  const qs = params.toString();
  return getJson<WorksResponse>(`${API_BASE}/works${qs ? `?${qs}` : ''}`, signal);
}

export function fetchWork(id: number, signal?: AbortSignal): Promise<WorkDetail> {
  return getJson<WorkDetail>(`${API_BASE}/works/${id}`, signal);
}

export function addCustomTag(workId: number, name: string): Promise<Tag> {
  return sendJson<Tag>('POST', `${API_BASE}/works/${workId}/tags`, { name });
}

export function removeTag(workId: number, tagId: number): Promise<void> {
  return sendJson<void>('DELETE', `${API_BASE}/works/${workId}/tags/${tagId}`);
}

/** 作品の非表示状態を変更する(PATCH /api/works/{id}) */
export function setWorkHidden(workId: number, hidden: boolean): Promise<void> {
  return sendJson<void>('PATCH', `${API_BASE}/works/${workId}`, { hidden });
}

/** 作品のお気に入り状態を変更する(PATCH /api/works/{id}) */
export function setWorkFavorite(workId: number, favorite: boolean): Promise<void> {
  return sendJson<void>('PATCH', `${API_BASE}/works/${workId}`, { favorite });
}

/** 作品の評価(1〜5)を変更する。null で評価を解除する(PATCH /api/works/{id}) */
export function setWorkRating(workId: number, rating: number | null): Promise<void> {
  return sendJson<void>('PATCH', `${API_BASE}/works/${workId}`, { rating });
}

// ---- タグ ----

export function fetchTags(
  opts: { category?: string; q?: string } = {},
  signal?: AbortSignal,
): Promise<TagsResponse> {
  const params = new URLSearchParams();
  if (opts.category) params.set('category', opts.category);
  if (opts.q) params.set('q', opts.q);
  const qs = params.toString();
  return getJson<TagsResponse>(`${API_BASE}/tags${qs ? `?${qs}` : ''}`, signal);
}

// ---- サークル ----

export function fetchCircles(
  opts: { q?: string } = {},
  signal?: AbortSignal,
): Promise<CirclesResponse> {
  const params = new URLSearchParams();
  if (opts.q) params.set('q', opts.q);
  const qs = params.toString();
  return getJson<CirclesResponse>(`${API_BASE}/circles${qs ? `?${qs}` : ''}`, signal);
}

// ---- ファイルブラウズ・配信 ----

export function fetchEntries(
  workId: number,
  path: string,
  signal?: AbortSignal,
): Promise<EntriesResponse> {
  const params = new URLSearchParams();
  params.set('path', path);
  return getJson<EntriesResponse>(
    `${API_BASE}/works/${workId}/entries?${params.toString()}`,
    signal,
  );
}

/** ファイル本体 URL(audio/video の src、画像の src 等に使う)。path は encodeURIComponent。 */
export function fileUrl(workId: number, path: string): string {
  return `${API_BASE}/works/${workId}/file?path=${encodeURIComponent(path)}`;
}

export function thumbnailUrl(workId: number): string {
  return `${API_BASE}/works/${workId}/thumbnail`;
}

/**
 * テキストファイルの中身を取得する。
 *
 * DLsite 同梱テキスト(readme.txt・台本 txt 等)は Shift_JIS(CP932)率が高いため、
 * `res.text()`(常に UTF-8 前提)は使わず、バイト列から `decodeTextBuffer` で
 * UTF-8 として妥当なら UTF-8、不正バイト列なら Shift_JIS とみなしてデコードする(issue #53)。
 */
export async function fetchTextFile(
  workId: number,
  path: string,
  signal?: AbortSignal,
): Promise<string> {
  const res = await fetch(fileUrl(workId, path), { signal });
  if (!res.ok) return parseError(res);
  const buf = await res.arrayBuffer();
  return decodeTextBuffer(buf);
}

// ---- 再生履歴 ----

export function recordPlay(workId: number, path: string): Promise<void> {
  return sendJson<void>('POST', `${API_BASE}/works/${workId}/plays`, { path });
}

export function fetchHistory(params: HistoryParams = {}, signal?: AbortSignal): Promise<HistoryResponse> {
  const sp = new URLSearchParams();
  sp.set('page', String(params.page ?? 1));
  if (params.q) sp.set('q', params.q);
  if (params.sort) sp.set('sort', params.sort);
  if (params.order) sp.set('order', params.order);
  return getJson<HistoryResponse>(`${API_BASE}/history?${sp.toString()}`, signal);
}

/**
 * 再生履歴を削除する(DELETE /api/history)。
 * workId を指定するとその作品の履歴のみ、省略すると全件削除する。
 */
export function deleteHistory(workId?: number): Promise<DeleteHistoryResult> {
  const qs = workId !== undefined ? `?work_id=${workId}` : '';
  return sendJson<DeleteHistoryResult>('DELETE', `${API_BASE}/history${qs}`);
}

// ---- 管理 ----

export async function importCsv(file: File): Promise<ImportResult> {
  const form = new FormData();
  form.append('file', file);
  const res = await fetch(`${API_BASE}/import/csv`, { method: 'POST', body: form });
  if (!res.ok) return parseError(res);
  return (await res.json()) as ImportResult;
}

export function runScan(): Promise<ScanResult> {
  return sendJson<ScanResult>('POST', `${API_BASE}/scan`);
}

/**
 * エクスポート済みメタデータ JSON ファイルをインポートする(POST /api/import/metadata)。
 * CSV インポートと異なり multipart ではなく、ファイルの中身を application/json として
 * そのまま POST する(サーバ側は GET /api/export と同じ JSON 形をボディ直送で受け取る)。
 */
export async function importMetadata(file: File): Promise<ImportMetadataResult> {
  const text = await file.text();
  const res = await fetch(`${API_BASE}/import/metadata`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: text,
  });
  if (!res.ok) return parseError(res);
  return (await res.json()) as ImportMetadataResult;
}

/** どの作品にも紐づかないタグを削除する */
export function cleanupTags(): Promise<TagCleanupResult> {
  return sendJson<TagCleanupResult>('POST', `${API_BASE}/tags/cleanup`);
}

/**
 * 全作品のサムネイル再生成ジョブを開始する(非同期。issue #55)。
 * 202 Accepted とともに開始時点の進捗スナップショット(running=true, total 確定済み)
 * を返す。完了までの進捗は fetchThumbnailRebuildStatus でポーリングする。
 */
export function rebuildThumbnails(): Promise<ThumbnailRebuildStatus> {
  return sendJson<ThumbnailRebuildStatus>('POST', `${API_BASE}/thumbnails/rebuild`);
}

/** サムネイル一括再生成ジョブの進捗を取得する(ポーリング用) */
export function fetchThumbnailRebuildStatus(
  signal?: AbortSignal,
): Promise<ThumbnailRebuildStatus> {
  return getJson<ThumbnailRebuildStatus>(`${API_BASE}/thumbnails/rebuild/status`, signal);
}

/** 単一作品のサムネイルを再生成チェックする(fire-and-forget で利用) */
export function refreshThumbnail(workId: number): Promise<ThumbnailRefreshResult> {
  return sendJson<ThumbnailRefreshResult>(
    'POST',
    `${API_BASE}/works/${workId}/thumbnail/refresh`,
  );
}
