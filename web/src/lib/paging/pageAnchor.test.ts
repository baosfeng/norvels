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

  it("取「从本页起第一个起始于本页或之后的段落」——跨页长段不会把读者钉在段首", () => {
    expect(firstParaOnPage(boxes, 0, 354)).toBe(0);
    expect(firstParaOnPage(boxes, 1, 354)).toBe(1);
    expect(firstParaOnPage(boxes, 2, 354)).toBe(2);
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
