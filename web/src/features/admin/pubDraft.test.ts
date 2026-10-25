import { describe, expect, it } from "vitest";
import type { BookDetail } from "../../api/types";
import { pubBuildPatch, pubChangedCount, pubDraftOf, toDateInput } from "./pubDraft";

const base: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 2,
  wordCount: 34000,
  volumeCount: 1,
  intro: "",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  tags: ["武侠"],
  serialStatus: "ongoing",
  volumes: [],
};

describe("出版字段草稿：回填、脏值计数与只发改动项", () => {
  it("回填四个字段；后端没给就按空串（存量书不回填）", () => {
    const draft = pubDraftOf({
      ...base,
      publisher: "示例出版社",
      pubDate: "2019-01-02",
      isbn: "978-7-0000-0000-0",
      language: "zh-CN",
    });
    expect(draft.publisher).toBe("示例出版社");
    expect(draft.pubDate).toBe("2019-01-02");
    expect(draft.isbn).toBe("978-7-0000-0000-0");
    expect(draft.language).toBe("zh-CN");

    const empty = pubDraftOf(base);
    expect([empty.publisher, empty.pubDate, empty.isbn, empty.language]).toEqual(["", "", "", ""]);
  });

  it("出版日期带时间戳时截到 YYYY-MM-DD，认不出的值留空", () => {
    expect(toDateInput("2019-01-02T08:00:00Z")).toBe("2019-01-02");
    expect(toDateInput(" 2019-01-02 ")).toBe("2019-01-02");
    expect(toDateInput("2019/01/02")).toBe("");
    expect(toDateInput(undefined)).toBe("");
    expect(pubDraftOf({ ...base, pubDate: "2019-01-02T08:00:00Z" }).pubDate).toBe("2019-01-02");
  });

  it("没改动时 patch 为空、计数为 0", () => {
    const origin = pubDraftOf({ ...base, publisher: "示例出版社" });
    expect(pubChangedCount(origin, origin)).toBe(0);
    expect(pubBuildPatch(origin, origin)).toEqual({});
  });

  it("只改一个出版字段：计数 1，patch 只带它", () => {
    const origin = pubDraftOf(base);
    const draft = { ...origin, isbn: " 978-7-0000-0000-0 " };
    expect(pubChangedCount(draft, origin)).toBe(1);
    // 提交前去掉首尾空白，与基础字段同一口径
    expect(pubBuildPatch(draft, origin)).toEqual({ isbn: "978-7-0000-0000-0" });
  });

  it("清空出版字段下发空串（契约里空串就是清空）", () => {
    const origin = pubDraftOf({ ...base, publisher: "示例出版社", language: "zh-CN" });
    const draft = { ...origin, publisher: "", language: "" };
    expect(pubChangedCount(draft, origin)).toBe(2);
    expect(pubBuildPatch(draft, origin)).toEqual({ publisher: "", language: "" });
  });

  it("出版字段与基础字段一起改：计数与 patch 都合在一起", () => {
    const origin = pubDraftOf(base);
    const draft = { ...origin, title: "新书名", publisher: "示例出版社", pubDate: "2020-03-04" };
    expect(pubChangedCount(draft, origin)).toBe(3);
    expect(pubBuildPatch(draft, origin)).toEqual({ title: "新书名", publisher: "示例出版社", pubDate: "2020-03-04" });
  });
});
