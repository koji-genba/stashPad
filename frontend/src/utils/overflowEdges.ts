// 横スクロール要素の「隠れている内容」を判定する純関数。
// ScrollableText(useHorizontalOverflow 経由)から呼ばれ、フェード表示の on/off に使う。

export interface OverflowEdges {
  start: boolean;
  end: boolean;
}

interface ScrollMetrics {
  scrollLeft: number;
  scrollWidth: number;
  clientWidth: number;
}

// サブピクセル丸め対策の許容誤差。ブラウザ・ズーム倍率によって scrollLeft や
// scrollWidth - clientWidth が理論値からわずかにずれることがあるため、
// 1px 未満の差は「はみ出していない」とみなす。
const EDGE_TOLERANCE_PX = 1;

export function overflowEdges(m: ScrollMetrics): OverflowEdges {
  const hiddenEnd = m.scrollWidth - m.clientWidth - m.scrollLeft;
  return {
    start: m.scrollLeft > EDGE_TOLERANCE_PX,
    end: hiddenEnd > EDGE_TOLERANCE_PX,
  };
}
