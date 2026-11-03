import { describe, expect, it } from "vitest";
import type { ChapterAnchor } from "./readerStreamLayout";
import { nextScrollTop, pickGrowth } from "./useStreamAnchor";

function anchors(entries: [number, number][]): Map<number, ChapterAnchor> {
  return new Map(entries.map(([id, offset]) => [id, { offset, height: 1000 }]));
}

describe("连续滚动跨章的视口补偿", () => {
  it("换章提交按「提交前位置 + 位移差」算，不累加到被钳过的当前值上", () => {
    // 读者滚到 1500（在 99 章内）；换章时上面的 98 章被移出窗口，99 的 offset 从 1000 变 0
    const before = anchors([[98, 0], [99, 1000]]);
    const after = anchors([[99, 0]]);
    const growth = pickGrowth(before, after, 99, 99);
    expect(growth).toBe(-1000);
    // 浏览器已按变矮后的高度把 scrollTop 钳到 0：在当前值上累加只会得到 0（少补一段）；
    // 按提交前的 1500 算才是读者该停的位置
    expect(nextScrollTop({ growth, base: 1500, currentTop: 0 })).toBe(500);
  });

  it("新章提交前还没有位置：退回上一个可见章的位移差", () => {
    // 换到 500 章时它刚进窗口；499 章还在窗口里，它上方的增减就是读者眼前的位移
    const before = anchors([[499, 1000]]);
    const after = anchors([[499, 0], [500, 2000]]);
    expect(pickGrowth(before, after, 500, 499)).toBe(-1000);
  });

  it("可见章自己的位移优先于上一个可见章", () => {
    const before = anchors([[499, 1000], [500, 2000]]);
    const after = anchors([[500, 500]]);
    expect(pickGrowth(before, after, 500, 499)).toBe(-1500);
  });

  it("不需要补偿时返回 null：位移不足半像素，或两章都没有旧位置", () => {
    expect(nextScrollTop({ growth: 0.2, base: 100, currentTop: 100 })).toBeNull();
    expect(nextScrollTop({ growth: null, base: 100, currentTop: 100 })).toBeNull();
    expect(pickGrowth(anchors([[99, 0]]), anchors([[100, 0]]), 100, 99)).toBeNull();
  });

  it("非换章提交（没有提交前位置）在当前值上累加，且不会为负", () => {
    expect(nextScrollTop({ growth: 120, base: null, currentTop: 300 })).toBe(420);
    expect(nextScrollTop({ growth: -500, base: null, currentTop: 300 })).toBe(0);
  });
});
