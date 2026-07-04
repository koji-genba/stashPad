// fetch 失敗時のエラーメッセージ + 再試行ボタンの共通表示(issue #70)。
// 既存のエラー表示(global.css の .error / .muted)の見た目を踏襲しつつ、
// リトライ導線(onRetry)を付ける。呼び出し側は retryNonce の increment 等で
// データ取得 effect を再実行する。
import styles from './FetchError.module.css';

interface Props {
  /** 表示するエラーメッセージ */
  message: string;
  /** 再試行ボタン押下時に呼ばれる(データ取得の再実行) */
  onRetry: () => void;
}

export default function FetchError({ message, onRetry }: Props) {
  return (
    <div className={styles.wrap}>
      <p className="error">{message}</p>
      <button type="button" className="btn" onClick={onRetry}>
        再試行
      </button>
    </div>
  );
}
