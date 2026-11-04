import { describe, expect, it } from "vitest";
import {
  COMMIT_RATIO,
  dragProgress,
  lockDrag,
  rubberOffset,
  settleDuration,
  shouldCommit,
  turnDirection,
} from "./turnGesture";

describe("翻页手势：方向锁", () => {
  it("横向位移压过纵向才归翻页；竖向优先（读者多半在滚正文）", () => {
    expect(lockDrag(20, 5)).toBe("horizontal");
    expect(lockDrag(5, 20)).toBe("vertical");
    expect(lockDrag(3, 3)).toBe("undecided");
    // 斜着滚：纵向更大 → 归滚动，不翻页
    expect(lockDrag(20, 30)).toBe("vertical");
    // 正好 45° 归纵向，避免抖动翻页
    expect(lockDrag(10, 10)).toBe("vertical");
  });

  it("左滑是下一页，右滑是上一页", () => {
    expect(turnDirection(-40)).toBe(1);
    expect(turnDirection(40)).toBe(-1);
  });
});

describe("翻页手势：进度与提交", () => {
  it("p 按可视宽归一（不是版心宽），上限 1.25", () => {
    expect(dragProgress(-195, 390)).toBeCloseTo(0.5);
    expect(dragProgress(-1000, 390)).toBe(1.25);
    expect(dragProgress(-50, 0)).toBe(0);
  });

  it("位移过 22% 就翻页", () => {
    expect(shouldCommit(COMMIT_RATIO + 0.01, 0, -100)).toBe(true);
    expect(shouldCommit(0.1, 0, -40)).toBe(false);
  });

  it("甩得够快且方向一致也翻页；反向回甩不算", () => {
    expect(shouldCommit(0.05, -1.2, -40)).toBe(true);
    expect(shouldCommit(0.05, 1.2, -40)).toBe(false);
    // 速度不够（低于 0.45 px/ms）
    expect(shouldCommit(0.05, -0.3, -40)).toBe(false);
  });

  it("收尾时长：回弹按剩余距离等比缩短（×0.75），提交走满全程", () => {
    expect(settleDuration(260, 0.3, 1)).toBe(182);
    expect(settleDuration(260, 0.3, 0)).toBe(59);
    expect(settleDuration(0, 0.3, 1)).toBe(0);
  });

  it("越界阻尼：位移随拉动量饱和，永远不会超过 18% 屏宽", () => {
    const max = 0.18 * 390 * 0.5;
    expect(Math.abs(rubberOffset(-200, 390))).toBeLessThanOrEqual(max);
    expect(Math.abs(rubberOffset(-2000, 390))).toBeCloseTo(max, 5);
    // 方向跟着手指：右滑给正值（上一页方向）
    expect(rubberOffset(100, 390)).toBeGreaterThan(0);
    expect(rubberOffset(0, 390)).toBe(0);
  });
});
