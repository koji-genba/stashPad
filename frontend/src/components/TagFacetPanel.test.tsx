// TagFacetPanel のテスト。
// issue #27: q が空でも非空でも共有 tagStore の全件キャッシュを参照し、
// 絞り込み入力はクライアントサイド(name.toLowerCase().includes)で行う。
// これにより q 入力時にサーバへ再フェッチ(fetchTags)しないことを確認する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';

// api/client をモック化(tagStore.test.ts と同様の手法)。
vi.mock('@/api/client', () => ({
  fetchTags: vi.fn(),
}));

import { fetchTags } from '@/api/client';
import { useTagStore } from '@/store/tagStore';
import type { TagFacet } from '@/api/types';
import TagFacetPanel from './TagFacetPanel';

const fetchTagsMock = vi.mocked(fetchTags);

function makeTag(id: number, name: string, category = 'genre', work_count = id): TagFacet {
  return { id, name, category, work_count };
}

const SAMPLE_TAGS: TagFacet[] = [
  makeTag(1, 'ファンタジー'),
  makeTag(2, 'Fantasy World'),
  makeTag(3, '現代'),
];

function resetStore() {
  useTagStore.setState({ items: [], loaded: false, loading: false, error: null }, false);
}

function renderPanel() {
  const onToggle = vi.fn();
  const utils = render(<TagFacetPanel selected={[]} excluded={[]} onToggle={onToggle} />);
  return { onToggle, ...utils };
}

beforeEach(() => {
  resetStore();
  fetchTagsMock.mockReset();
  fetchTagsMock.mockResolvedValue({ items: SAMPLE_TAGS });
});

afterEach(() => {
  cleanup();
});

describe('TagFacetPanel 初回表示', () => {
  it('マウント時に fetchTags が q なしで 1 回呼ばれ、全件表示される', async () => {
    renderPanel();
    await screen.findByText('ファンタジー');

    expect(fetchTagsMock).toHaveBeenCalledTimes(1);
    expect(fetchTagsMock).toHaveBeenCalledWith({});
    expect(screen.getByText('Fantasy World')).toBeInTheDocument();
    expect(screen.getByText('現代')).toBeInTheDocument();
  });

  it('すでに tagStore がロード済みなら fetchTags を呼ばずにそのまま表示する', async () => {
    useTagStore.setState({ items: SAMPLE_TAGS, loaded: true, loading: false, error: null });

    renderPanel();
    await screen.findByText('ファンタジー');

    expect(fetchTagsMock).not.toHaveBeenCalled();
  });
});

describe('TagFacetPanel クライアントサイド絞り込み', () => {
  it('入力後も fetchTags は再フェッチされない(呼び出し回数 1 のまま)', async () => {
    renderPanel();
    await screen.findByText('ファンタジー');

    fireEvent.change(screen.getByPlaceholderText('タグを絞り込み'), {
      target: { value: 'fantasy' },
    });

    expect(screen.queryByText('現代')).toBeNull();
    expect(fetchTagsMock).toHaveBeenCalledTimes(1);
  });

  it('部分一致でフィルタされる', async () => {
    renderPanel();
    await screen.findByText('ファンタジー');

    fireEvent.change(screen.getByPlaceholderText('タグを絞り込み'), {
      target: { value: 'World' },
    });

    expect(screen.getByText('Fantasy World')).toBeInTheDocument();
    expect(screen.queryByText('ファンタジー')).toBeNull();
    expect(screen.queryByText('現代')).toBeNull();
  });

  it('大文字小文字を無視してマッチする', async () => {
    renderPanel();
    await screen.findByText('ファンタジー');

    fireEvent.change(screen.getByPlaceholderText('タグを絞り込み'), {
      target: { value: 'FANTASY' },
    });

    expect(screen.getByText('Fantasy World')).toBeInTheDocument();
    expect(screen.queryByText('現代')).toBeNull();
  });

  it('絞り込みをクリアすると全件表示に戻る(再フェッチなし)', async () => {
    renderPanel();
    await screen.findByText('ファンタジー');

    const input = screen.getByPlaceholderText('タグを絞り込み');
    fireEvent.change(input, { target: { value: 'fantasy' } });
    expect(screen.queryByText('現代')).toBeNull();

    fireEvent.change(input, { target: { value: '' } });
    expect(screen.getByText('現代')).toBeInTheDocument();
    expect(fetchTagsMock).toHaveBeenCalledTimes(1);
  });
});

describe('TagFacetPanel fetch 失敗時の再試行導線 (issue #70)', () => {
  it('fetch 失敗でエラーメッセージと再試行ボタンが表示され、クリックで store が再取得されて表示に戻る', async () => {
    fetchTagsMock
      .mockRejectedValueOnce(new Error('Network error'))
      .mockResolvedValueOnce({ items: SAMPLE_TAGS });

    renderPanel();

    // 共有 tagStore の error 経由でエラー表示になる
    await screen.findByText('タグ一覧の読み込みに失敗しました');

    // 再試行 → store の refresh() が走り、成功すると一覧が表示される
    fireEvent.click(screen.getByRole('button', { name: '再試行' }));

    await screen.findByText('ファンタジー');
    expect(fetchTagsMock).toHaveBeenCalledTimes(2);
    expect(screen.queryByText('タグ一覧の読み込みに失敗しました')).toBeNull();
  });
});

describe('TagFacetPanel 選択・除外の挙動(既存動作の維持)', () => {
  it('クリックで onToggle にタグ id が渡る', async () => {
    const { onToggle } = renderPanel();
    await screen.findByText('ファンタジー');

    fireEvent.click(screen.getByText('ファンタジー').closest('button')!);
    expect(onToggle).toHaveBeenCalledWith(1);
  });

  it('work_count が表示される', async () => {
    renderPanel();
    await screen.findByText('ファンタジー');
    // makeTag(1, 'ファンタジー') の work_count は 1
    const btn = screen.getByText('ファンタジー').closest('button')!;
    expect(btn).toHaveTextContent('1');
  });
});
