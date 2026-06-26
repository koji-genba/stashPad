// API クライアント。全レスポンス型は ./types に定義。
// エラーは一律 {"error": "..."} なので ApiRequestError に正規化する。
import type {
  CirclesResponse,
  EntriesResponse,
  HistoryResponse,
  ImportResult,
  ScanResult,
  SortKey,
  SortOrder,
  Tag,
  TagCleanupResult,
  TagsResponse,
  ThumbnailRebuildResult,
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
  /** true のとき非表示作品のみを返す */
  hidden?: boolean;
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
  if (query.sort) params.set('sort', query.sort);
  if (query.order) params.set('order', query.order);
  if (query.page) params.set('page', String(query.page));
  if (query.limit) params.set('limit', String(query.limit));
  if (query.hidden) params.set('hidden', '1');
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

/** テキストファイルの中身を取得 */
export async function fetchTextFile(
  workId: number,
  path: string,
  signal?: AbortSignal,
): Promise<string> {
  const res = await fetch(fileUrl(workId, path), { signal });
  if (!res.ok) return parseError(res);
  return res.text();
}

// ---- 再生履歴 ----

export function recordPlay(workId: number, path: string): Promise<void> {
  return sendJson<void>('POST', `${API_BASE}/works/${workId}/plays`, { path });
}

export function fetchHistory(page = 1, signal?: AbortSignal): Promise<HistoryResponse> {
  return getJson<HistoryResponse>(`${API_BASE}/history?page=${page}`, signal);
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

/** どの作品にも紐づかないタグを削除する */
export function cleanupTags(): Promise<TagCleanupResult> {
  return sendJson<TagCleanupResult>('POST', `${API_BASE}/tags/cleanup`);
}

/** 全作品のサムネイルを再生成チェックする(時間がかかる場合がある) */
export function rebuildThumbnails(): Promise<ThumbnailRebuildResult> {
  return sendJson<ThumbnailRebuildResult>('POST', `${API_BASE}/thumbnails/rebuild`);
}

/** 単一作品のサムネイルを再生成チェックする(fire-and-forget で利用) */
export function refreshThumbnail(workId: number): Promise<ThumbnailRefreshResult> {
  return sendJson<ThumbnailRefreshResult>(
    'POST',
    `${API_BASE}/works/${workId}/thumbnail/refresh`,
  );
}
