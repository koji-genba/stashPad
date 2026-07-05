// API クライアントのユニットテスト。
// global fetch を vi.stubGlobal でモックし、副作用を切り離す。
// 各テストで: (a) 正しい URL・メソッド・ボディで fetch が呼ばれる
//              (b) 正常レスポンスのパース
//              (c) HTTP エラー時の挙動(ApiRequestError をスロー)
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  addCustomTag,
  ApiRequestError,
  cleanupTags,
  fetchEntries,
  fetchHistory,
  fetchTags,
  fetchTextFile,
  fetchThumbnailRebuildStatus,
  fetchWork,
  fetchWorks,
  fileUrl,
  importCsv,
  importMetadata,
  rebuildThumbnails,
  recordPlay,
  refreshThumbnail,
  removeTag,
  runScan,
  thumbnailUrl,
} from './client';

// ---- fetch モックヘルパ ----

/** 正常 JSON レスポンスを返す fetch モック */
function mockFetchOk(body: unknown, status = 200) {
  return vi.fn().mockResolvedValue({
    ok: true,
    status,
    statusText: 'OK',
    json: vi.fn().mockResolvedValue(body),
    text: vi.fn().mockResolvedValue(JSON.stringify(body)),
  });
}

/** エラーレスポンスを返す fetch モック */
function mockFetchError(status: number, errorBody: unknown = { error: 'エラー発生' }) {
  return vi.fn().mockResolvedValue({
    ok: false,
    status,
    statusText: 'Error',
    json: vi.fn().mockResolvedValue(errorBody),
    text: vi.fn().mockResolvedValue(JSON.stringify(errorBody)),
  });
}

/** ボディなしの正常レスポンス(204)を返す fetch モック */
function mockFetchNoContent() {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 204,
    statusText: 'No Content',
    json: vi.fn().mockResolvedValue(undefined),
    text: vi.fn().mockResolvedValue(''),
  });
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// ---- URL 生成関数 ----

describe('fileUrl / thumbnailUrl', () => {
  it('fileUrl は正しい URL を生成する', () => {
    expect(fileUrl(1, 'track.mp3')).toBe('/api/works/1/file?path=track.mp3');
  });

  it('fileUrl は path を encodeURIComponent でエンコードする', () => {
    expect(fileUrl(2, 'サブフォルダ/音声ファイル.mp3')).toBe(
      '/api/works/2/file?path=%E3%82%B5%E3%83%96%E3%83%95%E3%82%A9%E3%83%AB%E3%83%80%2F%E9%9F%B3%E5%A3%B0%E3%83%95%E3%82%A1%E3%82%A4%E3%83%AB.mp3',
    );
  });

  it('thumbnailUrl は正しい URL を生成する', () => {
    expect(thumbnailUrl(3)).toBe('/api/works/3/thumbnail');
  });
});

// ---- fetchWorks ----

describe('fetchWorks', () => {
  it('パラメータなしで GET /api/works を呼ぶ', async () => {
    const data = { items: [], total: 0, page: 1, limit: 20 };
    fetchMock = mockFetchOk(data);
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchWorks({});
    // signal=undefined が渡される(getJson は常に { signal } を渡す)
    expect(fetchMock).toHaveBeenCalledWith('/api/works', { signal: undefined });
    expect(result).toEqual(data);
  });

  it('クエリパラメータを正しく構築する', async () => {
    fetchMock = mockFetchOk({ items: [], total: 0, page: 2, limit: 10 });
    vi.stubGlobal('fetch', fetchMock);

    await fetchWorks({ q: 'テスト', page: 2, limit: 10, sort: 'title', order: 'asc' });

    const [url] = fetchMock.mock.calls[0] as [string, ...unknown[]];
    const params = new URL(url, 'http://localhost').searchParams;
    expect(params.get('q')).toBe('テスト');
    expect(params.get('page')).toBe('2');
    expect(params.get('limit')).toBe('10');
    expect(params.get('sort')).toBe('title');
    expect(params.get('order')).toBe('asc');
  });

  it('tags パラメータをカンマ区切りで送る', async () => {
    fetchMock = mockFetchOk({ items: [], total: 0, page: 1, limit: 20 });
    vi.stubGlobal('fetch', fetchMock);

    await fetchWorks({ tags: [1, 2, 3] });

    const [url] = fetchMock.mock.calls[0] as [string, ...unknown[]];
    const params = new URL(url, 'http://localhost').searchParams;
    expect(params.get('tags')).toBe('1,2,3');
  });

  it('circle / series パラメータを送る', async () => {
    fetchMock = mockFetchOk({ items: [], total: 0, page: 1, limit: 20 });
    vi.stubGlobal('fetch', fetchMock);

    await fetchWorks({ circle: 'サークルA', series: 'シリーズB' });

    const [url] = fetchMock.mock.calls[0] as [string, ...unknown[]];
    const params = new URL(url, 'http://localhost').searchParams;
    expect(params.get('circle')).toBe('サークルA');
    expect(params.get('series')).toBe('シリーズB');
  });

  it('日本語クエリの URL エンコードが正しく行われる', async () => {
    fetchMock = mockFetchOk({ items: [], total: 0, page: 1, limit: 20 });
    vi.stubGlobal('fetch', fetchMock);

    await fetchWorks({ q: '日本語テスト' });

    const [url] = fetchMock.mock.calls[0] as [string, ...unknown[]];
    // URLSearchParams が自動的にエンコードするため、q パラメータを含む URL になる
    expect(url).toContain('q=');
    const params = new URL(url, 'http://localhost').searchParams;
    expect(params.get('q')).toBe('日本語テスト');
  });

  it('HTTP エラー時に ApiRequestError をスローする', async () => {
    fetchMock = mockFetchError(404, { error: '見つかりません' });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchWorks({})).rejects.toThrow(ApiRequestError);
    await expect(fetchWorks({})).rejects.toMatchObject({ status: 404, message: '見つかりません' });
  });
});

// ---- fetchWork ----

describe('fetchWork', () => {
  it('GET /api/works/:id を呼ぶ', async () => {
    const data = { id: 1, title: 'テスト作品', rj_number: 'RJ123456' };
    fetchMock = mockFetchOk(data);
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchWork(1);
    expect(fetchMock).toHaveBeenCalledWith('/api/works/1', { signal: undefined });
    expect(result).toEqual(data);
  });

  it('HTTP エラー時に ApiRequestError をスローする', async () => {
    fetchMock = mockFetchError(500, { error: 'サーバーエラー' });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchWork(1)).rejects.toThrow(ApiRequestError);
    await expect(fetchWork(1)).rejects.toMatchObject({ status: 500, message: 'サーバーエラー' });
  });
});

// ---- addCustomTag ----

describe('addCustomTag', () => {
  it('POST /api/works/:id/tags を正しい JSON ボディで呼ぶ', async () => {
    const tag = { id: 10, name: 'カスタムタグ', category: 'custom' };
    fetchMock = mockFetchOk(tag);
    vi.stubGlobal('fetch', fetchMock);

    const result = await addCustomTag(1, 'カスタムタグ');
    expect(fetchMock).toHaveBeenCalledWith('/api/works/1/tags', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'カスタムタグ' }),
    });
    expect(result).toEqual(tag);
  });

  it('HTTP エラー時に ApiRequestError をスローする', async () => {
    fetchMock = mockFetchError(400, { error: '不正なリクエスト' });
    vi.stubGlobal('fetch', fetchMock);

    await expect(addCustomTag(1, 'タグ')).rejects.toThrow(ApiRequestError);
  });
});

// ---- removeTag ----

describe('removeTag', () => {
  it('DELETE /api/works/:id/tags/:tagId を呼ぶ', async () => {
    fetchMock = mockFetchNoContent();
    vi.stubGlobal('fetch', fetchMock);

    await removeTag(1, 5);
    expect(fetchMock).toHaveBeenCalledWith('/api/works/1/tags/5', {
      method: 'DELETE',
      headers: undefined,
      body: undefined,
    });
  });

  it('HTTP エラー時に ApiRequestError をスローする', async () => {
    fetchMock = mockFetchError(404, { error: 'タグが見つかりません' });
    vi.stubGlobal('fetch', fetchMock);

    await expect(removeTag(1, 5)).rejects.toThrow(ApiRequestError);
  });
});

// ---- fetchTags ----

describe('fetchTags', () => {
  it('パラメータなしで GET /api/tags を呼ぶ', async () => {
    fetchMock = mockFetchOk({ items: [] });
    vi.stubGlobal('fetch', fetchMock);

    await fetchTags();
    expect(fetchMock).toHaveBeenCalledWith('/api/tags', { signal: undefined });
  });

  it('category と q パラメータを送る', async () => {
    fetchMock = mockFetchOk({ items: [] });
    vi.stubGlobal('fetch', fetchMock);

    await fetchTags({ category: 'genre', q: '音楽' });

    const [url] = fetchMock.mock.calls[0] as [string, ...unknown[]];
    const params = new URL(url, 'http://localhost').searchParams;
    expect(params.get('category')).toBe('genre');
    expect(params.get('q')).toBe('音楽');
  });
});

// ---- fetchEntries ----

describe('fetchEntries', () => {
  it('GET /api/works/:id/entries?path=... を呼ぶ', async () => {
    const data = { path: 'dir', parent: '', entries: [] };
    fetchMock = mockFetchOk(data);
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchEntries(1, 'dir');
    const [url] = fetchMock.mock.calls[0] as [string, ...unknown[]];
    expect(url).toContain('/api/works/1/entries');
    const params = new URL(url, 'http://localhost').searchParams;
    expect(params.get('path')).toBe('dir');
    expect(result).toEqual(data);
  });

  it('日本語パスを正しくエンコードする', async () => {
    fetchMock = mockFetchOk({ path: '日本語フォルダ', parent: '', entries: [] });
    vi.stubGlobal('fetch', fetchMock);

    await fetchEntries(1, '日本語フォルダ');

    const [url] = fetchMock.mock.calls[0] as [string, ...unknown[]];
    const params = new URL(url, 'http://localhost').searchParams;
    expect(params.get('path')).toBe('日本語フォルダ');
  });

  it('HTTP エラー時に ApiRequestError をスローする', async () => {
    fetchMock = mockFetchError(403, { error: 'アクセス禁止' });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchEntries(1, 'secret')).rejects.toThrow(ApiRequestError);
  });
});

// ---- fetchTextFile ----

describe('fetchTextFile', () => {
  it('ファイル URL を GET してテキストを返す(UTF-8)', async () => {
    const buf = new TextEncoder().encode('テキストの内容').buffer;
    const fetchMockWithBuffer = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: vi.fn().mockResolvedValue({}),
      arrayBuffer: vi.fn().mockResolvedValue(buf),
    });
    vi.stubGlobal('fetch', fetchMockWithBuffer);

    const result = await fetchTextFile(1, 'readme.txt');
    const [url] = fetchMockWithBuffer.mock.calls[0] as [string, ...unknown[]];
    expect(url).toBe('/api/works/1/file?path=readme.txt');
    expect(result).toBe('テキストの内容');
  });

  it('Shift_JIS のバイト列を正しくデコードする(issue #53)', async () => {
    // "あ" の Shift_JIS バイト列
    const buf = new Uint8Array([0x82, 0xa0]).buffer;
    const fetchMockWithBuffer = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: vi.fn().mockResolvedValue({}),
      arrayBuffer: vi.fn().mockResolvedValue(buf),
    });
    vi.stubGlobal('fetch', fetchMockWithBuffer);

    const result = await fetchTextFile(1, 'sjis.txt');
    expect(result).toBe('あ');
  });

  it('HTTP エラー時に ApiRequestError をスローする', async () => {
    fetchMock = mockFetchError(404, { error: 'ファイルが見つかりません' });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchTextFile(1, 'missing.txt')).rejects.toThrow(ApiRequestError);
  });
});

// ---- recordPlay ----

describe('recordPlay', () => {
  it('POST /api/works/:id/plays を正しいボディで呼ぶ', async () => {
    fetchMock = mockFetchNoContent();
    vi.stubGlobal('fetch', fetchMock);

    await recordPlay(1, 'track.mp3');
    expect(fetchMock).toHaveBeenCalledWith('/api/works/1/plays', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: 'track.mp3' }),
    });
  });
});

// ---- fetchHistory ----

describe('fetchHistory', () => {
  it('GET /api/history?page=1 を呼ぶ(デフォルト)', async () => {
    fetchMock = mockFetchOk({ items: [], page: 1 });
    vi.stubGlobal('fetch', fetchMock);

    await fetchHistory();
    expect(fetchMock).toHaveBeenCalledWith('/api/history?page=1', { signal: undefined });
  });

  it('ページ番号を指定できる', async () => {
    fetchMock = mockFetchOk({ items: [], page: 3 });
    vi.stubGlobal('fetch', fetchMock);

    await fetchHistory({ page: 3 });
    expect(fetchMock).toHaveBeenCalledWith('/api/history?page=3', { signal: undefined });
  });

  it('q・sort・order を指定できる', async () => {
    fetchMock = mockFetchOk({ items: [], page: 1 });
    vi.stubGlobal('fetch', fetchMock);

    await fetchHistory({ page: 1, q: '猫', sort: 'play_count', order: 'asc' });
    const expected = new URLSearchParams();
    expected.set('page', '1');
    expected.set('q', '猫');
    expected.set('sort', 'play_count');
    expected.set('order', 'asc');
    expect(fetchMock).toHaveBeenCalledWith(
      `/api/history?${expected.toString()}`,
      { signal: undefined },
    );
  });
});

// ---- importCsv ----

describe('importCsv', () => {
  it('POST /api/import/csv を multipart/form-data で呼ぶ', async () => {
    const result = { created: 0, updated: 1, linked: 1, skipped: 2, errors: [] };
    fetchMock = mockFetchOk(result);
    vi.stubGlobal('fetch', fetchMock);

    const file = new File(['csv data'], 'works.csv', { type: 'text/csv' });
    const returned = await importCsv(file);

    const [url, options] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/import/csv');
    expect(options.method).toBe('POST');
    expect(options.body).toBeInstanceOf(FormData);
    expect(returned).toEqual(result);
  });

  it('HTTP エラー時に ApiRequestError をスローする', async () => {
    fetchMock = mockFetchError(422, { error: 'CSV が不正' });
    vi.stubGlobal('fetch', fetchMock);

    const file = new File([''], 'bad.csv');
    await expect(importCsv(file)).rejects.toThrow(ApiRequestError);
  });
});

// ---- importMetadata ----
// CSV インポートと異なり multipart ではなく、ファイルの中身(JSON テキスト)を
// application/json としてそのまま POST する(issue #78)。

describe('importMetadata', () => {
  it('POST /api/import/metadata を application/json ボディ直送で呼ぶ', async () => {
    const result = { matched: 2, skipped: 1, tags_added: 3, errors: [] };
    fetchMock = mockFetchOk(result);
    vi.stubGlobal('fetch', fetchMock);

    const jsonText = '{"version":1,"works":[]}';
    const file = new File([jsonText], 'stashpad-metadata-20260705.json', {
      type: 'application/json',
    });
    const returned = await importMetadata(file);

    const [url, options] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/import/metadata');
    expect(options.method).toBe('POST');
    expect(options.headers).toEqual({ 'Content-Type': 'application/json' });
    expect(options.body).toBe(jsonText);
    expect(returned).toEqual(result);
  });

  it('HTTP エラー時に ApiRequestError をスローする', async () => {
    fetchMock = mockFetchError(400, { error: 'version が不正です' });
    vi.stubGlobal('fetch', fetchMock);

    const file = new File(['{"version":2,"works":[]}'], 'bad.json');
    await expect(importMetadata(file)).rejects.toThrow(ApiRequestError);
  });
});

// ---- runScan ----

describe('runScan', () => {
  it('POST /api/scan を呼ぶ', async () => {
    const result = { works_found: 5, newly_registered: 2, linked_to_csv: 3, missing_marked: 0 };
    fetchMock = mockFetchOk(result);
    vi.stubGlobal('fetch', fetchMock);

    const returned = await runScan();
    expect(fetchMock).toHaveBeenCalledWith('/api/scan', {
      method: 'POST',
      headers: undefined,
      body: undefined,
    });
    expect(returned).toEqual(result);
  });
});

// ---- cleanupTags ----

describe('cleanupTags', () => {
  it('POST /api/tags/cleanup を呼ぶ', async () => {
    fetchMock = mockFetchOk({ deleted: 3 });
    vi.stubGlobal('fetch', fetchMock);

    const result = await cleanupTags();
    expect(fetchMock).toHaveBeenCalledWith('/api/tags/cleanup', {
      method: 'POST',
      headers: undefined,
      body: undefined,
    });
    expect(result).toEqual({ deleted: 3 });
  });
});

// ---- rebuildThumbnails ----

describe('rebuildThumbnails', () => {
  it('POST /api/thumbnails/rebuild を呼び、202 の進捗スナップショットを返す', async () => {
    fetchMock = mockFetchOk({ running: true, checked: 0, regenerated: 0, total: 10 }, 202);
    vi.stubGlobal('fetch', fetchMock);

    const result = await rebuildThumbnails();
    expect(fetchMock).toHaveBeenCalledWith('/api/thumbnails/rebuild', {
      method: 'POST',
      headers: undefined,
      body: undefined,
    });
    expect(result).toEqual({ running: true, checked: 0, regenerated: 0, total: 10 });
  });
});

// ---- fetchThumbnailRebuildStatus ----

describe('fetchThumbnailRebuildStatus', () => {
  it('GET /api/thumbnails/rebuild/status を呼ぶ', async () => {
    fetchMock = mockFetchOk({ running: true, checked: 3, regenerated: 1, total: 10 });
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchThumbnailRebuildStatus();
    expect(fetchMock).toHaveBeenCalledWith('/api/thumbnails/rebuild/status', { signal: undefined });
    expect(result).toEqual({ running: true, checked: 3, regenerated: 1, total: 10 });
  });
});

// ---- refreshThumbnail ----

describe('refreshThumbnail', () => {
  it('POST /api/works/:id/thumbnail/refresh を呼ぶ', async () => {
    fetchMock = mockFetchOk({ refreshed: true });
    vi.stubGlobal('fetch', fetchMock);

    const result = await refreshThumbnail(42);
    expect(fetchMock).toHaveBeenCalledWith('/api/works/42/thumbnail/refresh', {
      method: 'POST',
      headers: undefined,
      body: undefined,
    });
    expect(result).toEqual({ refreshed: true });
  });
});

// ---- ApiRequestError ----

describe('ApiRequestError', () => {
  it('status と message を持つ', () => {
    const err = new ApiRequestError(404, 'Not Found');
    expect(err.status).toBe(404);
    expect(err.message).toBe('Not Found');
    expect(err.name).toBe('ApiRequestError');
    expect(err).toBeInstanceOf(Error);
  });

  it('エラーボディが JSON でない場合はステータスコードのメッセージになる', async () => {
    fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      statusText: 'Service Unavailable',
      json: vi.fn().mockRejectedValue(new SyntaxError('JSON parse error')),
    });
    vi.stubGlobal('fetch', fetchMock);

    try {
      await fetchWork(1);
      expect.fail('エラーがスローされるべき');
    } catch (e) {
      expect(e).toBeInstanceOf(ApiRequestError);
      expect((e as ApiRequestError).status).toBe(503);
      expect((e as ApiRequestError).message).toBe('503 Service Unavailable');
    }
  });
});
