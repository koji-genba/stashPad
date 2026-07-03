// Overlays(VideoModal / TextModal / ImageViewer)の issue #52 対応テスト:
// - VideoModal / TextModal も Escape で閉じられる
// - 3 オーバーレイとも表示中は body スクロールをロックする
// - <Overlays> が history 同期フックをマウントしている(戻るで閉じられる)
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { useOverlayStore } from '@/store/overlayStore';
import { __resetForTests } from '@/hooks/useBodyScrollLock';
import { MEDIA_OVERLAY_FLAG } from '@/hooks/useOverlayHistorySync';
import Overlays from './Overlays';

vi.mock('@/api/client', () => ({
  fetchTextFile: vi.fn().mockResolvedValue('テキスト本文'),
  fileUrl: (workId: number, path: string) => `/api/works/${workId}/file?path=${path}`,
  recordPlay: vi.fn().mockResolvedValue(undefined),
}));

function resetStore() {
  useOverlayStore.setState({ image: null, video: null, text: null }, false);
}

function openVideo() {
  useOverlayStore
    .getState()
    .openVideo({ workId: 1, workTitle: 'w', path: 'v.mp4', name: 'v.mp4' });
}

function openText() {
  useOverlayStore.getState().openText({ workId: 1, path: 't.txt', name: 't.txt' });
}

function openImage() {
  useOverlayStore.getState().openImage({
    workId: 1,
    dir: '',
    entries: [{ name: 'p1.jpg', is_dir: false, size: 0, media_kind: 'image' }],
    startName: 'p1.jpg',
  });
}

function renderOverlays() {
  return render(
    <MemoryRouter initialEntries={['/works/1']}>
      <Overlays />
    </MemoryRouter>,
  );
}

describe('Overlays の Escape / スクロールロック (issue #52)', () => {
  beforeEach(() => {
    resetStore();
    __resetForTests();
    document.body.style.overflow = '';
  });
  afterEach(cleanup);

  it('VideoModal は Escape で閉じる', async () => {
    renderOverlays();
    openVideo();
    await screen.findByText('v.mp4');

    fireEvent.keyDown(window, { key: 'Escape' });
    await waitFor(() => expect(useOverlayStore.getState().video).toBeNull());
  });

  it('TextModal は Escape で閉じる', async () => {
    renderOverlays();
    openText();
    await screen.findByText('t.txt');

    fireEvent.keyDown(window, { key: 'Escape' });
    await waitFor(() => expect(useOverlayStore.getState().text).toBeNull());
  });

  it('VideoModal 表示中は body スクロールがロックされ、閉じると解除される', async () => {
    renderOverlays();
    openVideo();
    await screen.findByText('v.mp4');
    expect(document.body.style.overflow).toBe('hidden');

    useOverlayStore.getState().closeVideo();
    await waitFor(() => expect(document.body.style.overflow).toBe(''));
  });

  it('TextModal 表示中は body スクロールがロックされる', async () => {
    renderOverlays();
    openText();
    await screen.findByText('t.txt');
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('ImageViewer 表示中は body スクロールがロックされる', async () => {
    renderOverlays();
    openImage();
    await screen.findByLabelText('次のページ');
    expect(document.body.style.overflow).toBe('hidden');
  });
});

describe('Overlays の history 同期 (issue #52)', () => {
  beforeEach(() => {
    resetStore();
    __resetForTests();
  });
  afterEach(cleanup);

  it('<Overlays> が同期フックをマウントしている(開くとフラグが積まれる)', async () => {
    function FlagProbe() {
      const location = useLocation();
      const flag =
        (location.state as Record<string, unknown> | null)?.[MEDIA_OVERLAY_FLAG] === true;
      return <div data-testid="flag">{String(flag)}</div>;
    }
    render(
      <MemoryRouter initialEntries={['/works/1']}>
        <Overlays />
        <FlagProbe />
      </MemoryRouter>,
    );

    openVideo();
    await waitFor(() => expect(screen.getByTestId('flag').textContent).toBe('true'));
  });
});
