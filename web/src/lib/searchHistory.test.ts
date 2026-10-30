import { beforeEach, describe, expect, it } from "vitest";
import {
  SEARCH_HISTORY_KEY,
  SEARCH_HISTORY_MAX,
  clearSearchHistory,
  readSearchHistory,
  rememberSearch,
} from "./searchHistory";

/**
 * 搜索历史：本地一份、最多 10 条、去重前置、读坏数据不抛错。
 * 这些是「用户天天会碰到」的路径（搜完再搜、清空后重来），不是边界补充。
 */
describe("搜索历史", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("记一条就读得到", () => {
    rememberSearch("长夜");
    expect(readSearchHistory()).toEqual(["长夜"]);
  });

  it("最近搜的排最前，同一条词不会出现两次", () => {
    rememberSearch("长夜");
    rememberSearch("旧城");
    rememberSearch("长夜");
    expect(readSearchHistory()).toEqual(["长夜", "旧城"]);
  });

  it("去重不区分大小写，但保留用户原样写法", () => {
    rememberSearch("Night");
    rememberSearch("night");
    expect(readSearchHistory()).toEqual(["night"]);
  });

  it("最多留 10 条，第 11 条把最老的挤出去", () => {
    for (let i = 1; i <= SEARCH_HISTORY_MAX + 1; i += 1) rememberSearch(`关键词${i}`);
    const history = readSearchHistory();
    expect(history).toHaveLength(SEARCH_HISTORY_MAX);
    expect(history[0]).toBe(`关键词${SEARCH_HISTORY_MAX + 1}`);
    expect(history).not.toContain("关键词1");
  });

  it("少于 2 个字符不记（后端同样不返回结果，记下来点了也没用）", () => {
    rememberSearch("长");
    rememberSearch("  ");
    expect(readSearchHistory()).toEqual([]);
  });

  it("两端空白被折叠，不会因为多打一个空格占两条", () => {
    rememberSearch("  长夜  ");
    rememberSearch("长夜");
    expect(readSearchHistory()).toEqual(["长夜"]);
  });

  it("清空之后读回空数组", () => {
    rememberSearch("长夜");
    clearSearchHistory();
    expect(readSearchHistory()).toEqual([]);
  });

  it("存储里是坏 JSON 时当作没有历史，不抛错", () => {
    localStorage.setItem(SEARCH_HISTORY_KEY, "{不是 JSON");
    expect(readSearchHistory()).toEqual([]);
  });

  it("存储里不是数组、或混了非字符串，只留能用的", () => {
    localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify({ a: 1 }));
    expect(readSearchHistory()).toEqual([]);
    localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(["长夜", 42, "", "旧城"]));
    expect(readSearchHistory()).toEqual(["长夜", "旧城"]);
  });
});
