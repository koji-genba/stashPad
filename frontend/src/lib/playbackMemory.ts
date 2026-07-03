// 音声の再生位置を localStorage に記憶し「続きから再生」を実現するユーティリティ。
// キュー・再生状態(playerStore)自体は「ファイルが消えている可能性」を理由に
// 意図的に非永続化しているが、位置だけは別モジュールとして localStorage に持つ。
//
// 保存ルール:
//  - position < 30 秒 → 削除(頭付近は「続き」に値しない・巻き戻しの意図を尊重)
//  - duration > 0 かつ duration - position < 30 秒 → 削除(聴き終わった扱い)
//  - それ以外 → upsert
// LRU 上限 200 件。localStorage 例外・JSON 破損は握りつぶし機能全体を no-op にする。

const STORAGE_KEY = 'stashpad-playback-positions';
const THROTTLE_MS = 5000;
const MIN_POSITION_SEC = 30;
const END_MARGIN_SEC = 30;
const MAX_ENTRIES = 200;

interface PositionEntry {
  position: number;
  duration: number;
  updatedAt: number;
}

type PositionStore = Record<string, PositionEntry>;

// キーごとの直近保存時刻(throttle 用)。モジュール内シングルトン。
let lastSavedAt: Record<string, number> = {};

function keyFor(workId: number, path: string): string {
  return `${workId}:${path}`;
}

function readStore(): PositionStore {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as PositionStore;
    }
    return {};
  } catch {
    return {};
  }
}

function writeStore(store: PositionStore): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(store));
  } catch {
    // quota 超過・localStorage 無効化等は無視(機能全体が no-op になるだけ)
  }
}

/** LRU 上限を超えていたら updatedAt が古い順に削除する(store を直接変更) */
function enforceLru(store: PositionStore): void {
  const keys = Object.keys(store);
  if (keys.length <= MAX_ENTRIES) return;
  keys.sort((a, b) => store[a].updatedAt - store[b].updatedAt);
  const excess = keys.length - MAX_ENTRIES;
  for (let i = 0; i < excess; i++) {
    delete store[keys[i]];
  }
}

/** 保存ルールを適用して store を直接変更する(削除 or upsert) */
function applyRule(store: PositionStore, key: string, position: number, duration: number): void {
  const nearEnd = duration > 0 && duration - position < END_MARGIN_SEC;
  if (position < MIN_POSITION_SEC || nearEnd) {
    delete store[key];
    return;
  }
  store[key] = { position, duration, updatedAt: Date.now() };
  enforceLru(store);
}

/** 保存済みの再開位置を返す。エントリが無ければ null */
export function loadResumePosition(workId: number, path: string): number | null {
  try {
    const store = readStore();
    const entry = store[keyFor(workId, path)];
    return entry ? entry.position : null;
  } catch {
    return null;
  }
}

/** 進捗を保存する(5秒 throttle 付き)。onTimeUpdate から高頻度に呼ばれる想定 */
export function recordProgress(workId: number, path: string, position: number, duration: number): void {
  try {
    const key = keyFor(workId, path);
    const now = Date.now();
    const last = lastSavedAt[key] ?? -Infinity;
    if (now - last < THROTTLE_MS) return;
    lastSavedAt[key] = now;

    const store = readStore();
    applyRule(store, key, position, duration);
    writeStore(store);
  } catch {
    // no-op
  }
}

/** throttle を無視して即座に保存する(pause 時用) */
export function flushProgress(workId: number, path: string, position: number, duration: number): void {
  try {
    const key = keyFor(workId, path);
    lastSavedAt[key] = Date.now();

    const store = readStore();
    applyRule(store, key, position, duration);
    writeStore(store);
  } catch {
    // no-op
  }
}

/** エントリを削除する(トラック終了時用) */
export function clearProgress(workId: number, path: string): void {
  try {
    const key = keyFor(workId, path);
    const store = readStore();
    delete store[key];
    writeStore(store);
  } catch {
    // no-op
  }
}

/** テスト専用: throttle の内部状態をリセットする */
export function _resetForTest(): void {
  lastSavedAt = {};
}
