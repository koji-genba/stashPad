// 画像ビューア / 動画 / テキスト の全画面オーバーレイ状態。
// FileBrowser から起動し、ルート直下の <Overlays> が描画する。
import { create } from 'zustand';
import type { Entry } from '@/api/types';

export interface ImageViewerState {
  workId: number;
  dir: string;
  /** 同一ディレクトリの image を自然順(entries 順)で並べたページ列 */
  pages: { name: string; path: string }[];
  index: number;
}

export interface VideoState {
  workId: number;
  workTitle: string;
  path: string;
  name: string;
}

export interface TextState {
  workId: number;
  path: string;
  name: string;
}

interface OverlayState {
  image: ImageViewerState | null;
  video: VideoState | null;
  text: TextState | null;

  openImage: (args: {
    workId: number;
    dir: string;
    entries: Entry[];
    startName: string;
  }) => void;
  setImageIndex: (index: number) => void;
  imageNext: () => void;
  imagePrev: () => void;
  closeImage: () => void;

  openVideo: (v: VideoState) => void;
  closeVideo: () => void;

  openText: (t: TextState) => void;
  closeText: () => void;
}

function joinPath(dir: string, name: string): string {
  return dir ? `${dir.replace(/\/+$/, '')}/${name}` : name;
}

export const useOverlayStore = create<OverlayState>((set, get) => ({
  image: null,
  video: null,
  text: null,

  openImage: ({ workId, dir, entries, startName }) => {
    const pages = entries
      .filter((e) => !e.is_dir && e.media_kind === 'image')
      .map((e) => ({ name: e.name, path: joinPath(dir, e.name) }));
    const index = pages.findIndex((p) => p.name === startName);
    if (index < 0) return;
    set({ image: { workId, dir, pages, index } });
  },
  setImageIndex: (index) => {
    const { image } = get();
    if (!image) return;
    if (index < 0 || index >= image.pages.length) return;
    set({ image: { ...image, index } });
  },
  imageNext: () => {
    const { image } = get();
    if (image && image.index + 1 < image.pages.length) {
      set({ image: { ...image, index: image.index + 1 } });
    }
  },
  imagePrev: () => {
    const { image } = get();
    if (image && image.index - 1 >= 0) {
      set({ image: { ...image, index: image.index - 1 } });
    }
  },
  closeImage: () => set({ image: null }),

  openVideo: (video) => set({ video }),
  closeVideo: () => set({ video: null }),

  openText: (text) => set({ text }),
  closeText: () => set({ text: null }),
}));
