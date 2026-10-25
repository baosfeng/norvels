import { describe, expect, it } from "vitest";
import type { ChapterBrief } from "../api/types";
import { buildTocRows, columnsForWidth, filterChapters, rowIndexOfChapter } from "./tocRows";

function chapters(n: number): ChapterBrief[] {
  return Array.from({ length: n }, (_, i) => ({
    id: 100 + i,
    idx: i,
    title: `第${i + 1}章 标题${i + 1}`,
    volumeId: 1,
    wordCount: 1000 + i,
  }));
}

describe("目录视觉行", () => {
  it("按列数切行：3 列时一行 3 章，最后一行可以不满", () => {
    const rows = buildTocRows(chapters(7), 3);
    expect(rows).toHaveLength(3);
    expect(rows[0].items.map((c) => c.idx)).toEqual([0, 1, 2]);
    expect(rows[2].items.map((c) => c.idx)).toEqual([6]);
  });

  it("列数非法时按 1 列处理", () => {
    expect(buildTocRows(chapters(3), 0)).toHaveLength(3);
    expect(buildTocRows(chapters(3), Number.NaN)).toHaveLength(3);
  });

  it("空目录不产生行", () => {
    expect(buildTocRows([], 3)).toEqual([]);
  });

  it("当前章所在行按列数折算", () => {
    const list = chapters(7);
    expect(rowIndexOfChapter(list, 1, 103)).toBe(3);
    expect(rowIndexOfChapter(list, 3, 103)).toBe(1);
    expect(rowIndexOfChapter(list, 3, 106)).toBe(2);
  });

  it("没有当前章或找不到时返回 -1", () => {
    expect(rowIndexOfChapter(chapters(3), 3, undefined)).toBe(-1);
    expect(rowIndexOfChapter(chapters(3), 3, 999)).toBe(-1);
  });

  it("列数断点与 CSS 一致：≥1180 三列、≥900 两列、其余一列", () => {
    expect(columnsForWidth(1440)).toBe(3);
    expect(columnsForWidth(1180)).toBe(3);
    expect(columnsForWidth(1100)).toBe(2);
    expect(columnsForWidth(900)).toBe(2);
    expect(columnsForWidth(899)).toBe(1);
    expect(columnsForWidth(390)).toBe(1);
  });
});

describe("目录搜索", () => {
  it("空关键词返回全部", () => {
    expect(filterChapters(chapters(5), "  ")).toHaveLength(5);
  });

  it("按标题匹配（大小写不敏感）", () => {
    const list = [
      { id: 1, idx: 0, title: "第一章 Alpha", volumeId: 1, wordCount: 1 },
      { id: 2, idx: 1, title: "第二章 Beta", volumeId: 1, wordCount: 1 },
    ];
    expect(filterChapters(list, "beta").map((c) => c.id)).toEqual([2]);
  });

  it("按章号匹配", () => {
    expect(filterChapters(chapters(12), "12").map((c) => c.idx + 1)).toEqual([12]);
  });

  it("没命中返回空数组", () => {
    expect(filterChapters(chapters(3), "不存在的标题")).toEqual([]);
  });
});
