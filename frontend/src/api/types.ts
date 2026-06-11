// implementation-notes.md §4 の JSON 契約に厳密に対応した型定義。

export type MediaKind = 'audio' | 'video' | 'image' | 'text' | 'other' | '';

export type TagCategory =
  | 'genre'
  | 'detail_genre'
  | 'voice_actor'
  | 'scenario'
  | 'illustration'
  | 'music'
  | 'custom'
  | string;

/** GET /api/works の items 要素 */
export interface WorkListItem {
  id: number;
  rj_number: string | null;
  title: string;
  circle: string | null;
  age_rating: string | null;
  has_folder: boolean;
  thumbnail_url: string;
}

/** GET /api/works のレスポンス */
export interface WorksResponse {
  items: WorkListItem[];
  total: number;
  page: number;
  limit: number;
}

export interface Tag {
  id: number;
  name: string;
  category: TagCategory;
}

/** GET /api/works/{id} のレスポンス */
export interface WorkDetail {
  id: number;
  rj_number: string | null;
  title: string;
  circle: string | null;
  series_name: string | null;
  purchase_date: string | null;
  work_type: string | null;
  age_rating: string | null;
  file_format: string | null;
  file_size_text: string | null;
  has_folder: boolean;
  tags: Tag[];
}

/** GET /api/works/{id}/entries の entries 要素 */
export interface Entry {
  name: string;
  is_dir: boolean;
  size: number;
  media_kind: MediaKind;
}

/** GET /api/works/{id}/entries のレスポンス */
export interface EntriesResponse {
  path: string;
  parent: string;
  entries: Entry[];
}

/** GET /api/tags の items 要素 */
export interface TagFacet {
  id: number;
  name: string;
  category: TagCategory;
  work_count: number;
}

export interface TagsResponse {
  items: TagFacet[];
}

/** GET /api/history の items 要素 */
export interface HistoryItem {
  work: {
    id: number;
    title: string;
    thumbnail_url: string;
  };
  last_played_at: string;
  last_file_path: string;
  play_count: number;
}

export interface HistoryResponse {
  items: HistoryItem[];
  page: number;
  limit: number;
}

/** POST /api/import/csv のレスポンス */
export interface ImportResult {
  created: number;
  updated: number;
  linked: number;
  errors: string[];
}

/** POST /api/scan のレスポンス */
export interface ScanResult {
  works_found: number;
  newly_registered: number;
  linked_to_csv: number;
  missing_marked: number;
}

/** POST /api/tags/cleanup のレスポンス */
export interface TagCleanupResult {
  deleted: number;
}

/** POST /api/thumbnails/rebuild のレスポンス */
export interface ThumbnailRebuildResult {
  checked: number;
  regenerated: number;
}

/** POST /api/works/{id}/thumbnail/refresh のレスポンス */
export interface ThumbnailRefreshResult {
  refreshed: boolean;
}

/** エラーレスポンス */
export interface ApiError {
  error: string;
}

export type SortKey = 'purchase_date' | 'title' | 'created_at' | 'circle';
export type SortOrder = 'asc' | 'desc';
