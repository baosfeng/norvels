import { describe, expect, it } from "vitest";
import {
  DEFAULT_FILTERS,
  hasShelfFilterQuery,
  hasShelfQuery,
  mergeShelfQuery,
  parseShelfQuery,
  writeShelfQuery,
} from "./shelfQuery";

describe("书架筛选 ⇄ 地址栏", () => {
  it("默认状态不写任何参数：空地址与「全部书」等价", () => {
    expect(writeShelfQuery(DEFAULT_FILTERS).toString()).toBe("");
  });

  it("写出的参数能被原样读回（七个筛选 + 排序）", () => {
    const filters = {
      sort: "title" as const,
      authorId: 7,
      tag: "民国",
      readStatus: "done" as const,
      rating: 4,
      collection: 3,
      categoryL1: "武侠",
      categoryL2: "群像",
    };
    const parsed = parseShelfQuery(writeShelfQuery(filters));
    expect(parsed).toEqual(filters);
  });

  it("非法值当没给：不报错、不猜，分类小项无主时丢弃", () => {
    expect(parseShelfQuery(new URLSearchParams("sort=乱写"))).toEqual({});
    expect(parseShelfQuery(new URLSearchParams("status=unknown"))).toEqual({});
    expect(parseShelfQuery(new URLSearchParams("rating=9"))).toEqual({});
    expect(parseShelfQuery(new URLSearchParams("author=-1"))).toEqual({});
    expect(parseShelfQuery(new URLSearchParams("l2=群像"))).toEqual({});
  });

  it("合并时只替换书架自己的参数，别人的查询串原样保留；无变化返回 null", () => {
    const prev = new URLSearchParams("tag=民国&utm=x");
    const next = mergeShelfQuery(prev, { ...DEFAULT_FILTERS, sort: "words" });
    expect(next?.toString()).toBe("utm=x&sort=words");
    expect(mergeShelfQuery(new URLSearchParams("sort=words"), { ...DEFAULT_FILTERS, sort: "words" })).toBeNull();
  });

  it("hasShelfQuery 只认书架参数：其它参数不算「以地址为准」", () => {
    expect(hasShelfQuery(new URLSearchParams("utm=x"))).toBe(false);
    expect(hasShelfQuery(new URLSearchParams("tag=民国"))).toBe(true);
  });

  it("hasShelfFilterQuery 看共享书库的筛选（排序不算）：地址带筛选 = 必须落在全部书", () => {
    expect(hasShelfFilterQuery(new URLSearchParams("sort=title"))).toBe(false);
    expect(hasShelfFilterQuery(new URLSearchParams("view=mine"))).toBe(false);
    expect(hasShelfFilterQuery(new URLSearchParams("author=8"))).toBe(true);
    expect(hasShelfFilterQuery(new URLSearchParams("l1=武侠&l2=群像"))).toBe(true);
  });
});
