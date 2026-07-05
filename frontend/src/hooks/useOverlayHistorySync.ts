// 画像 / 動画 / テキストのオーバーレイ(overlayStore)を history と同期するフック。
// <Overlays> に 1 つだけマウントする。
//
// フルスクリーンプレイヤー(usePlayerOverlay)と同じく「表示中 = history エントリ 1 段」
// で表現することで、Android の「戻る」がページ遷移ではなくオーバーレイを閉じる操作に
// なる(issue #52)。ただしオーバーレイの内容(ページ列・再生対象)は複雑なので
// location.state には持たせず zustand(overlayStore)を正とし、このフックは
// 開閉と history エントリの対応だけを取る:
//
//   store が開いた(フラグ無し)     → フラグ付きエントリを push
//   戻る/進む/ページ遷移でフラグが消えた → store を閉じる
//   ✕ / Escape 等で store が閉じた  → 残ったフラグエントリを 1 段巻き戻す
//   リロード直後にフラグだけ残った   → 同上(巻き戻して掃除)
//
// 既存の close 系(closeImage / closeVideo / closeText)は store を直接閉じるだけで
// よく、履歴の巻き戻しはこのフックが引き受ける。多重発火は useGuardedHistoryNav の
// ガードで防ぐ。
import { useEffect, useRef } from 'react';
import { useLocation } from 'react-router-dom';
import { useOverlayStore } from '@/store/overlayStore';
import { stateOf, useGuardedHistoryNav } from './useGuardedHistoryNav';

/** location.state に積むフラグのキー */
export const MEDIA_OVERLAY_FLAG = 'mediaOverlay';

export function useOverlayHistorySync(): void {
  const location = useLocation();
  const nav = useGuardedHistoryNav();
  const anyOpen = useOverlayStore(
    (s) => s.image !== null || s.video !== null || s.text !== null,
  );
  const closeAll = useOverlayStore((s) => s.closeAll);

  const flagOn = stateOf(location)[MEDIA_OVERLAY_FLAG] === true;
  // 直前 effect 実行時のフラグ状態。「store が開いた直後(まだ push 前)」と
  // 「戻るでフラグが消えた直後」はどちらも anyOpen && !flagOn の形になるため、
  // フラグの true → false 遷移かどうかで区別する。
  const prevFlagOn = useRef(false);

  useEffect(() => {
    const wasFlagOn = prevFlagOn.current;
    prevFlagOn.current = flagOn;

    if (anyOpen && !flagOn) {
      if (wasFlagOn) {
        // 戻る / 進む / ページ遷移でフラグエントリを離れた → オーバーレイを閉じる
        closeAll();
      } else {
        // store で開かれた → 「戻る」で閉じられるようフラグ付きエントリを push
        nav.push({ [MEDIA_OVERLAY_FLAG]: true });
      }
      return;
    }
    if (!anyOpen && flagOn) {
      // ✕ / Escape で store が先に閉じた、またはリロードでフラグだけ残った
      // → フラグエントリを巻き戻す
      nav.back(-1);
    }
    // nav / closeAll は location / store に対して安定なので依存に含めない
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [anyOpen, flagOn, location.key]);
}
