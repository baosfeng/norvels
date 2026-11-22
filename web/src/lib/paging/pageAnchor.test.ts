import { describe, expect, it } from "vitest";
import { firstParaOnPage, normalizeParaIndex, pageOfParagraph } from "./pageAnchor";

const boxes = [
  { pi: 0, left: 0 },
  { pi: 1, left: 380 },
  { pi: 2, left: 760 },
  { pi: 3, left: 1500 },
];

describe("段落锚点：段号 ↔ 页码", () => {
  it("段落起点落在第几列，那一列就是它的页", () => {
    expect(pageOfParagraph(boxes, 0, 354)).toBe(0);
    expect(pageOfParagraph(boxes, 1, 354)).toBe(1);
    expect(pageOfParagraph(boxes, 3, 354)).toBe(4);
  });

  it("段号不存在时返回 null，调用方回落到原页码", () => {
    expect(pageOfParagraph(boxes, 42, 354)).toBeNull();
    expect(pageOfParagraph(boxes, 0, 0)).toBe(0);
  });

  it("取「起点不晚于页首」的最后一个段落——页首那几行属于谁就锚在谁身上", () => {
    // 页 0 页首 left=0：段 0 正好从页首开始
    expect(firstParaOnPage(boxes, 0, 354)).toBe(0);
    // 页 1 页首 left=354：段 0 从 0 开始、段 1 从 380 开始（页首之后）→ 锚段 0
    expect(firstParaOnPage(boxes, 1, 354)).toBe(0);
    // 页 2 页首 left=708：段 1 从 380 开始 → 锚段 1
    expect(firstParaOnPage(boxes, 2, 354)).toBe(1);
  });

  /**
   * 防复发：实测 20→28 字号，读者在第 3 页（页首是段 9 的续列，页尾才开始段 15），
   * 锚点若记成页尾段，重排后读者被送到段 15 所在页、整页内容全变。
   */
  it("同列内页中部的段不能当页首锚点（left 相同，要靠 top 区分）", () => {
    const boxes2 = [
      { pi: 9, left: 708, top: 0 }, // 页 2 列首开始
      { pi: 13, left: 708, top: 260 }, // 同列中部
      { pi: 15, left: 708, top: 520 }, // 同列靠后
    ];
    expect(firstParaOnPage(boxes2, 2, 354)).toBe(9);
  });

  it("页首全是上一段的后半（本页没有列首段）时，取起始页 ≤ 本页的最后一段", () => {
    const boxes3 = [
      { pi: 7, left: 354, top: 0 }, // 从页 1 列首开始，跨页到页 2
      { pi: 11, left: 1062, top: 0 }, // 页 3 才开始
    ];
    expect(firstParaOnPage(boxes3, 2, 354)).toBe(7);
  });

  it("末页没有新段起点时回落到最后一段（末页常是长段的后半截）", () => {
    expect(firstParaOnPage(boxes, 9, 354)).toBe(3);
    expect(firstParaOnPage([], 0, 354)).toBeNull();
  });

  it("脏段号（后端 / 老数据）一律归一到非负整数", () => {
    expect(normalizeParaIndex("7")).toBe(7);
    expect(normalizeParaIndex(-2)).toBe(0);
    expect(normalizeParaIndex(null)).toBe(0);
    expect(normalizeParaIndex(3.9)).toBe(3);
    expect(normalizeParaIndex(Number.NaN, 5)).toBe(5);
  });
});
