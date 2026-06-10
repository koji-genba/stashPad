import { NavLink, Route, Routes } from 'react-router-dom';
import { useStore } from 'zustand';
import WorksListPage from './pages/WorksListPage';
import WorkDetailPage from './pages/WorkDetailPage';
import HistoryPage from './pages/HistoryPage';
import SettingsPage from './pages/SettingsPage';
import AudioPlayer from './components/AudioPlayer';
import Overlays from './components/Overlays';
import { usePlayerStore } from './store/playerStore';
import styles from './App.module.css';

function TabBar() {
  const links: { to: string; label: string; icon: string }[] = [
    { to: '/', label: 'ライブラリ', icon: '▦' },
    { to: '/history', label: '履歴', icon: '↺' },
    { to: '/settings', label: '設定', icon: '⚙' },
  ];
  return (
    <nav className={styles.tabbar}>
      {links.map((l) => (
        <NavLink
          key={l.to}
          to={l.to}
          end={l.to === '/'}
          className={({ isActive }) =>
            `${styles.tab} ${isActive ? styles.tabActive : ''}`
          }
        >
          <span className={styles.tabIcon}>{l.icon}</span>
          <span className={styles.tabLabel}>{l.label}</span>
        </NavLink>
      ))}
    </nav>
  );
}

export default function App() {
  // プレイヤーがアクティブなときだけ本文下端にプレイヤー分の余白を足す。
  const hasPlayer = useStore(usePlayerStore, (s) => s.ctx !== null);

  return (
    <div className={styles.app}>
      <main
        className={`${styles.main} ${hasPlayer ? styles.mainWithPlayer : ''}`}
      >
        <Routes>
          <Route path="/" element={<WorksListPage />} />
          <Route path="/works/:id" element={<WorkDetailPage />} />
          <Route path="/history" element={<HistoryPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<WorksListPage />} />
        </Routes>
      </main>

      {/* ルート直下に常駐。ページ遷移しても unmount されない。 */}
      <AudioPlayer />
      <TabBar />
      <Overlays />
    </div>
  );
}
