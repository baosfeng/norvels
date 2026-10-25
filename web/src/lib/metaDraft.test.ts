import { describe, expect, it } from "vitest";
import type { BookDetail } from "../api/types";
import { authorsOf, buildPatch, changedCount, draftOf, parseAuthors, parseSeriesIndex, parseTags } from "./metaDraft";

const book: BookDetail = {
  id: 3,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者", "另一位"],
  chapterCount: 2,
  wordCount: 100,
  volumeCount: 1,
  intro: "简介。",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  series: "示例系列",
  seriesIndex: 2,
  tags: ["武侠"],
  serialStatus: "ongoing",
  volumes: [],
};

describe("元信息草稿", () => {
  it("作者兜底：authors 优先，缺失时退回主作者", () => {
    expect(authorsOf(book)).toEqual(["示例作者", "另一位"]);
    expect(authorsOf({ ...book, authors: [], authorName: "只一个" })).toEqual(["只一个"]);
  });

  it("初值把多作者与标签拼成一行，序号为空时不显示 0", () => {
    const draft = draftOf({ ...book, seriesIndex: 0 });
    expect(draft.authors).toBe("示例作者，另一位");
    expect(draft.tags).toBe("武侠");
    expect(draft.seriesIndex).toBe("");
    expect(draft.serialStatus).toBe("ongoing");
  });

  it("解析：作者按多种分隔符拆分并去重，标签同理", () => {
    expect(parseAuthors("甲、乙, 甲；丙")).toEqual(["甲", "乙", "丙"]);
    expect(parseTags("武侠, 武侠\n群像")).toEqual(["武侠", "群像"]);
    expect(parseSeriesIndex("")).toBe(0);
    expect(parseSeriesIndex(" 3 ")).toBe(3);
    expect(Number.isNaN(parseSeriesIndex("abc"))).toBe(true);
    expect(Number.isNaN(parseSeriesIndex("-1"))).toBe(true);
  });

  it("只提交改过的字段：空串与 0 照发（它们就是清空）", () => {
    const origin = draftOf(book);
    expect(buildPatch(origin, origin)).toEqual({});
    const cleared = buildPatch({ ...origin, series: "", seriesIndex: "", tags: "", serialStatus: "" }, origin);
    expect(cleared).toEqual({ series: "", seriesIndex: 0, tags: [], serialStatus: "" });
  });

  it("脏标记数字段，不数字符", () => {
    const origin = draftOf(book);
    expect(changedCount(origin, origin)).toBe(0);
    expect(changedCount({ ...origin, title: "新书名", series: "新系列" }, origin)).toBe(2);
  });
});
