import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// 本番ビルドは dist/ を Go バイナリに go:embed して単一コンテナで配信する。
// 開発時は Vite dev server を :5173 で起動し、/api を backend(:8080)へ proxy する。
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});
