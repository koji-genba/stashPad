// history(location.state)駆動オーバーレイの共通基盤。
// usePlayerOverlay(フルスクリーンプレイヤー/キュー画面)と
// useOverlayHistorySync(画像/動画/テキスト)が共有する。
//
// ブラウザの navigate は非同期なので、location が変わるまでの間に push / back が
// 多重発火し得る(Escape 連打、複数フックインスタンスの effect 同時実行等)。
// 同一 history エントリ(location.key)からの push / back を 1 回だけ通す
// モジュール共有ガードを提供する。
import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

// 同一 history エントリからの二重 push / 二重 back 防止(全フックインスタンス共有)。
let lastPushKey: string | null = null;
let lastBackKey: string | null = null;
// ガードをどの location.key に対して解除済みか。StrictMode の effect 二重実行や
// 複数インスタンスの effect が同一コミットで走っても、解除は「エントリ到着につき
// 1 回だけ」になるようにする(無条件で null に戻すと二重実行でガードが消える)。
let resetForKey: string | null = null;

/** location.state をオブジェクトとして安全に取り出す */
export function stateOf(location: { state: unknown }): Record<string, unknown> {
  const s = location.state;
  return typeof s === 'object' && s !== null ? (s as Record<string, unknown>) : {};
}

export interface GuardedHistoryNav {
  /** 現在のパスに state を上書きマージした新しい history エントリを積む */
  push: (nextFlags: Record<string, unknown>) => void;
  /** history を delta 段戻す(delta は負数) */
  back: (delta: number) => void;
}

export function useGuardedHistoryNav(): GuardedHistoryNav {
  const location = useLocation();
  const navigate = useNavigate();
  const here = location.pathname + location.search + location.hash;

  // 新しいエントリに到着したらガードを解除する。進む/戻るで同じ key に再訪した場合も
  // 解除する必要があるため、key の変化を resetForKey との比較で検出する
  useEffect(() => {
    if (resetForKey !== location.key) {
      resetForKey = location.key;
      lastPushKey = null;
      lastBackKey = null;
    }
  });

  return {
    push: (nextFlags) => {
      if (lastPushKey === location.key) return;
      lastPushKey = location.key;
      navigate(here, { state: { ...stateOf(location), ...nextFlags } });
    },
    back: (delta) => {
      if (lastBackKey === location.key) return;
      lastBackKey = location.key;
      navigate(delta);
    },
  };
}

/**
 * テスト専用: モジュール共有ガード(lastPushKey/lastBackKey/resetForKey)をリセットする。
 * useBodyScrollLock の __resetForTests と同じ流儀。ガードはモジュールシングルトンなので、
 * 同一テストファイル内の各 it 間で状態が漏れないよう beforeEach から呼ぶ。
 */
export function __resetForTests(): void {
  lastPushKey = null;
  lastBackKey = null;
  resetForKey = null;
}
