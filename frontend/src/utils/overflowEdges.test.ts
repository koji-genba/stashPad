// overflowEdges の純関数テスト。
//
// テストリスト:
// - 収まっている(scrollWidth <= clientWidth)ときは start/end とも false
// - 先頭位置(scrollLeft=0)で右にはみ出すときは end のみ true
// - 途中位置(左右どちらにもはみ出しが残る)では start/end とも true
// - 末尾位置(右端までスクロール済み)では start のみ true
// - 誤差 1px 以内のはみ出しは start・end どちらも「はみ出し扱いしない」
import { describe, expect, it } from 'vitest';
import { overflowEdges } from './overflowEdges';

describe('overflowEdges', () => {
  it('収まっているときは start も end も false になる', () => {
    const edges = overflowEdges({ scrollLeft: 0, scrollWidth: 100, clientWidth: 100 });
    expect(edges).toEqual({ start: false, end: false });
  });

  it('先頭位置で右にはみ出すときは end のみ true になる', () => {
    const edges = overflowEdges({ scrollLeft: 0, scrollWidth: 300, clientWidth: 100 });
    expect(edges).toEqual({ start: false, end: true });
  });

  it('途中位置では start も end も true になる', () => {
    const edges = overflowEdges({ scrollLeft: 100, scrollWidth: 300, clientWidth: 100 });
    expect(edges).toEqual({ start: true, end: true });
  });

  it('末尾位置では start のみ true になる', () => {
    // scrollWidth(300) - clientWidth(100) = 200 = 最大 scrollLeft
    const edges = overflowEdges({ scrollLeft: 200, scrollWidth: 300, clientWidth: 100 });
    expect(edges).toEqual({ start: true, end: false });
  });

  it('1px 以内の誤差は start・end どちらもはみ出し扱いしない', () => {
    // scrollLeft が 1px だけ残っている(サブピクセル丸め) → start は false
    const nearStart = overflowEdges({ scrollLeft: 1, scrollWidth: 300, clientWidth: 200 });
    expect(nearStart.start).toBe(false);

    // 右側の隠れ幅が 1px だけ残っている → end は false
    // scrollWidth(300) - clientWidth(200) - scrollLeft(99) = 1
    const nearEnd = overflowEdges({ scrollLeft: 99, scrollWidth: 300, clientWidth: 200 });
    expect(nearEnd.end).toBe(false);
  });
});
