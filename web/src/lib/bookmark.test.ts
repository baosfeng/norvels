import { describe, expect, it } from "vitest";
import type { Bookmark } from "../api/types";
import { bookmarkLabel, bookmarkTitle, findBookmarkAt, isValidPosition } from "./bookmark";

const base: Bookmark = {
  id: 1,
  chapterId: 10,
  chapterIdx: 0,
  chapterTitle: "第一章",
  paraIndex: 2,
  label: "雨从傍晚开始下。",
  createdAt: "2026-10-25T10:00:00.000Z",
};

describe("书签：标签与位置", () => {
  it("默认标签取该段前 30 字并压掉空白", () => {
    const label = bookmarkLabel("  雨从傍晚 开始下。\n他站在巷口，没有打伞。远处有人收摊，雨停了。 ", 12);
    expect(label).toBe("雨从傍晚开始下。他站在巷");
    expect(bookmarkLabel("一段话")).toBe("一段话");
  });

  it("label 留空时退回「第 N 段」", () => {
    expect(bookmarkTitle({ ...base, label: "  " })).toBe("第 3 段");
    expect(bookmarkTitle(base)).toBe("雨从傍晚开始下。");
  });

  it("同章同段视为同一书签，不同段不冲突", () => {
    const list = [base];
    expect(findBookmarkAt(list, 10, 2)?.id).toBe(1);
    expect(findBookmarkAt(list, 10, 3)).toBeNull();
    expect(findBookmarkAt(list, 11, 2)).toBeNull();
  });

  it("非法位置不下发：章必须正整数，段必须非负整数", () => {
    expect(isValidPosition(10, 0)).toBe(true);
    expect(isValidPosition(10, -1)).toBe(false);
    expect(isValidPosition(0, 2)).toBe(false);
    expect(isValidPosition(10.5, 2)).toBe(false);
    expect(isValidPosition(Number.NaN, 2)).toBe(false);
  });
});
