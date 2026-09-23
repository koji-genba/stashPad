// 長いファイル名/曲名/作品名を読めるようにする部品。
// ellipsis で切り詰めると元のテキストを知る手段がなくなってしまうスマホ幅では、
// 横スクロール + はみ出している側だけのフェードの方が「隠れた続きがある」と
// 分かった上でスワイプして読めるため実用的。縦幅を取らないようスクロールバーは隠す。
import { useRef } from 'react';
import { useHorizontalOverflow } from '@/hooks/useHorizontalOverflow';
import styles from './ScrollableText.module.css';

interface Props {
  children: string;
  className?: string;
  title?: string;
}

// プレイヤー等ではマウントされたまま曲名だけが変わるため、内部で作り直さないと
// はみ出し状態の再計測もスクロール位置のリセットも起きない。呼び出し側に
// 「key を付ける」ことを求めると付け忘れの元になるので、children(表示テキスト)
// を key にして ScrollableTextBody ごと作り直すことでここで吸収する。
export default function ScrollableText(props: Props) {
  return <ScrollableTextBody key={props.children} {...props} />;
}

function ScrollableTextBody({ children, className, title }: Props) {
  const ref = useRef<HTMLSpanElement>(null);
  const { start, end } = useHorizontalOverflow(ref);

  return (
    <span
      ref={ref}
      className={className ? `${styles.scroller} ${className}` : styles.scroller}
      title={title}
      data-overflow-start={start || undefined}
      data-overflow-end={end || undefined}
    >
      {children}
    </span>
  );
}
