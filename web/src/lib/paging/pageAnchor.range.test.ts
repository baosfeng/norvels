import { describe, expect, it } from "vitest";
import { paragraphRangeOfPage } from "./pageAnchor";

const boxes = [
  { pi: 0, left: 0 },
  { pi: 1, left: 380 },
  { pi: 2, left: 760 },
  { pi: 3, left: 1500 },
];

describe("段落区间：读屏按页取文字", () => {
  it("页内最后一个已开始的段就是终点（本页至少读到这里）", () => {
    expect(paragraphRangeOfPage(boxes, 0, 354)).toEqual({ start: 0, end: 0 });
    expect(paragraphRangeOfPage(boxes, 1, 354)).toEqual({ start: 0, end: 1 });
    expect(paragraphRangeOfPage(boxes, 2, 354)).toEqual({ start: 1, end: 2 });
  });

  it("长段跨页：本页没有新段起点时，读的仍是那一段（不会退回章首）", () => {
    const sparse = [
      { pi: 0, left: 0 },
      { pi: 1, left: 1500 },
    ];
    expect(paragraphRangeOfPage(sparse, 2, 354)).toEqual({ start: 0, end: 0 });
  });

  it("末页：从最后一个已开始的段读起", () => {
    expect(paragraphRangeOfPage(boxes, 9, 354)).toEqual({ start: 3, end: 3 });
  });

  it("任何一页的区间都不会覆盖全章（读屏不会把整章念一遍）", () => {
    for (let page = 0; page < 10; page++) {
      const range = paragraphRangeOfPage(boxes, page, 354);
      expect(range).not.toBeNull();
      expect(range!.end - range!.start).toBeLessThan(boxes.length);
    }
  });

  it("空章 / 尺寸不可用时返回 null", () => {
    expect(paragraphRangeOfPage([], 0, 354)).toBeNull();
    expect(paragraphRangeOfPage(boxes, 0, 0)).toBeNull();
  });
});
