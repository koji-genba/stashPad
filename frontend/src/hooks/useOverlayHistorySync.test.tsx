// useOverlayHistorySync のテスト。
// 画像/動画/テキストのオーバーレイ(overlayStore)と history の同期を検証する:
// - store が開いたら mediaOverlay フラグ付きエントリが積まれる
// - Android の「戻る」(= navigate(-1))でオーバーレイが閉じる
// - ✕ / Escape 等で store が直接閉じたらフラグエントリが巻き戻される
// - リロード直後にフラグだけ残った場合も巻き戻して掃除する
// MemoryRouter の navigate(-1) はブラウザの「戻る」と同じ経路(history スタックの pop)。
import { StrictMode } from 'react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { useOverlayStore } from '@/store/overlayStore';
import { __resetForTests } from './useGuardedHistoryNav';
import { useOverlayHistorySync, MEDIA_OVERLAY_FLAG } from './useOverlayHistorySync';

function resetStore() {
  useOverlayStore.setState({ image: null, video: null, text: null }, false);
}

// フックの状態と操作ボタンを露出するテスト用プローブ。
function Probe() {
  useOverlayHistorySync();
  const location = useLocation();
  const navigate = useNavigate();
  const video = useOverlayStore((s) => s.video);
  const image = useOverlayStore((s) => s.image);
  const flag = (location.state as Record<string, unknown> | null)?.[MEDIA_OVERLAY_FLAG] === true;
  const memo = (location.state as Record<string, unknown> | null)?.memo ?? null;
  return (
    <div>
      <div data-testid="status">
        {`flag=${flag} video=${video !== null} image=${image !== null} memo=${JSON.stringify(memo)}`}
      </div>
      <button
        onClick={() =>
          useOverlayStore
            .getState()
            .openVideo({ workId: 1, workTitle: 'w', path: 'v.mp4', name: 'v.mp4' })
        }
      >
        openVideo
      </button>
      <button
        onClick={() =>
          useOverlayStore.getState().openImage({
            workId: 1,
            dir: '',
            entries: [{ name: 'p1.jpg', is_dir: false, size: 0, media_kind: 'image' }],
            startName: 'p1.jpg',
          })
        }
      >
        openImage
      </button>
      <button onClick={() => useOverlayStore.getState().closeVideo()}>closeVideo</button>
      <button onClick={() => useOverlayStore.getState().closeImage()}>closeImage</button>
      <button onClick={() => navigate(-1)}>browserBack</button>
    </div>
  );
}

function statusText() {
  return screen.getByTestId('status').textContent ?? '';
}

describe('useOverlayHistorySync', () => {
  beforeEach(() => {
    resetStore();
    __resetForTests();
  });
  afterEach(cleanup);

  it('store が開くとフラグ付きエントリが積まれ、戻るで閉じる', async () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: '/works/1', state: { memo: 'base' } }]}>
        <Probe />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByText('openVideo'));
    // push は effect 経由なので反映を待つ
    await waitFor(() => expect(statusText()).toContain('flag=true video=true'));
    // 元エントリの state(memo)は引き継がれる
    expect(statusText()).toContain('memo="base"');

    // Android の「戻る」相当
    fireEvent.click(screen.getByText('browserBack'));
    await waitFor(() => expect(statusText()).toContain('video=false'));
    expect(statusText()).toContain('flag=false');
    expect(statusText()).toContain('memo="base"'); // 元のエントリに戻っている
  });

  it('✕ 等で store が直接閉じられたらフラグエントリを巻き戻す', async () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: '/works/1', state: { memo: 'base' } }]}>
        <Probe />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByText('openVideo'));
    await waitFor(() => expect(statusText()).toContain('flag=true video=true'));

    fireEvent.click(screen.getByText('closeVideo'));
    // store が閉じ、フラグエントリが 1 段巻き戻される
    await waitFor(() => expect(statusText()).toContain('flag=false video=false'));
    expect(statusText()).toContain('memo="base"');
  });

  it('image オーバーレイでも同様に動く', async () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: '/works/1', state: { memo: 'base' } }]}>
        <Probe />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByText('openImage'));
    await waitFor(() => expect(statusText()).toContain('image=true'));
    expect(statusText()).toContain('flag=true');

    fireEvent.click(screen.getByText('browserBack'));
    await waitFor(() => expect(statusText()).toContain('image=false'));
    expect(statusText()).toContain('flag=false');
  });

  it('StrictMode でも push は 1 回だけ(戻る 1 回で元エントリに戻る)', async () => {
    render(
      <StrictMode>
        <MemoryRouter initialEntries={[{ pathname: '/works/1', state: { memo: 'base' } }]}>
          <Probe />
        </MemoryRouter>
      </StrictMode>,
    );

    fireEvent.click(screen.getByText('openVideo'));
    await waitFor(() => expect(statusText()).toContain('flag=true video=true'));

    fireEvent.click(screen.getByText('browserBack'));
    await waitFor(() => expect(statusText()).toContain('video=false'));
    // 二重 push されていれば 1 回の戻るでは flag が残る
    expect(statusText()).toContain('flag=false');
    expect(statusText()).toContain('memo="base"');
  });

  it('リロード等でフラグだけ残った場合は巻き戻して掃除する', async () => {
    render(
      <MemoryRouter
        initialEntries={[
          { pathname: '/works/1', state: { memo: 'base' } },
          { pathname: '/works/1', state: { memo: 'base', [MEDIA_OVERLAY_FLAG]: true } },
        ]}
        initialIndex={1}
      >
        <Probe />
      </MemoryRouter>,
    );

    // store は空なのでフラグエントリが自動で巻き戻される
    await waitFor(() => expect(statusText()).toContain('flag=false'));
    expect(statusText()).toContain('memo="base"');
  });

  it('オーバーレイ表示中にページ遷移するとオーバーレイが閉じる', async () => {
    function NavProbe() {
      const navigate = useNavigate();
      return <button onClick={() => navigate('/other')}>goOther</button>;
    }
    render(
      <MemoryRouter initialEntries={[{ pathname: '/works/1', state: { memo: 'base' } }]}>
        <Probe />
        <NavProbe />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByText('openVideo'));
    await waitFor(() => expect(statusText()).toContain('flag=true video=true'));

    fireEvent.click(screen.getByText('goOther'));
    await waitFor(() => expect(statusText()).toContain('video=false'));
  });
});
