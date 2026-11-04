import { describe, expect, it } from "vitest";
import {
  clampPage,
  computePageBox,
  countPages,
  formatPageLabel,
  isEmptyBox,
  pageOfOffset,
  pagePercent,
  translateForPage,
} from "./pageGeometry";

describe("分页几何：页宽 / 页高", () => {
  it("页宽 = 可视宽减两侧留白，页高向下取整到行高整数倍", () => {
    const box = computePageBox({ stageW: 390, stageH: 748, padX: 18, lineH: 38 });
    expect(box.pageW).toBe(354);
    // 748 / 38 = 19.68 → 19 行
    expect(box.pageH).toBe(722);
    expect(box.padY).toBe(13);
  });

  it("余量上下均分：0..(视图高-页高) 之间的那截不留给底部一块空白", () => {
    const box = computePageBox({ stageW: 390, stageH: 770, padX: 20, lineH: 40 });
    expect(box.pageH).toBe(760);
    expect(box.padY).toBe(5);
  });

  it("尺寸还没量出来（隐藏 / 0 高）时页宽页高为 0，调用方据此跳过落位", () => {
    const box = computePageBox({ stageW: 0, stageH: 0, padX: 18, lineH: 38 });
    expect(box.pageW).toBe(0);
    expect(isEmptyBox(box)).toBe(true);
    expect(isEmptyBox(computePageBox({ stageW: 390, stageH: 748, padX: 18, lineH: 38 }))).toBe(false);
  });

  it("行高缺失时不强行取整（量不到行高就别把页高算成 0）", () => {
    const box = computePageBox({ stageW: 390, stageH: 748, padX: 18, lineH: 0 });
    expect(box.pageH).toBe(748);
    expect(box.padY).toBe(0);
  });
});

describe("分页几何：页数与页码", () => {
  it("页数 = 排版源 scrollWidth / 页宽（多栏容器的唯一真值）", () => {
    expect(countPages(3540, 354)).toBe(10);
    expect(countPages(3510, 390)).toBe(9);
    // 至少一页：内容没铺满也算一页
    expect(countPages(0, 354)).toBe(1);
    expect(countPages(120, 0)).toBe(1);
  });

  it("页码夹在首末页之间，越界不返回 NaN", () => {
    expect(clampPage(-3, 10)).toBe(0);
    expect(clampPage(99, 10)).toBe(9);
    expect(clampPage(Number.NaN, 10)).toBe(0);
    expect(clampPage(3.4, 10)).toBe(3);
  });

  it("页内进度：首 0、末 1；只有一页时为 0（细条不显示成半截）", () => {
    expect(pagePercent(0, 10)).toBe(0);
    expect(pagePercent(9, 10)).toBe(1);
    expect(pagePercent(4, 9)).toBeCloseTo(0.5);
    expect(pagePercent(0, 1)).toBe(0);
  });

  it("页码文案是「第 p / P 页」，1 起数", () => {
    expect(formatPageLabel(0, 10)).toBe("第 1 / 10 页");
    expect(formatPageLabel(9, 10)).toBe("第 10 / 10 页");
    expect(formatPageLabel(0, 0)).toBe("第 1 / 1 页");
  });

  it("横向偏移 → 列号：整列边界不算到下一列，亚像素误差只吃 1px", () => {
    expect(pageOfOffset(0, 354)).toBe(0);
    expect(pageOfOffset(353, 354)).toBe(0);
    expect(pageOfOffset(354, 354)).toBe(1);
    expect(pageOfOffset(353.6, 354)).toBe(0);
    expect(pageOfOffset(-5, 354)).toBe(0);
  });

  it("第 N 页的位移是负的 N 个页宽", () => {
    expect(translateForPage(0, 354)).toBe(0);
    expect(translateForPage(3, 354)).toBe(-1062);
  });
});
