// body スクロールロックを参照カウントで管理するフック。
// 複数のオーバーレイが同時に active になると lock が積み重なるため、
// 最後の 1 つが閉じるまで "hidden" を維持し、全部閉じたとき初めて元の値に戻す。
import { useEffect } from 'react';

let lockCount = 0;
let savedOverflow = '';

export function useBodyScrollLock(active: boolean): void {
  useEffect(() => {
    if (!active) return;
    if (lockCount === 0) {
      savedOverflow = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
    }
    lockCount++;
    return () => {
      lockCount--;
      if (lockCount === 0) document.body.style.overflow = savedOverflow;
    };
  }, [active]);
}

export function __resetForTests(): void {
  lockCount = 0;
  savedOverflow = '';
}
