// ScrollableText のテスト。
// useHorizontalOverflow フックの計測ロジックもここで一緒に検証する(設計メモ参照)。
//
// jsdom はレイアウトしないため scrollWidth/clientWidth は常に 0 になる。
// 要素インスタンスに Object.defineProperty で寸法を上書きして overflow 状態を作る。
//
// マウント時計測(useLayoutEffect 内の初回 measure())だけは ResizeObserver の
// コールバックを一切呼ばずに検証する必要があるため、render 前に
// HTMLElement.prototype.scrollWidth/clientWidth を spyOn で差し替える。
//
// テストリスト:
// - はみ出している(scrollWidth > clientWidth)とマウント時(ResizeObserver 未発火)に data-overflow-end が付く
// - 収まっていれば data-overflow-start/end のどちらも付かない
// - scroll イベント後、start が付き end が外れる(末尾までスクロールした状態)
// - ResizeObserver のコールバックで再計測される(要素サイズ変化 = 画面回転など)
// - アンマウントで scroll リスナと ResizeObserver.disconnect が呼ばれる
// - title props がそのまま span の title 属性になる
// - テキストが変わると(呼び出し側が key を付けなくても)自動的に再計測される
// - テキストが変わるとスクロール位置が先頭に戻る(内部で要素ごと作り直される)
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import ScrollableText from './ScrollableText';

/** 指定した要素の scrollWidth/clientWidth/scrollLeft を差し替える(jsdom はレイアウトしないため)。 */
function setDimensions(el: HTMLElement, dims: { scrollWidth: number; clientWidth: number; scrollLeft?: number }) {
  Object.defineProperty(el, 'scrollWidth', { configurable: true, value: dims.scrollWidth });
  Object.defineProperty(el, 'clientWidth', { configurable: true, value: dims.clientWidth });
  if (dims.scrollLeft !== undefined) {
    Object.defineProperty(el, 'scrollLeft', { configurable: true, value: dims.scrollLeft, writable: true });
  }
}

// ResizeObserver をモック化し、observe に渡されたコールバックを捕まえられるようにする。
// (test-setup.ts の no-op スタブは観測用フックを持たないので、ここで差し替える)
let roCallback: (() => void) | null = null;
let disconnectSpy = vi.fn();

beforeEach(() => {
  disconnectSpy = vi.fn();
  roCallback = null;
  vi.stubGlobal(
    'ResizeObserver',
    class {
      constructor(cb: () => void) {
        roCallback = cb;
      }
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {
        disconnectSpy();
      }
    },
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('ScrollableText', () => {
  it('はみ出している場合、マウント時(ResizeObserver 未発火)に data-overflow-end が付く', () => {
    // useLayoutEffect 内の初回 measure() だけで計測されることを検証したいので、
    // ResizeObserver のコールバック(roCallback)は一切呼ばない。
    // 要素インスタンスへの Object.defineProperty ではなく prototype への
    // spyOn を使うのは、render() より前に(要素が存在する前に)寸法を
    // 確定させておく必要があるため。
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(300);
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(100);

    render(<ScrollableText title="long-name.mp3">long-name.mp3</ScrollableText>);
    const el = screen.getByTitle('long-name.mp3');

    expect(el).toHaveAttribute('data-overflow-end', 'true');
    expect(el).not.toHaveAttribute('data-overflow-start');
  });

  it('収まっていれば data-overflow-start/end のどちらも付かない', () => {
    render(<ScrollableText title="short.mp3">short.mp3</ScrollableText>);
    const el = screen.getByTitle('short.mp3');
    setDimensions(el, { scrollWidth: 100, clientWidth: 100, scrollLeft: 0 });
    act(() => roCallback?.());

    expect(el).not.toHaveAttribute('data-overflow-start');
    expect(el).not.toHaveAttribute('data-overflow-end');
  });

  it('scroll イベント後、末尾までスクロールしていれば start が付き end が外れる', () => {
    render(<ScrollableText title="long-name.mp3">long-name.mp3</ScrollableText>);
    const el = screen.getByTitle('long-name.mp3');
    setDimensions(el, { scrollWidth: 300, clientWidth: 100, scrollLeft: 0 });
    act(() => roCallback?.());
    expect(el).toHaveAttribute('data-overflow-end', 'true');

    // 末尾までスクロール(scrollWidth - clientWidth = 200)
    setDimensions(el, { scrollWidth: 300, clientWidth: 100, scrollLeft: 200 });
    fireEvent.scroll(el);

    expect(el).toHaveAttribute('data-overflow-start', 'true');
    expect(el).not.toHaveAttribute('data-overflow-end');
  });

  it('ResizeObserver のコールバックで再計測される', () => {
    render(<ScrollableText title="name.mp3">name.mp3</ScrollableText>);
    const el = screen.getByTitle('name.mp3');
    // マウント直後は収まっている(0/0)ので属性なし
    expect(el).not.toHaveAttribute('data-overflow-end');

    // 画面回転等でサイズが変わり、はみ出す状態になったとみなす
    setDimensions(el, { scrollWidth: 300, clientWidth: 100, scrollLeft: 0 });
    act(() => roCallback?.());

    expect(el).toHaveAttribute('data-overflow-end', 'true');
  });

  it('アンマウントで scroll リスナと ResizeObserver.disconnect が解除される', () => {
    const { unmount } = render(<ScrollableText title="name.mp3">name.mp3</ScrollableText>);
    const el = screen.getByTitle('name.mp3');
    const removeSpy = vi.spyOn(el, 'removeEventListener');

    unmount();

    expect(disconnectSpy).toHaveBeenCalledTimes(1);
    expect(removeSpy).toHaveBeenCalledWith('scroll', expect.any(Function));
  });

  it('title props がそのまま span の title 属性になる', () => {
    render(<ScrollableText title="フルネーム.mp3">表示テキスト</ScrollableText>);
    expect(screen.getByTitle('フルネーム.mp3')).toBeInTheDocument();
  });
});

describe('ScrollableText テキスト変更時の挙動(issue #107 プレイヤーでの曲名変化対応)', () => {
  it('テキストが変わると呼び出し側で key を付けなくても自動的に再計測される', () => {
    // scrollWidth はテキストの長さに比例する想定のダミー実装。
    // clientWidth は固定し、短いテキストでは収まり・長いテキストでははみ出すようにする。
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(100);
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockImplementation(function (this: HTMLElement) {
      return (this.textContent?.length ?? 0) * 20;
    });

    const { rerender } = render(<ScrollableText title="short">short</ScrollableText>);
    // "short" は 5 文字 * 20 = 100 = clientWidth なので収まっている
    expect(screen.getByTitle('short')).not.toHaveAttribute('data-overflow-end');

    const longText = 'a much longer track name that definitely overflows';
    rerender(<ScrollableText title={longText}>{longText}</ScrollableText>);

    expect(screen.getByTitle(longText)).toHaveAttribute('data-overflow-end', 'true');
  });

  it('テキストが変わるとスクロール位置が先頭に戻る', () => {
    const { rerender } = render(<ScrollableText title="short">short</ScrollableText>);
    const before = screen.getByTitle('short');
    // 末尾までスクロールして読んでいた状態を模す
    Object.defineProperty(before, 'scrollLeft', { configurable: true, value: 120, writable: true });
    expect(before.scrollLeft).toBe(120);

    rerender(<ScrollableText title="long name after change">long name after change</ScrollableText>);
    const after = screen.getByTitle('long name after change');

    expect(after.scrollLeft).toBe(0);
  });
});
