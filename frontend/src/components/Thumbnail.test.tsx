// Thumbnail コンポーネントのテスト。
// img + onError ハンドラの状態管理を DOM 直接操作ではなく React state で行うことを担保する。
import { afterEach, describe, expect, it } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import Thumbnail from './Thumbnail';

describe('Thumbnail', () => {
  afterEach(() => cleanup());

  it('src / alt / className / loading を img にそのまま渡す', () => {
    render(
      <Thumbnail
        src="/api/works/1/thumbnail"
        alt="作品サムネ"
        className="my-thumb"
        loading="lazy"
      />,
    );
    const img = screen.getByAltText('作品サムネ') as HTMLImageElement;
    expect(img.getAttribute('src')).toBe('/api/works/1/thumbnail');
    expect(img.className).toBe('my-thumb');
    expect(img.getAttribute('loading')).toBe('lazy');
  });

  it('alt 未指定なら空文字(装飾画像扱い)', () => {
    render(<Thumbnail src="/api/works/1/thumbnail" />);
    const img = document.querySelector('img')!;
    expect(img.getAttribute('alt')).toBe('');
  });

  it('初期状態では visibility は隠されていない', () => {
    render(<Thumbnail src="/api/works/1/thumbnail" alt="x" />);
    const img = screen.getByAltText('x');
    expect(img.style.visibility).not.toBe('hidden');
  });

  it('onError が発火すると visibility:hidden が付く', () => {
    render(<Thumbnail src="/api/works/1/thumbnail" alt="x" />);
    const img = screen.getByAltText('x');
    fireEvent.error(img);
    expect(img.style.visibility).toBe('hidden');
  });

  it('src が変わると broken 状態がリセットされる(同じ要素が再利用されても)', () => {
    const { rerender } = render(
      <Thumbnail src="/api/works/1/thumbnail" alt="x" />,
    );
    const img = screen.getByAltText('x');
    fireEvent.error(img);
    expect(img.style.visibility).toBe('hidden');

    // src を変えると broken が false に戻る
    rerender(<Thumbnail src="/api/works/2/thumbnail" alt="x" />);
    expect(img.style.visibility).not.toBe('hidden');
    expect(img.getAttribute('src')).toBe('/api/works/2/thumbnail');
  });

  it('同じ src で再レンダリングしても broken は維持される', () => {
    const { rerender } = render(
      <Thumbnail src="/api/works/1/thumbnail" alt="x" />,
    );
    const img = screen.getByAltText('x');
    fireEvent.error(img);
    rerender(<Thumbnail src="/api/works/1/thumbnail" alt="x" />);
    expect(img.style.visibility).toBe('hidden');
  });
});
