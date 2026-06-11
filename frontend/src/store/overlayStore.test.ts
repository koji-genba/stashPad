// overlayStore の状態遷移テスト。
// 各テスト前にストアを初期状態にリセットする。
import { beforeEach, describe, expect, it } from 'vitest';
import { useOverlayStore } from './overlayStore';
import type { Entry } from '@/api/types';

function resetStore() {
  // replace=false でマージ: メソッドを保持しつつデータプロパティのみリセット
  useOverlayStore.setState({ image: null, video: null, text: null }, false);
}

function makeEntry(name: string, media_kind: Entry['media_kind'] = 'image', is_dir = false): Entry {
  return { name, is_dir, size: 0, media_kind };
}

// ---- image overlay ----
describe('overlayStore 初期状態', () => {
  beforeEach(resetStore);

  it('初期値がすべて null', () => {
    const s = useOverlayStore.getState();
    expect(s.image).toBeNull();
    expect(s.video).toBeNull();
    expect(s.text).toBeNull();
  });
});

describe('openImage', () => {
  beforeEach(resetStore);

  const entries: Entry[] = [
    makeEntry('page1.jpg', 'image'),
    makeEntry('page2.jpg', 'image'),
    makeEntry('page3.jpg', 'image'),
    makeEntry('audio.mp3', 'audio'), // image 以外はページに含まれない
    makeEntry('subdir', 'image', true), // is_dir=true はページに含まれない
  ];

  it('startName が見つかると image が設定される', () => {
    useOverlayStore.getState().openImage({
      workId: 10,
      dir: '',
      entries,
      startName: 'page2.jpg',
    });
    const { image } = useOverlayStore.getState();
    expect(image).not.toBeNull();
    expect(image!.workId).toBe(10);
    expect(image!.index).toBe(1); // page2.jpg はページリストの 2 番目
    expect(image!.pages).toHaveLength(3); // audio.mp3 と subdir は除外
    expect(image!.pages[0].name).toBe('page1.jpg');
    expect(image!.pages[1].name).toBe('page2.jpg');
  });

  it('dir が指定された場合、pages[].path に dir が付く', () => {
    useOverlayStore.getState().openImage({
      workId: 1,
      dir: 'vol1',
      entries: [makeEntry('p1.jpg', 'image')],
      startName: 'p1.jpg',
    });
    const { image } = useOverlayStore.getState();
    expect(image!.pages[0].path).toBe('vol1/p1.jpg');
  });

  it('startName が見つからない場合は何もしない', () => {
    useOverlayStore.getState().openImage({
      workId: 1,
      dir: '',
      entries,
      startName: 'nonexistent.jpg',
    });
    expect(useOverlayStore.getState().image).toBeNull();
  });

  it('image エントリが空の場合は何もしない', () => {
    useOverlayStore.getState().openImage({
      workId: 1,
      dir: '',
      entries: [makeEntry('audio.mp3', 'audio')],
      startName: 'audio.mp3',
    });
    expect(useOverlayStore.getState().image).toBeNull();
  });
});

describe('setImageIndex', () => {
  beforeEach(() => {
    resetStore();
    useOverlayStore.setState({
      image: {
        workId: 1,
        dir: '',
        pages: [
          { name: 'p1.jpg', path: 'p1.jpg' },
          { name: 'p2.jpg', path: 'p2.jpg' },
          { name: 'p3.jpg', path: 'p3.jpg' },
        ],
        index: 0,
      },
    });
  });

  it('有効な index に設定できる', () => {
    useOverlayStore.getState().setImageIndex(2);
    expect(useOverlayStore.getState().image!.index).toBe(2);
  });

  it('負の index は無視される', () => {
    useOverlayStore.getState().setImageIndex(-1);
    expect(useOverlayStore.getState().image!.index).toBe(0);
  });

  it('範囲外の index は無視される', () => {
    useOverlayStore.getState().setImageIndex(10);
    expect(useOverlayStore.getState().image!.index).toBe(0);
  });

  it('image が null のとき何もしない(クラッシュしない)', () => {
    useOverlayStore.setState({ image: null });
    expect(() => useOverlayStore.getState().setImageIndex(0)).not.toThrow();
  });
});

describe('imageNext / imagePrev', () => {
  beforeEach(() => {
    resetStore();
    useOverlayStore.setState({
      image: {
        workId: 1,
        dir: '',
        pages: [
          { name: 'p1.jpg', path: 'p1.jpg' },
          { name: 'p2.jpg', path: 'p2.jpg' },
          { name: 'p3.jpg', path: 'p3.jpg' },
        ],
        index: 1,
      },
    });
  });

  it('imageNext で次のページに進む', () => {
    useOverlayStore.getState().imageNext();
    expect(useOverlayStore.getState().image!.index).toBe(2);
  });

  it('最後のページで imageNext しても進まない', () => {
    useOverlayStore.setState({ image: { ...useOverlayStore.getState().image!, index: 2 } });
    useOverlayStore.getState().imageNext();
    expect(useOverlayStore.getState().image!.index).toBe(2);
  });

  it('imagePrev で前のページに戻る', () => {
    useOverlayStore.getState().imagePrev();
    expect(useOverlayStore.getState().image!.index).toBe(0);
  });

  it('先頭ページで imagePrev しても戻らない', () => {
    useOverlayStore.setState({ image: { ...useOverlayStore.getState().image!, index: 0 } });
    useOverlayStore.getState().imagePrev();
    expect(useOverlayStore.getState().image!.index).toBe(0);
  });

  it('image が null のとき imageNext は何もしない', () => {
    useOverlayStore.setState({ image: null });
    expect(() => useOverlayStore.getState().imageNext()).not.toThrow();
    expect(useOverlayStore.getState().image).toBeNull();
  });

  it('image が null のとき imagePrev は何もしない', () => {
    useOverlayStore.setState({ image: null });
    expect(() => useOverlayStore.getState().imagePrev()).not.toThrow();
    expect(useOverlayStore.getState().image).toBeNull();
  });
});

describe('closeImage', () => {
  beforeEach(() => {
    resetStore();
    useOverlayStore.setState({
      image: {
        workId: 1,
        dir: '',
        pages: [{ name: 'p.jpg', path: 'p.jpg' }],
        index: 0,
      },
    });
  });

  it('closeImage で image が null になる', () => {
    useOverlayStore.getState().closeImage();
    expect(useOverlayStore.getState().image).toBeNull();
  });
});

// ---- video overlay ----
describe('openVideo / closeVideo', () => {
  beforeEach(resetStore);

  const videoState = {
    workId: 5,
    workTitle: '動画作品',
    path: 'movie.mp4',
    name: 'movie.mp4',
  };

  it('openVideo で video が設定される', () => {
    useOverlayStore.getState().openVideo(videoState);
    expect(useOverlayStore.getState().video).toEqual(videoState);
  });

  it('closeVideo で video が null になる', () => {
    useOverlayStore.getState().openVideo(videoState);
    useOverlayStore.getState().closeVideo();
    expect(useOverlayStore.getState().video).toBeNull();
  });

  it('openVideo は image や text に影響しない', () => {
    useOverlayStore.setState({
      image: {
        workId: 1,
        dir: '',
        pages: [{ name: 'p.jpg', path: 'p.jpg' }],
        index: 0,
      },
    });
    useOverlayStore.getState().openVideo(videoState);
    expect(useOverlayStore.getState().image).not.toBeNull();
  });
});

// ---- text overlay ----
describe('openText / closeText', () => {
  beforeEach(resetStore);

  const textState = {
    workId: 7,
    path: 'readme.txt',
    name: 'readme.txt',
  };

  it('openText で text が設定される', () => {
    useOverlayStore.getState().openText(textState);
    expect(useOverlayStore.getState().text).toEqual(textState);
  });

  it('closeText で text が null になる', () => {
    useOverlayStore.getState().openText(textState);
    useOverlayStore.getState().closeText();
    expect(useOverlayStore.getState().text).toBeNull();
  });

  it('openText は video に影響しない', () => {
    const videoState = { workId: 5, workTitle: '動画', path: 'v.mp4', name: 'v.mp4' };
    useOverlayStore.getState().openVideo(videoState);
    useOverlayStore.getState().openText(textState);
    expect(useOverlayStore.getState().video).toEqual(videoState);
    expect(useOverlayStore.getState().text).toEqual(textState);
  });
});
