// CircleFacetPanel のテスト。
// issue #27: パネル初回表示時に q なしで全件を 1 回だけ fetch し、
// 以降の入力による絞り込みはクライアントサイド(name.toLowerCase().includes)で行う。
// デバウンス付きの再フェッチは発生しないこと(fetchCircles の呼び出し回数が 1 のまま)を確認する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';

// API クライアントをモック化して実際の HTTP リクエストを発生させない。
vi.mock('@/api/client', () => ({
  fetchCircles: vi.fn(),
}));

import { fetchCircles } from '@/api/client';
import type { CircleFacet } from '@/api/types';
import CircleFacetPanel from './CircleFacetPanel';

const fetchCirclesMock = vi.mocked(fetchCircles);

const SAMPLE_CIRCLES: CircleFacet[] = [
  { name: 'Alpha Circle', work_count: 3 },
  { name: 'beta works', work_count: 5 },
  { name: 'ガンマ工房', work_count: 2 },
];

function renderPanel(selected = '') {
  const onSelect = vi.fn();
  const utils = render(<CircleFacetPanel selected={selected} onSelect={onSelect} />);
  return { onSelect, ...utils };
}

beforeEach(() => {
  fetchCirclesMock.mockReset();
  fetchCirclesMock.mockResolvedValue({ items: SAMPLE_CIRCLES });
});

afterEach(() => {
  cleanup();
});

describe('CircleFacetPanel 初回表示', () => {
  it('マウント時に fetchCircles が q なしで 1 回呼ばれ、全件表示される', async () => {
    renderPanel();
    await screen.findByText('Alpha Circle');

    expect(fetchCirclesMock).toHaveBeenCalledTimes(1);
    expect(fetchCirclesMock).toHaveBeenCalledWith({}, expect.anything());
    expect(screen.getByText('beta works')).toBeInTheDocument();
    expect(screen.getByText('ガンマ工房')).toBeInTheDocument();
  });
});

describe('CircleFacetPanel クライアントサイド絞り込み', () => {
  it('入力後も fetchCircles は再フェッチされない(呼び出し回数 1 のまま)', async () => {
    renderPanel();
    await screen.findByText('Alpha Circle');

    fireEvent.change(screen.getByPlaceholderText('サークルを絞り込み'), {
      target: { value: 'beta' },
    });

    await waitFor(() => {
      expect(screen.queryByText('Alpha Circle')).toBeNull();
    });
    expect(screen.getByText('beta works')).toBeInTheDocument();
    expect(fetchCirclesMock).toHaveBeenCalledTimes(1);
  });

  it('部分一致でフィルタされる', async () => {
    renderPanel();
    await screen.findByText('Alpha Circle');

    fireEvent.change(screen.getByPlaceholderText('サークルを絞り込み'), {
      target: { value: 'works' },
    });

    expect(screen.getByText('beta works')).toBeInTheDocument();
    expect(screen.queryByText('Alpha Circle')).toBeNull();
    expect(screen.queryByText('ガンマ工房')).toBeNull();
  });

  it('大文字小文字を無視してマッチする', async () => {
    renderPanel();
    await screen.findByText('Alpha Circle');

    fireEvent.change(screen.getByPlaceholderText('サークルを絞り込み'), {
      target: { value: 'ALPHA' },
    });

    expect(screen.getByText('Alpha Circle')).toBeInTheDocument();
    expect(screen.queryByText('beta works')).toBeNull();
  });

  it('マッチしない語では 0 件になり「サークルがありません」が表示される', async () => {
    renderPanel();
    await screen.findByText('Alpha Circle');

    fireEvent.change(screen.getByPlaceholderText('サークルを絞り込み'), {
      target: { value: '存在しない語' },
    });

    expect(screen.getByText('サークルがありません')).toBeInTheDocument();
  });

  it('絞り込みをクリアすると全件表示に戻る(再フェッチなし)', async () => {
    renderPanel();
    await screen.findByText('Alpha Circle');

    const input = screen.getByPlaceholderText('サークルを絞り込み');
    fireEvent.change(input, { target: { value: 'beta' } });
    expect(screen.queryByText('Alpha Circle')).toBeNull();

    fireEvent.change(input, { target: { value: '' } });
    expect(screen.getByText('Alpha Circle')).toBeInTheDocument();
    expect(screen.getByText('ガンマ工房')).toBeInTheDocument();
    expect(fetchCirclesMock).toHaveBeenCalledTimes(1);
  });
});

describe('CircleFacetPanel 選択・解除の挙動(既存動作の維持)', () => {
  it('クリックで onSelect にサークル名が渡る', async () => {
    const { onSelect } = renderPanel();
    await screen.findByText('Alpha Circle');

    fireEvent.click(screen.getByText('Alpha Circle').closest('button')!);
    expect(onSelect).toHaveBeenCalledWith('Alpha Circle');
  });

  it('選択中のサークルを再クリックすると解除(空文字列)される', async () => {
    const { onSelect } = renderPanel('Alpha Circle');
    await screen.findByText('Alpha Circle');

    fireEvent.click(screen.getByText('Alpha Circle').closest('button')!);
    expect(onSelect).toHaveBeenCalledWith('');
  });

  it('work_count が表示される', async () => {
    renderPanel();
    await screen.findByText('Alpha Circle');
    expect(screen.getByText('3')).toBeInTheDocument();
  });
});

describe('CircleFacetPanel 表示上限 MAX_DISPLAY=50', () => {
  it('フィルタ後の件数が 50 件を超える場合は上位 50 件のみ表示し注記が出る', async () => {
    const many: CircleFacet[] = Array.from({ length: 60 }, (_, i) => ({
      name: `Circle ${String(i).padStart(2, '0')}`,
      work_count: i,
    }));
    fetchCirclesMock.mockResolvedValue({ items: many });

    renderPanel();
    await screen.findByText('Circle 00');

    expect(screen.getByText('Circle 49')).toBeInTheDocument();
    expect(screen.queryByText('Circle 50')).toBeNull();
    expect(screen.getByText(/上位 50 件を表示中/)).toBeInTheDocument();
  });
});
