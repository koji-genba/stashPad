// useBodyScrollLock フックのテスト。
// 複数のオーバーレイが同時に active になれば参照カウントで "hidden" を維持し、
// 全部が unmount されて初めて元の overflow 値に戻ることを検証する。
import { StrictMode } from 'react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { render, cleanup } from '@testing-library/react';
import { useBodyScrollLock, __resetForTests } from './useBodyScrollLock';

// テスト用コンポーネント: active フラグを受け取るだけのプローブ
function LockProbe({ active }: { active: boolean }) {
  useBodyScrollLock(active);
  return null;
}

describe('useBodyScrollLock', () => {
  beforeEach(() => {
    __resetForTests();
    document.body.style.overflow = '';
  });

  afterEach(() => {
    cleanup();
    document.body.style.overflow = '';
  });

  it('active=true でマウントすると overflow が "hidden" になる', () => {
    render(<LockProbe active={true} />);
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('unmount すると元の値("") に戻る', () => {
    const { unmount } = render(<LockProbe active={true} />);
    expect(document.body.style.overflow).toBe('hidden');
    unmount();
    expect(document.body.style.overflow).toBe('');
  });

  it('事前に overflow="auto" がセットされていれば unmount で "auto" に戻る', () => {
    document.body.style.overflow = 'auto';
    const { unmount } = render(<LockProbe active={true} />);
    expect(document.body.style.overflow).toBe('hidden');
    unmount();
    expect(document.body.style.overflow).toBe('auto');
  });

  it('active=false では overflow を変えない', () => {
    render(<LockProbe active={false} />);
    expect(document.body.style.overflow).toBe('');
  });

  it('active=false を unmount しても overflow はそのまま', () => {
    document.body.style.overflow = 'auto';
    const { unmount } = render(<LockProbe active={false} />);
    unmount();
    expect(document.body.style.overflow).toBe('auto');
  });

  it('参照カウント: 2 つが active=true のとき片方を unmount しても "hidden" を維持する', () => {
    const { unmount: unmount1 } = render(<LockProbe active={true} />);
    render(<LockProbe active={true} />);
    expect(document.body.style.overflow).toBe('hidden');
    unmount1();
    // まだもう 1 つが active なので hidden のまま
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('参照カウント: 両方 unmount されて初めて元の値に戻る', () => {
    const { unmount: unmount1 } = render(<LockProbe active={true} />);
    const { unmount: unmount2 } = render(<LockProbe active={true} />);
    unmount1();
    expect(document.body.style.overflow).toBe('hidden');
    unmount2();
    expect(document.body.style.overflow).toBe('');
  });

  it('active が true → false と切り替わると overflow が戻る', () => {
    const { rerender } = render(<LockProbe active={true} />);
    expect(document.body.style.overflow).toBe('hidden');
    rerender(<LockProbe active={false} />);
    expect(document.body.style.overflow).toBe('');
  });

  it('active が false → true → false と切り替わっても正しく動く', () => {
    const { rerender } = render(<LockProbe active={false} />);
    expect(document.body.style.overflow).toBe('');
    rerender(<LockProbe active={true} />);
    expect(document.body.style.overflow).toBe('hidden');
    rerender(<LockProbe active={false} />);
    expect(document.body.style.overflow).toBe('');
  });

  it('active=true → false のとき、別コンポーネントが lock 中なら "hidden" を維持する', () => {
    // 別コンポーネントが lock 中
    render(<LockProbe active={true} />);
    const { rerender } = render(<LockProbe active={true} />);
    // 片方を false に切り替え
    rerender(<LockProbe active={false} />);
    // 別コンポーネントが lock 中なので hidden を維持
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('StrictMode で effect が二重実行されても参照カウントが狂わない', () => {
    const { unmount } = render(
      <StrictMode>
        <LockProbe active={true} />
      </StrictMode>,
    );
    // StrictMode では effect が mount/unmount/remount される。
    // 最終的に 1 つのインスタンスが active なので hidden であるべき
    expect(document.body.style.overflow).toBe('hidden');
    unmount();
    expect(document.body.style.overflow).toBe('');
  });
});
