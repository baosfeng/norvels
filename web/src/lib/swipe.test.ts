import { describe, expect, it } from "vitest";
import {
  followLimit,
  followOffset,
  isIgnorableSwipeTarget,
  lockSwipe,
  resolveSwipe,
  SWIPE_ANGLE_RATIO,
  SWIPE_FOLLOW_MAX,
  SWIPE_FOLLOW_MIN,
  SWIPE_FOLLOW_DAMPING,
  SWIPE_LOCK_SLOP,
  SWIPE_SETTLE_MS,
  SWIPE_THRESHOLD,
} from "./swipe";

describe("resolveSwipe 手势判定", () => {
  it("左滑足够远 = 下一章，右滑足够远 = 上一章", () => {
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - SWIPE_THRESHOLD, y: 400 })).toBe("next");
    expect(resolveSwipe({ x: 100, y: 400 }, { x: 100 + SWIPE_THRESHOLD, y: 400 })).toBe("prev");
  });

  it("位移不足阈值不翻页（轻微误触）", () => {
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - (SWIPE_THRESHOLD - 1), y: 400 })).toBeNull();
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - 10, y: 400 })).toBeNull();
  });

  it("斜向滑动以纵向为主时视为滚动，不翻页", () => {
    const dx = SWIPE_THRESHOLD * 1.1;
    const dy = dx; // 45 度，明显是滚动
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - dx, y: 400 + dy })).toBeNull();
  });

  it("横向位移远超纵向时才翻页", () => {
    const dx = SWIPE_THRESHOLD * 1.5;
    const dy = (dx / SWIPE_ANGLE_RATIO) * 0.9;
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - dx, y: 400 + dy })).toBe("next");
  });

  /**
   * 防复发：滚正文时手指横向漂移 40–70px 很常见，旧阈值 1.2（≈40°）把这种滚动判成翻章。
   * 实测 33° 的斜向滑动（dx=70 / dy=45）翻到了下一章，读者「明明在往下滚」。
   */
  it("滚动时的横向漂移不翻章：dx=70 / dy=45（33°）→ null", () => {
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 230, y: 445 })).toBeNull();
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 230, y: 355 })).toBeNull();
  });

  it("明显横向才翻章：dx=100 / dy=40（22°）→ next", () => {
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 200, y: 440 })).toBe("next");
    expect(resolveSwipe({ x: 200, y: 400 }, { x: 300, y: 440 })).toBe("prev");
  });

  it("角度门槛是 2.0（≈27°）：27° 以外整段判为滚动", () => {
    expect(SWIPE_ANGLE_RATIO).toBe(2);
    // 45° / 30° 都不算横向；只有 ≤26.6° 才接管
    expect(resolveSwipe({ x: 300, y: 0 }, { x: 200, y: 100 })).toBeNull();
    expect(resolveSwipe({ x: 300, y: 0 }, { x: 200, y: 58 })).toBeNull();
    expect(resolveSwipe({ x: 300, y: 0 }, { x: 200, y: 49 })).toBe("next");
  });

  it("竖向滚动再远也不翻页", () => {
    expect(resolveSwipe({ x: 200, y: 100 }, { x: 200, y: 500 })).toBeNull();
  });
});

describe("lockSwipe 方向锁定", () => {
  it("位移太小时还没表态", () => {
    expect(lockSwipe({ x: 300, y: 400 }, { x: 304, y: 402 })).toBe("undecided");
    expect(lockSwipe({ x: 300, y: 400 }, { x: 300 + SWIPE_LOCK_SLOP - 1, y: 400 })).toBe("undecided");
  });

  it("明显横向锁定为翻页；斜向漂移锁定为滚动", () => {
    expect(lockSwipe({ x: 300, y: 400 }, { x: 270, y: 402 })).toBe("horizontal");
    expect(lockSwipe({ x: 300, y: 400 }, { x: 270, y: 445 })).toBe("vertical");
  });

  it("先竖后横不改判：一旦按滚动走，整段都不翻章", () => {
    const from = { x: 300, y: 400 };
    expect(lockSwipe(from, { x: 302, y: 430 })).toBe("vertical");
    expect(lockSwipe(from, { x: 200, y: 430 })).toBe("horizontal");
  });
});

describe("followOffset 跟手位移", () => {
  it("跟着手指走但更慢（阻尼），方向与滑动一致", () => {
    expect(followOffset(-40)).toBeCloseTo(-40 * SWIPE_FOLLOW_DAMPING, 5);
    expect(followOffset(-40)).toBeLessThan(0);
    expect(followOffset(40)).toBeGreaterThan(0);
  });

  it("封顶，不把正文拖出屏外", () => {
    expect(followOffset(-400)).toBe(-SWIPE_FOLLOW_MAX);
    expect(followOffset(400)).toBe(SWIPE_FOLLOW_MAX);
    expect(Math.abs(followOffset(-400))).toBeLessThanOrEqual(SWIPE_FOLLOW_MAX);
  });

  it("上限按屏宽取：手机上约 1/3 屏，夹在 72–120 之间", () => {
    expect(followLimit(393)).toBeCloseTo(117.9, 5);
    expect(followOffset(-600, followLimit(393))).toBeCloseTo(-117.9, 5);
    // 窄屏不缩到抓不住，宽屏不放大到甩出屏外
    expect(followLimit(200)).toBe(SWIPE_FOLLOW_MIN);
    expect(followLimit(1440)).toBe(SWIPE_FOLLOW_MAX);
    expect(followLimit(Number.NaN)).toBe(SWIPE_FOLLOW_MAX);
  });

  it("非法位移一律 0（NaN / Infinity 不写进样式）", () => {
    expect(followOffset(Number.NaN)).toBe(0);
    expect(followOffset(Number.POSITIVE_INFINITY)).toBe(0);
    expect(followOffset(0)).toBe(0);
  });

  it("弹回过渡在 180–220ms 之间", () => {
    expect(SWIPE_SETTLE_MS).toBeGreaterThanOrEqual(180);
    expect(SWIPE_SETTLE_MS).toBeLessThanOrEqual(220);
  });
});

describe("isIgnorableSwipeTarget 忽略区域", () => {
  it("按钮/链接/输入框与可编辑区域的起点不接管", () => {
    const wrap = document.createElement("div");
    wrap.innerHTML = '<button id="b">b</button><a id="a" href="/x">a</a><input id="i" /><div id="e" contenteditable="true"></div>';
    for (const id of ["b", "a", "i", "e"]) {
      expect(isIgnorableSwipeTarget(wrap.querySelector(`#${id}`))).toBe(true);
    }
  });

  it("正文段落与普通容器接管，非元素目标不接管", () => {
    const p = document.createElement("p");
    expect(isIgnorableSwipeTarget(p)).toBe(false);
    expect(isIgnorableSwipeTarget(null)).toBe(false);
  });

  it("已选中文字时不接管（避免把选词当翻页）", () => {
    const p = document.createElement("p");
    p.textContent = "一段正文";
    document.body.appendChild(p);
    const range = document.createRange();
    range.selectNodeContents(p);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(isIgnorableSwipeTarget(p)).toBe(true);
    selection?.removeAllRanges();
    document.body.removeChild(p);
  });
});
