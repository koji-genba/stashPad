import { useState } from 'react';

interface Props {
  src: string;
  /** 装飾画像なら省略可。スクリーンリーダ向けに意味のある画像なら指定する。 */
  alt?: string;
  className?: string;
  loading?: 'lazy' | 'eager';
}

/**
 * 共通サムネイル <img>。読み込み失敗時に visibility:hidden で隠すロジックを内包する。
 *
 * 旧実装は各 callsite で onError ハンドラから e.currentTarget.style.visibility を
 * 直接書き換えていたが、それだと React の再レンダリングで同じ要素が別 work に
 * 使い回されたとき "壊れた状態" が残り続ける危険があった。ここでは broken を
 * state として保持し、src の変化で確実にリセットする。
 */
export default function Thumbnail({ src, alt = '', className, loading }: Props) {
  const [broken, setBroken] = useState(false);
  // src が変わったら broken をリセット(React 公式の "props で state をリセット" パターン。
  // useEffect で同期するより 1 レンダリング早く反映でき、effect 不要)
  const [prevSrc, setPrevSrc] = useState(src);
  if (src !== prevSrc) {
    setPrevSrc(src);
    setBroken(false);
  }

  return (
    <img
      src={src}
      alt={alt}
      className={className}
      loading={loading}
      style={broken ? { visibility: 'hidden' } : undefined}
      onError={() => setBroken(true)}
    />
  );
}
