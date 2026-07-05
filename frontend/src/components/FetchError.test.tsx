// FetchError のテスト。
// エラーメッセージ表示 + 再試行ボタンの共通コンポーネント。
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import FetchError from './FetchError';

afterEach(() => {
  cleanup();
});

describe('FetchError', () => {
  it('message を表示する', () => {
    render(<FetchError message="読み込み失敗" onRetry={vi.fn()} />);
    expect(screen.getByText('読み込み失敗')).toBeInTheDocument();
  });

  it('再試行ボタンが表示される', () => {
    render(<FetchError message="読み込み失敗" onRetry={vi.fn()} />);
    expect(screen.getByRole('button', { name: '再試行' })).toBeInTheDocument();
  });

  it('再試行ボタンをクリックすると onRetry が呼ばれる', () => {
    const onRetry = vi.fn();
    render(<FetchError message="読み込み失敗" onRetry={onRetry} />);

    fireEvent.click(screen.getByRole('button', { name: '再試行' }));

    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('メッセージに error クラスが付く(既存のエラー表示の見た目を踏襲)', () => {
    render(<FetchError message="読み込み失敗" onRetry={vi.fn()} />);
    expect(screen.getByText('読み込み失敗')).toHaveClass('error');
  });
});
