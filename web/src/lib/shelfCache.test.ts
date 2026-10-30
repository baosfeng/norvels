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

  // 回归：readShelfCache 曾漏拼 categoryL1/categoryL2（接口声明了、回读漏了），
  // 表现为「从分类筛选进详情再返回，筛选条件又没了」（ShelfPage 的 cached?.categoryL1 恒为 null）。
  it("分类两级（categoryL1/L1）写入后能原样读回", () => {
    writeShelfCache({ ...base, categoryL1: "武侠", categoryL2: "群像" });
    const cached = readShelfCache();
    expect(cached?.categoryL1).toBe("武侠");
    expect(cached?.categoryL2).toBe("群像");
  });

  it("筛选字段全部往返：作者、标签、阅读状态、评分、书单、分类", () => {
    writeShelfCache({
      ...base,
      authorId: 7,
      tag: "民国",
      readStatus: "done",
      rating: 4,
      collection: 3,
      categoryL1: "武侠",
      categoryL2: "群像",
    });
    expect(readShelfCache()).toMatchObject({
      authorId: 7,
      tag: "民国",
      readStatus: "done",
      rating: 4,
      collection: 3,
      categoryL1: "武侠",
      categoryL2: "群像",
    });
  });

  it("老缓存没有分类字段 / 分类是空串时读成 null（不造出无效筛选值）", () => {
    writeShelfCache(base);
    expect(readShelfCache()?.categoryL1).toBeNull();
    expect(readShelfCache()?.categoryL2).toBeNull();
    writeShelfCache({ ...base, categoryL1: "", categoryL2: "" });
    expect(readShelfCache()?.categoryL1).toBeNull();
    expect(readShelfCache()?.categoryL2).toBeNull();
  });

  it("合并刷新：按 id 去重、新的在前、保留已加载数量", () => {
    const merged = mergeBooks([bookAt(3), bookAt(1)], [bookAt(1), bookAt(2), bookAt(4)], 3);
    expect(merged.map((item) => item.id)).toEqual([3, 1, 2]);
    expect(mergeBooks([bookAt(9)], [bookAt(1)], 5).map((item) => item.id)).toEqual([9, 1]);
  });
});
