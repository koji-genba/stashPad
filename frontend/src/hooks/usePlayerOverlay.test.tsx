// usePlayerOverlay フックのテスト。
// オーバーレイ(フルスクリーンプレイヤー/キュー画面)の表示状態を history(location.state)
// で表現し、Android の「戻る」で 1 段ずつ閉じられることを検証する。
// MemoryRouter の navigate(-1) はブラウザの「戻る」と同じ経路(history スタックの pop)。
import { StrictMode, useEffect } from 'react';
import { afterEach, describe, expect, it } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { usePlayerOverlay } from './usePlayerOverlay';

// フックの状態と操作ボタンを露出するテスト用プローブ。
// suffix で区別して同一画面に複数インスタンスを置けるようにする(二重発火ガードの検証用)。
function Probe({ suffix = '' }: { suffix?: string }) {
  const location = useLocation();
  const o = usePlayerOverlay();
  return (
    <div>
      <div data-testid={`flags${suffix}`}>
        {`player=${o.playerOpen} queue=${o.queueOpen}`}
      </div>
      <div data-testid={`path${suffix}`}>{location.pathname}</div>
      <div data-testid={`pageState${suffix}`}>
        {JSON.stringify((location.state as Record<string, unknown> | null)?.memo ?? null)}
      </div>
      <button onClick={() => o.openPlayer()}>openPlayer{suffix}</button>
      <button onClick={() => o.openQueue()}>openQueue{suffix}</button>
      <button onClick={() => o.closePlayer()}>closePlayer{suffix}</button>
      <button onClick={() => o.closeQueue()}>closeQueue{suffix}</button>
      <button onClick={() => o.unwind()}>unwind{suffix}</button>
      {/* 連打(同一 history エントリからの多重 back)を再現するボタン */}
      <button
        onClick={() => {
          o.closePlayer();
          o.closePlayer();
        }}
      >
        closePlayerTwice{suffix}
      </button>
      <button
        onClick={() => {
          o.openPlayer();
          o.openPlayer();
        }}
      >
        openPlayerTwice{suffix}
      </button>
      <button
        onClick={() => {
          o.unwind();
          o.unwind();
        }}
      >
        unwindTwice{suffix}
      </button>
    </div>
  );
}

function renderProbe(
  ui: React.ReactNode,
  initialEntries: ({ pathname: string; state?: unknown } | string)[] = ['/'],
) {
  return render(
    <MemoryRouter initialEntries={initialEntries} initialIndex={initialEntries.length - 1}>
      {ui}
    </MemoryRouter>,
  );
}

const flags = (suffix = '') => screen.getByTestId(`flags${suffix}`).textContent;
const path = (suffix = '') => screen.getByTestId(`path${suffix}`).textContent;
const click = (name: string) => fireEvent.click(screen.getByRole('button', { name }));

describe('usePlayerOverlay 基本の開閉', () => {
  afterEach(cleanup);

  it('初期状態(state なし)では両オーバーレイとも閉じている', () => {
    renderProbe(<Probe />);
    expect(flags()).toBe('player=false queue=false');
  });

  it('openPlayer でプレイヤーが開く(URL は変わらない)', () => {
    renderProbe(<Probe />, [{ pathname: '/works/1' }]);
    click('openPlayer');
    expect(flags()).toBe('player=true queue=false');
    expect(path()).toBe('/works/1');
  });

  it('openQueue でキュー画面が開く(プレイヤーが開いていることが前提)', () => {
    renderProbe(<Probe />);
    click('openPlayer');
    click('openQueue');
    expect(flags()).toBe('player=true queue=true');
  });

  it('プレイヤーが閉じたまま openQueue しても何も起きない', () => {
    renderProbe(<Probe />);
    click('openQueue');
    expect(flags()).toBe('player=false queue=false');
  });

  it('closeQueue でキュー画面だけが閉じ、プレイヤーは残る(戻る 1 回相当)', () => {
    renderProbe(<Probe />);
    click('openPlayer');
    click('openQueue');
    click('closeQueue');
    expect(flags()).toBe('player=true queue=false');
  });

  it('closePlayer でプレイヤーが閉じて元のページに戻る', () => {
    renderProbe(<Probe />, [{ pathname: '/works/2' }]);
    click('openPlayer');
    click('closePlayer');
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/2');
  });

  it('キュー画面が開いている間は closePlayer は無効(キューを先に閉じる)', () => {
    renderProbe(<Probe />);
    click('openPlayer');
    click('openQueue');
    click('closePlayer');
    expect(flags()).toBe('player=true queue=true');
  });

  it('ページ自身の location.state を壊さない(open 時にマージ、閉じれば元どおり)', () => {
    renderProbe(<Probe />, [{ pathname: '/', state: { memo: 'keep' } }]);
    click('openPlayer');
    expect(screen.getByTestId('pageState').textContent).toBe('"keep"');
    click('closePlayer');
    expect(screen.getByTestId('pageState').textContent).toBe('"keep"');
  });
});

describe('usePlayerOverlay 戻る操作との整合(多重発火ガード)', () => {
  afterEach(cleanup);

  it('openPlayer を同一 tick に 2 回呼んでも 1 段しか積まない(close 1 回で全部閉じる)', () => {
    renderProbe(<Probe />, ['/list', '/works/3']);
    click('openPlayerTwice');
    expect(flags()).toBe('player=true queue=false');
    click('closePlayer');
    // 2 回積まれていたら 1 回の close ではプレイヤーが開いたままになる
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/3');
  });

  it('closePlayer を同一 tick に 2 回呼んでも 1 段しか戻らない(下のページを突き抜けない)', () => {
    renderProbe(<Probe />, ['/list', '/works/3']);
    click('openPlayer');
    click('closePlayerTwice');
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/3'); // /list まで戻ってしまったら NG
  });

  it('別インスタンス同士でも多重 back しない(Escape とボタンの同時押し相当)', () => {
    renderProbe(
      <>
        <Probe />
        <Probe suffix="B" />
      </>,
      ['/list', '/works/3'],
    );
    click('openPlayer');
    // 2 つのフックインスタンスから同一 tick に close
    fireEvent.click(screen.getByRole('button', { name: 'closePlayer' }));
    fireEvent.click(screen.getByRole('button', { name: 'closePlayerB' }));
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/3');
  });

  it('一度閉じて同じエントリからもう一度開閉できる(ガードが固着しない)', () => {
    renderProbe(<Probe />, ['/list', '/works/3']);
    click('openPlayer');
    click('closePlayer');
    click('openPlayer');
    expect(flags()).toBe('player=true queue=false');
    click('closePlayer');
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/3');
  });
});

describe('usePlayerOverlay unwind(キューが空になったときの巻き戻し)', () => {
  afterEach(cleanup);

  it('プレイヤー+キュー画面の 2 段を一括で巻き戻す', () => {
    renderProbe(<Probe />, ['/list', '/works/3']);
    click('openPlayer');
    click('openQueue');
    click('unwind');
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/3');
  });

  it('プレイヤーのみなら 1 段だけ巻き戻す', () => {
    renderProbe(<Probe />, ['/list', '/works/3']);
    click('openPlayer');
    click('unwind');
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/3');
  });

  it('何も開いていなければ何もしない', () => {
    renderProbe(<Probe />, ['/list', '/works/3']);
    click('unwind');
    expect(path()).toBe('/works/3');
  });

  it('unwind の多重呼び出しでも余計に戻らない', () => {
    renderProbe(<Probe />, ['/list', '/works/3']);
    click('openPlayer');
    click('unwindTwice');
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/3');
  });

  it('StrictMode(effect 二重実行)でも unwind が暴発しない', () => {
    // StrictMode では mount 時に effect が 2 回走る。unwind を effect から呼ぶ
    // 利用側(AudioPlayer)を想定し、二重実行しても 1 段しか戻らないことを確認する。
    function UnwindOnMount() {
      const o = usePlayerOverlay();
      // 利用側は「キューが空なら巻き戻す」を effect で行う。依存配列なし=毎コミット実行の
      // 乱暴な利用 + StrictMode 二重実行でも、1 段しか戻らないことを確認する
      useEffect(() => {
        if (o.playerOpen) o.unwind();
      });
      return <Probe />;
    }
    render(
      <StrictMode>
        <MemoryRouter
          initialEntries={[
            '/list',
            { pathname: '/works/3' },
            { pathname: '/works/3', state: { fsPlayer: true } },
          ]}
          initialIndex={2}
        >
          <UnwindOnMount />
        </MemoryRouter>
      </StrictMode>,
    );
    expect(flags()).toBe('player=false queue=false');
    expect(path()).toBe('/works/3');
  });
});
