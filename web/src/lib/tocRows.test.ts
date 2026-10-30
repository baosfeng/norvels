import { describe, expect, it } from "vitest";
import type { ChapterBrief } from "../api/types";
import { buildTocRows, columnsForWidth, filterChapters, rowIndexOfChapter, shouldAutoReveal, TOC_AUTO_LOAD_GAP } from "./tocRows";

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

describe("滚到底自动续载（task-2）", () => {
  const view = (scrollTop: number, clientHeight: number, scrollHeight: number) => ({
    scrollTop,
    clientHeight,
    scrollHeight,
  });

  it("距底部在余量内就补下一批（不必真滚到底）", () => {
    // 694 可视 + 240 余量：scrollTop ≥ 2780 - 694 - 240 = 1846
    expect(shouldAutoReveal(view(1846, 694, 2780), 60, 150)).toBe(true);
    expect(shouldAutoReveal(view(1845, 694, 2780), 60, 150)).toBe(false);
  });

  it("已全部显示 / 没有章节时不补", () => {
    expect(shouldAutoReveal(view(9999, 694, 2780), 150, 150)).toBe(false);
    expect(shouldAutoReveal(view(0, 694, 2780), 0, 0)).toBe(false);
  });

  it("还没排版（scrollHeight 为 0）时不补，交给滚动事件", () => {
    expect(shouldAutoReveal(view(0, 0, 0), 60, 150)).toBe(false);
  });

  it("一屏都装不满时直接补下一批，避免按钮卡在屏幕外", () => {
    expect(shouldAutoReveal(view(0, 600, 500), 60, 150)).toBe(true);
  });

  it("余量可覆盖，负余量按 0 处理", () => {
    expect(TOC_AUTO_LOAD_GAP).toBe(240);
    expect(shouldAutoReveal(view(2186, 694, 2880), 60, 150, 0)).toBe(true);
    expect(shouldAutoReveal(view(2185, 694, 2880), 60, 150, 0)).toBe(false);
    expect(shouldAutoReveal(view(2780, 694, 2880), 60, 150, -100)).toBe(true);
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
