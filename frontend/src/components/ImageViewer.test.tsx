// ImageViewer(画像ビューア)のテスト。issue #28: ズーム/パンと先読み拡張。
// react-zoom-pan-pinch は内部で ResizeObserver を使うが、jsdom には無いため
// ImageViewer.tsx モジュール側で最小限のフォールバックを用意している(import 時に適用される)。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { useOverlayStore } from '@/store/overlayStore';
import { isZoomed } from './ImageViewer';
import ImageViewer from './ImageViewer';

vi.mock('@/api/client', () => ({
  fileUrl: (workId: number, path: string) => `/api/works/${workId}/file?path=${encodeURIComponent(path)}`,
}));

function resetStore() {
  useOverlayStore.setState({ image: null, video: null, text: null }, false);
}

/** pages 枚数の image state をストアに積んで startIndex ページ目を開く */
function openPages(count: number, startIndex: number) {
  const entries = Array.from({ length: count }, (_, i) => ({
    name: `p${i}.jpg`,
    is_dir: false,
    size: 0,
    media_kind: 'image' as const,
  }));
  useOverlayStore.getState().openImage({
    workId: 1,
    dir: '',
    entries,
    startName: `p${startIndex}.jpg`,
  });
}

describe('ImageViewer プリロード', () => {
  beforeEach(resetStore);
  afterEach(cleanup);

  it('5 ページ中 index=0 表示時、非表示プリロード img が 3 枚(+1/+2/+3)', () => {
    openPages(5, 0);
    render(<ImageViewer />);
    const preloads = screen.getAllByTestId('preload-image');
    expect(preloads).toHaveLength(3);
  });

  it('残り 1 ページしか無い場合はプリロードが溢れず 1 枚だけになる', () => {
    // 5 ページ中 index=3 → 残りは index=4 の 1 枚のみ
    openPages(5, 3);
    render(<ImageViewer />);
    const preloads = screen.getAllByTestId('preload-image');
    expect(preloads).toHaveLength(1);
  });

  it('最終ページではプリロード img が存在しない', () => {
    openPages(5, 4);
    render(<ImageViewer />);
    expect(screen.queryAllByTestId('preload-image')).toHaveLength(0);
  });
});

describe('ImageViewer 等倍時の表示', () => {
  beforeEach(resetStore);
  afterEach(cleanup);

  it('tapPrev / tapNext ボタンが存在する', () => {
    openPages(3, 1);
    render(<ImageViewer />);
    expect(screen.getByLabelText('前のページ')).toBeInTheDocument();
    expect(screen.getByLabelText('次のページ')).toBeInTheDocument();
  });

  it('ページ番号と閉じるボタンが表示される', () => {
    openPages(3, 1);
    render(<ImageViewer />);
    expect(screen.getByText('2 / 3')).toBeInTheDocument();
    expect(screen.getByLabelText('閉じる')).toBeInTheDocument();
  });
});

describe('ImageViewer キーボード操作', () => {
  beforeEach(resetStore);
  afterEach(cleanup);

  it('ArrowRight でページが進む', () => {
    openPages(3, 0);
    render(<ImageViewer />);
    fireEvent.keyDown(window, { key: 'ArrowRight' });
    expect(useOverlayStore.getState().image?.index).toBe(1);
  });

  it('ArrowLeft でページが戻る', () => {
    openPages(3, 1);
    render(<ImageViewer />);
    fireEvent.keyDown(window, { key: 'ArrowLeft' });
    expect(useOverlayStore.getState().image?.index).toBe(0);
  });

  it('Escape で閉じる', () => {
    openPages(3, 0);
    render(<ImageViewer />);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(useOverlayStore.getState().image).toBeNull();
  });
});

describe('ImageViewer ページ切替', () => {
  beforeEach(resetStore);
  afterEach(cleanup);

  it('次ページボタンでページが進む', () => {
    openPages(3, 0);
    render(<ImageViewer />);
    fireEvent.click(screen.getByLabelText('次のページ'));
    expect(useOverlayStore.getState().image?.index).toBe(1);
  });

  it('前ページボタンでページが戻る', () => {
    openPages(3, 1);
    render(<ImageViewer />);
    fireEvent.click(screen.getByLabelText('前のページ'));
    expect(useOverlayStore.getState().image?.index).toBe(0);
  });
});

describe('ImageViewer ズーム連動(実描画)', () => {
  beforeEach(resetStore);
  afterEach(cleanup);

  it('ダブルクリックでズームすると tapPrev/tapNext が非表示になる', async () => {
    openPages(3, 0);
    render(<ImageViewer />);
    const img = screen.getByAltText('p0.jpg');
    fireEvent.doubleClick(img);
    await waitFor(() => {
      expect(screen.queryByLabelText('前のページ')).not.toBeInTheDocument();
      expect(screen.queryByLabelText('次のページ')).not.toBeInTheDocument();
    });
  });

  it('ページ送りで transform がリセットされ、次ページでは tapPrev/tapNext が再表示される', async () => {
    openPages(3, 0);
    render(<ImageViewer />);
    const img = screen.getByAltText('p0.jpg');
    fireEvent.doubleClick(img);
    await waitFor(() => {
      expect(screen.queryByLabelText('次のページ')).not.toBeInTheDocument();
    });
    // ← キーでのページ送りはズーム中でも有効
    fireEvent.keyDown(window, { key: 'ArrowRight' });
    await waitFor(() => {
      expect(useOverlayStore.getState().image?.index).toBe(1);
    });
    await waitFor(() => {
      expect(screen.getByLabelText('次のページ')).toBeInTheDocument();
    });
  });
});

describe('isZoomed(スケールしきい値の純関数)', () => {
  it('scale=1 は zoomed ではない', () => {
    expect(isZoomed(1)).toBe(false);
  });
  it('scale=1.01 は閾値以下として zoomed ではない', () => {
    expect(isZoomed(1.01)).toBe(false);
  });
  it('scale=1.02 は zoomed', () => {
    expect(isZoomed(1.02)).toBe(true);
  });
  it('scale=2.5 は zoomed', () => {
    expect(isZoomed(2.5)).toBe(true);
  });
});
