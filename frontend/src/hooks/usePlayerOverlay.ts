// フルスクリーンプレイヤー / キュー画面の表示状態を history(location.state)で表現するフック。
//
// 表示状態を zustand ではなく history エントリに持たせることで、Android の「戻る」
// (= popstate)が自然に「オーバーレイを 1 段閉じる」操作になる。階層は:
//   ページ → [push] フルスクリーンプレイヤー → [push] キュー画面
// キュー画面の表示/編集モードの切り替えはコンポーネントローカル状態であり、
// history には積まない(編集モード中の「戻る」はキュー画面ごと閉じてプレイヤーに戻す要件)。
//
// 多重発火(Escape 連打等)の防止は useGuardedHistoryNav のモジュール共有ガードに委譲する。
import { useLocation } from 'react-router-dom';
import { stateOf, useGuardedHistoryNav } from './useGuardedHistoryNav';

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

export function usePlayerOverlay(): PlayerOverlay {
  const location = useLocation();
  const nav = useGuardedHistoryNav();

  const flags = stateOf(location);
  const playerOpen = flags[PLAYER_FLAG] === true;
  const queueOpen = playerOpen && flags[QUEUE_FLAG] === true;
  // 巻き戻すべき段数はフラグから直接数える(片方だけ立った異常エントリにも耐える)
  const depth = (flags[PLAYER_FLAG] ? 1 : 0) + (flags[QUEUE_FLAG] ? 1 : 0);

  return {
    playerOpen,
    queueOpen,
    openPlayer: () => {
      if (!playerOpen) nav.push({ [PLAYER_FLAG]: true });
    },
    openQueue: () => {
      if (playerOpen && !queueOpen) nav.push({ [PLAYER_FLAG]: true, [QUEUE_FLAG]: true });
    },
    closePlayer: () => {
      if (playerOpen && !queueOpen) nav.back(-1);
    },
    closeQueue: () => {
      if (queueOpen) nav.back(-1);
    },
    unwind: () => {
      if (depth > 0) nav.back(-depth);
    },
  };
}
