// 長いファイル名/フォルダ名を読めるようにする部品。
// ellipsis で切り詰めると元のテキストを知る手段がなくなってしまうスマホ幅では、
// 横スクロール + はみ出している側だけのフェードの方が「隠れた続きがある」と
// 分かった上でスワイプして読めるため実用的。縦幅を取らないようスクロールバーは隠す。
import { useRef, type ReactNode } from 'react';
import { useHorizontalOverflow } from '@/hooks/useHorizontalOverflow';
import styles from './ScrollableText.module.css';

interface Props {
  children: ReactNode;
  className?: string;
  title?: string;
}

export default function ScrollableText({ children, className, title }: Props) {
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
