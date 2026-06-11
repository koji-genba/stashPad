// フルスクリーンプレイヤー / キュー画面の表示状態を history(location.state)で表現するフック。
//
// 表示状態を zustand ではなく history エントリに持たせることで、Android の「戻る」
// (= popstate)が自然に「オーバーレイを 1 段閉じる」操作になる。階層は:
//   ページ → [push] フルスクリーンプレイヤー → [push] キュー画面
// キュー画面の表示/編集モードの切り替えはコンポーネントローカル状態であり、
// history には積まない(編集モード中の「戻る」はキュー画面ごと閉じてプレイヤーに戻す要件)。
//
// 注意: ブラウザの navigate(-n) は非同期なので、location が変わるまでの間に close が
// 多重発火し得る(Escape 連打、Escape とボタンの同時押し等)。同一 history エントリ
// (location.key)からの戻り/積みは 1 回だけ通すモジュール共有ガードで防ぐ。
import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

export interface PlayerOverlay {
  /** フルスクリーンプレイヤーを表示中か */
  playerOpen: boolean;
  /** キュー画面を表示中か(playerOpen が前提) */
  queueOpen: boolean;
  openPlayer: () => void;
  openQueue: () => void;
  /** プレイヤーを閉じる(キュー画面表示中は無効。先にキューを閉じる) */
  closePlayer: () => void;
  closeQueue: () => void;
  /** キューが空になる等でエントリが宙に浮いたとき、積んだ段数をまとめて巻き戻す */
  unwind: () => void;
}

/** location.state に積むフラグのキー */
const PLAYER_FLAG = 'fsPlayer';
const QUEUE_FLAG = 'fsQueue';

interface OverlayFlags {
  [PLAYER_FLAG]?: boolean;
  [QUEUE_FLAG]?: boolean;
}

// 同一 history エントリからの二重 push / 二重 back 防止(全フックインスタンス共有)。
let lastPushKey: string | null = null;
let lastBackKey: string | null = null;
// ガードをどの location.key に対して解除済みか。StrictMode の effect 二重実行や
// 複数インスタンスの effect が同一コミットで走っても、解除は「エントリ到着につき
// 1 回だけ」になるようにする(無条件で null に戻すと二重実行でガードが消える)。
let resetForKey: string | null = null;

function stateOf(location: { state: unknown }): OverlayFlags & Record<string, unknown> {
  const s = location.state;
  return typeof s === 'object' && s !== null ? (s as OverlayFlags & Record<string, unknown>) : {};
}

export function usePlayerOverlay(): PlayerOverlay {
  const location = useLocation();
  const navigate = useNavigate();

  const flags = stateOf(location);
  const playerOpen = flags[PLAYER_FLAG] === true;
  const queueOpen = playerOpen && flags[QUEUE_FLAG] === true;
  // 巻き戻すべき段数はフラグから直接数える(片方だけ立った異常エントリにも耐える)
  const depth = (flags[PLAYER_FLAG] ? 1 : 0) + (flags[QUEUE_FLAG] ? 1 : 0);
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

  const push = (nextFlags: OverlayFlags) => {
    if (lastPushKey === location.key) return;
    lastPushKey = location.key;
    navigate(here, { state: { ...stateOf(location), ...nextFlags } });
  };

  const back = (delta: number) => {
    if (lastBackKey === location.key) return;
    lastBackKey = location.key;
    navigate(delta);
  };

  return {
    playerOpen,
    queueOpen,
    openPlayer: () => {
      if (!playerOpen) push({ [PLAYER_FLAG]: true });
    },
    openQueue: () => {
      if (playerOpen && !queueOpen) push({ [PLAYER_FLAG]: true, [QUEUE_FLAG]: true });
    },
    closePlayer: () => {
      if (playerOpen && !queueOpen) back(-1);
    },
    closeQueue: () => {
      if (queueOpen) back(-1);
    },
    unwind: () => {
      if (depth > 0) back(-depth);
    },
  };
}
