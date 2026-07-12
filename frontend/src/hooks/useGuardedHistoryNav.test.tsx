// useGuardedHistoryNav のテスト。
// push / back は同一 history エントリ(location.key)からの多重発火を防ぐため、
// lastPushKey / lastBackKey / resetForKey をモジュール共有(全フックインスタンス)で
// 保持している。実際の navigate() は push 後に必ず新しい key の history エントリへ
// 進むため通常は自己解消するが、テストで useNavigate をモックして location.key を
// 固定すると、モジュール状態がテスト間で残存しうる(PR #79 レビュー)。
// useBodyScrollLock の __resetForTests と同じ流儀でリセット関数を用意し、
// beforeEach から呼べるようにする。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { useGuardedHistoryNav, __resetForTests } from './useGuardedHistoryNav';

// 実際の navigate は push 後に別の key へ進むため location.key が変化するが、
// ここではモックして location.key を 'fixed-key' に固定し、モジュール共有ガードの
// 残存を検証しやすくする。
const navigateSpy = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return { ...actual, useNavigate: () => navigateSpy };
});

function Probe() {
  const nav = useGuardedHistoryNav();
  return <button onClick={() => nav.push({ flag: true })}>push</button>;
}

function renderAtFixedKey() {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/x', key: 'fixed-key' }]} initialIndex={0}>
      <Probe />
    </MemoryRouter>,
  );
}

describe('useGuardedHistoryNav の __resetForTests', () => {
  // このテストファイル自体もモジュール共有ガードの影響を受けるため、
  // 各テストの開始時にリセットしてテスト順への依存をなくす
  beforeEach(() => __resetForTests());

  afterEach(() => {
    cleanup();
    navigateSpy.mockClear();
  });

  it('同一 key でガードが残っていると再マウント後も push がブロックされ、__resetForTests で解除できる', () => {
    const { unmount } = renderAtFixedKey();
    fireEvent.click(screen.getByText('push'));
    expect(navigateSpy).toHaveBeenCalledTimes(1);
    unmount();

    // 同じ location.key で再マウント。navigate をモックしているため実際には
    // key が変わらず、mount 時の effect は resetForKey === location.key で
    // ガードを解除しない(= 残存する)
    renderAtFixedKey();
    fireEvent.click(screen.getByText('push'));
    expect(navigateSpy).toHaveBeenCalledTimes(1); // ガードが残っているため増えない

    __resetForTests();
    fireEvent.click(screen.getByText('push'));
    expect(navigateSpy).toHaveBeenCalledTimes(2); // リセット後は push が再度発火する
  });

  it('同一マウント内での連打は __resetForTests なしでも 1 回だけ通す(既存の多重発火防止)', () => {
    renderAtFixedKey();
    fireEvent.click(screen.getByText('push'));
    fireEvent.click(screen.getByText('push'));
    expect(navigateSpy).toHaveBeenCalledTimes(1);
  });
});
