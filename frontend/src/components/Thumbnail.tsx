import { useState } from 'react';

interface Props {
  /**
   * サムネ URL。API は thumbnail_url をサムネ無しの場合 `null` で返す(キー省略はしない、
   * issue #57)。呼び出し側がそのまま素通しできるよう null/undefined を受け付ける。
   */
  src?: string | null;
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
 *
 * src が null/undefined(サムネ無し)の場合は img に src 属性自体を付けない
 * (空文字だと現在のページ URL への無駄なリクエストが発生する)。そのうえで
 * 最初から broken=true として扱い、既存の「読み込み失敗時に隠す」プレースホルダ表示を
 * そのまま再利用する。
 */
export default function Thumbnail({ src, alt = '', className, loading }: Props) {
  const [broken, setBroken] = useState(!src);
  // src が変わったら broken をリセット(React 公式の "props で state をリセット" パターン。
  // useEffect で同期するより 1 レンダリング早く反映でき、effect 不要)
  const [prevSrc, setPrevSrc] = useState(src);
  if (src !== prevSrc) {
    setPrevSrc(src);
    setBroken(!src);
  }

  return (
    <img
      src={src ?? undefined}
      alt={alt}
      className={className}
      loading={loading}
      style={broken ? { visibility: 'hidden' } : undefined}
      onError={() => setBroken(true)}
    />
  );
}
