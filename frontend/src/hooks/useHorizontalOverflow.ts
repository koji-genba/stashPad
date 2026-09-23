// 横スクロール要素の左右はみ出し状態を追跡するフック。ScrollableText から使う。
//
// 計測タイミングはマウント直後(useLayoutEffect)・scroll イベント・ResizeObserver
// (要素サイズ変化。画面回転など)の 3 つ。値が変化しないときは前回の state
// オブジェクトをそのまま返して setState をスキップし、スクロール中に毎フレーム
// 再レンダーが走るのを防ぐ。
//
// 注意: 監視しているのは要素のサイズ・スクロール位置のみで、中身のテキスト
// (props.children)の変更そのものはこのフックでは検知しない。テキスト変更時の
// 再計測は ScrollableText が内部コンポーネントを key で作り直すことで担保して
// いる(呼び出し側がこのフックを直接使う場合は、同様に要素を作り直すこと)。
import { useLayoutEffect, useState, type RefObject } from 'react';
import { overflowEdges, type OverflowEdges } from '@/utils/overflowEdges';

const INITIAL_EDGES: OverflowEdges = { start: false, end: false };

export function useHorizontalOverflow(ref: RefObject<HTMLElement | null>): OverflowEdges {
  const [edges, setEdges] = useState<OverflowEdges>(INITIAL_EDGES);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;

    const measure = () => {
      const next = overflowEdges({
        scrollLeft: el.scrollLeft,
        scrollWidth: el.scrollWidth,
        clientWidth: el.clientWidth,
      });
      // 値が変わらないときは前の state オブジェクトを返し、React に再レンダーを
      // スキップさせる(setState の引数が現在の state と同一参照なら bail-out される)。
      setEdges((prev) => (prev.start === next.start && prev.end === next.end ? prev : next));
    };

    measure();
    el.addEventListener('scroll', measure);
    const observer = new ResizeObserver(measure);
    observer.observe(el);

    return () => {
      el.removeEventListener('scroll', measure);
      observer.disconnect();
    };
  }, [ref]);

  return edges;
}
