import { Link } from 'react-router-dom';
import Thumbnail from './Thumbnail';
import styles from './WorkCard.module.css';

interface Props {
  id: number;
  title: string;
  circle?: string | null;
  ageRating?: string | null;
  thumbnailUrl?: string | null;
  hasFolder?: boolean;
  /** 右下に出す補助テキスト(履歴の再生回数など) */
  badge?: string;
  /** お気に入り登録済みなら右上に ★ バッジを出す */
  favorited?: boolean;
  /** 1〜5 の評価。未評価は null/undefined */
  rating?: number | null;
}

export default function WorkCard({
  id,
  title,
  circle,
  ageRating,
  thumbnailUrl,
  hasFolder = true,
  badge,
  favorited,
  rating,
}: Props) {
  return (
    <Link to={`/works/${id}`} className={styles.card}>
      <div className={styles.thumbWrap}>
        <Thumbnail className={styles.thumb} src={thumbnailUrl} loading="lazy" />
        {!hasFolder && <span className={styles.notImported}>未取込</span>}
        {favorited && (
          <span className={styles.favoriteBadge} aria-label="お気に入り">
            ★
          </span>
        )}
        {rating && (
          <span className={styles.ratingBadge} aria-label={`評価 ${rating}`}>
            {'★'.repeat(rating)}
          </span>
        )}
        {ageRating && <span className={styles.age}>{ageRating}</span>}
        {badge && <span className={styles.badge}>{badge}</span>}
      </div>
      <div className={styles.info}>
        <div className={styles.title} title={title}>
          {title}
        </div>
        {circle && <div className={styles.circle}>{circle}</div>}
      </div>
    </Link>
  );
}
