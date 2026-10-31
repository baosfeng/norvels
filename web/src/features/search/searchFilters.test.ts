import { describe, expect, it } from "vitest";
import {
  EMPTY_FILTERS,
  activeFilterCount,
  filtersToSearchParams,
  readFilters,
  writeFilters,
} from "./searchFilters";

/**
 * 口径：筛选状态只有一份真相 = 地址栏。这里守三件事——
 * 1) 读：合法取值原样、非法/越界一律当「全部」（手改链接不该把页面变成 400）；
 * 2) 写：只落非默认值 + 回到第 1 页（URL 干净、分享出去不带一长串空参数）；
 * 3) 下发：零值不下发，后端「空值即不过滤」与前端一致。
 */
describe("搜索筛选：地址栏读写", () => {
  it("读：合法取值原样还原", () => {
    const params = new URLSearchParams(
      "q=旧城&categoryL1=玄幻&categoryL2=东方玄幻&rating=5&status=reading&serial=ongoing&author=9&tag=武侠",
    );
    expect(readFilters(params)).toEqual({
      categoryL1: "玄幻",
      categoryL2: "东方玄幻",
      rating: "5",
      status: "reading",
      serial: "ongoing",
      author: "9",
      tag: "武侠",
    });
    expect(activeFilterCount(readFilters(params))).toBe(7);
  });

  it("读：没有参数时全是默认值，计数为 0", () => {
    expect(readFilters(new URLSearchParams())).toEqual(EMPTY_FILTERS);
    expect(activeFilterCount(EMPTY_FILTERS)).toBe(0);
  });

  it("读：非法取值一律当「全部」（后端对非法值报 400，前端先挡一层）", () => {
    const filters = readFilters(new URLSearchParams("rating=9&status=bogus&serial=bogus&author=abc"));
    expect(filters.rating).toBe("");
    expect(filters.status).toBe("");
    expect(filters.serial).toBe("");
    expect(filters.author).toBe("");
    // rating 0 是「未评」，不是一个可见筛选项
    expect(readFilters(new URLSearchParams("rating=0")).rating).toBe("");
    expect(readFilters(new URLSearchParams("author=-3")).author).toBe("");
  });

  it("读：没有大分类时其余筛选一律不生效（渐进展开下它们是界面上看不见的组）", () => {
    const filters = readFilters(new URLSearchParams("rating=5&tag=武侠&status=done&serial=ongoing&author=9"));
    expect(filters).toEqual(EMPTY_FILTERS);
    expect(activeFilterCount(filters)).toBe(0);
  });

  it("读：只给小分类（没有大分类）时不算生效——与后端 appendCategoryWhere 同口径", () => {
    expect(readFilters(new URLSearchParams("categoryL2=奇幻")).categoryL2).toBe("");
    expect(activeFilterCount(readFilters(new URLSearchParams("categoryL2=奇幻")))).toBe(0);
  });

  it("写：只落非默认值，并回到第 1 页", () => {
    const next = writeFilters(new URLSearchParams("q=旧城&page=3"), {
      ...EMPTY_FILTERS,
      categoryL1: "玄幻",
      rating: "4",
    });
    expect(next.get("q")).toBe("旧城");
    expect(next.get("categoryL1")).toBe("玄幻");
    expect(next.get("rating")).toBe("4");
    expect(next.get("page")).toBeNull();
    // 默认值不写进地址栏
    expect(next.get("status")).toBeNull();
    expect(next.get("tag")).toBeNull();
  });

  it("写：清空筛选只删筛选键，关键词留着", () => {
    const next = writeFilters(new URLSearchParams("q=旧城&categoryL1=玄幻&tag=武侠&page=2"), EMPTY_FILTERS);
    expect(next.toString()).toBe("q=%E6%97%A7%E5%9F%8E");
  });

  it("下发：默认值不下发，数字与枚举转成接口口径", () => {
    expect(filtersToSearchParams(EMPTY_FILTERS, { type: "book", page: 1, size: 8 })).toEqual({
      type: "book",
      page: 1,
      size: 8,
    });
    const params = filtersToSearchParams(
      { ...EMPTY_FILTERS, categoryL1: "玄幻", categoryL2: "东方玄幻", rating: "5", status: "reading", serial: "none", author: "9", tag: "武侠" },
      { type: "book", page: 2, size: 8 },
    );
    expect(params).toEqual({
      type: "book",
      page: 2,
      size: 8,
      categoryL1: "玄幻",
      categoryL2: "东方玄幻",
      rating: 5,
      readStatus: "reading",
      serialStatus: "none",
      authorId: 9,
      tag: "武侠",
    });
  });
});
