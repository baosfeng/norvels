import { beforeEach, describe, expect, it } from "vitest";
import type { BookSummary } from "../api/types";
import {
  SHELF_CACHE_KEY,
  SHELF_CACHE_TTL_MS,
  mergeBooks,
  readShelfCache,
  writeShelfCache,
} from "./shelfCache";

function bookAt(id: number): BookSummary {
  return {
    id,
    title: `示例书 ${id}`,
    authorId: 1,
    authorName: "示例作者",
    chapterCount: 10,
    wordCount: 12000,
    volumeCount: 1,
    intro: "",
    status: "parsed",
    readStatus: "reading",
    importedAt: "2026-10-24T20:24:50.859Z",
    updatedAt: "2026-10-24T20:24:50.859Z",
  };
}

const base = {
  sort: "updated",
  authorId: null,
  tag: null,
  readStatus: null,
  size: 30,
  mode: "infinite" as const,
  items: [bookAt(1), bookAt(2)],
  total: 64,
  page: 2,
  scrollTop: 673,
  lastBookId: 2,
};

describe("书架会话缓存", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("写入后按原样读回（含分页、滚动位置与最近打开的书）", () => {
    writeShelfCache(base);
    const cached = readShelfCache();
    expect(cached).toMatchObject({ page: 2, total: 64, scrollTop: 673, lastBookId: 2, size: 30 });
    expect(cached?.items.map((item) => item.id)).toEqual([1, 2]);
  });

  it("超过 30 分钟视为过期，返回 null（下次照常从第 1 页取）", () => {
    const now = Date.now();
    writeShelfCache(base, now);
    expect(readShelfCache(now + SHELF_CACHE_TTL_MS - 1000)).not.toBeNull();
    expect(readShelfCache(now + SHELF_CACHE_TTL_MS + 1)).toBeNull();
  });

  it("坏 JSON / 结构不对 / 空存储都当没有缓存，不抛错", () => {
    expect(readShelfCache()).toBeNull();
    window.sessionStorage.setItem(SHELF_CACHE_KEY, "{不是 json");
    expect(readShelfCache()).toBeNull();
    window.sessionStorage.setItem(SHELF_CACHE_KEY, JSON.stringify({ savedAt: Date.now() }));
    expect(readShelfCache()).toBeNull();
  });

  it("合并刷新：按 id 去重、新的在前、保留已加载数量", () => {
    const merged = mergeBooks([bookAt(3), bookAt(1)], [bookAt(1), bookAt(2), bookAt(4)], 3);
    expect(merged.map((item) => item.id)).toEqual([3, 1, 2]);
    expect(mergeBooks([bookAt(9)], [bookAt(1)], 5).map((item) => item.id)).toEqual([9, 1]);
  });
});
