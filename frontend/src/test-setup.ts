// テスト共通セットアップ: @testing-library/jest-dom のカスタムマッチャを登録する。
import '@testing-library/jest-dom';

// jsdom には ResizeObserver が無く、react-zoom-pan-pinch(ImageViewer)が
// マウント時に参照して落ちるため、最小限の no-op 実装を与える。
if (typeof window.ResizeObserver === 'undefined') {
  class ResizeObserverStub {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  (window as unknown as { ResizeObserver: unknown }).ResizeObserver = ResizeObserverStub;
}
