import { describe, expect, it, vi } from "vitest";
import { fetchAllMemberIds, mergeMemberIds, withoutMember, MEMBER_FETCH_SIZE } from "./collectionMembers";

describe("书单成员：合并与移出的纯逻辑", () => {
  it("合并保留原有顺序，新书按传入顺序追加", () => {
    expect(mergeMemberIds([3, 1], [7, 1, 9])).toEqual([3, 1, 7, 9]);
  });

  it("合并去重：同一本加两次只留一条", () => {
    expect(mergeMemberIds([1, 2], [2, 2, 2])).toEqual([1, 2]);
  });

  it("移出一本：不在其中时原样返回（幂等）", () => {
    expect(withoutMember([1, 2, 3], 2)).toEqual([1, 3]);
    expect(withoutMember([1, 3], 2)).toEqual([1, 3]);
  });
});

describe("书单成员：写回前翻页取全量", () => {
  it("按 total 翻完所有页（不靠「本页是否满」判断）", async () => {
    const pages: Record<number, number[]> = { 1: [1, 2], 2: [3], 3: [] };
    const fetchPage = vi.fn(async (page: number) => ({ ids: pages[page] ?? [], total: 3 }));
    expect(await fetchAllMemberIds(fetchPage)).toEqual([1, 2, 3]);
    expect(fetchPage).toHaveBeenCalledTimes(2);
    expect(fetchPage).toHaveBeenCalledWith(1, MEMBER_FETCH_SIZE);
  });

  it("空书单只请求一次", async () => {
    const fetchPage = vi.fn(async () => ({ ids: [], total: 0 }));
    expect(await fetchAllMemberIds(fetchPage)).toEqual([]);
    expect(fetchPage).toHaveBeenCalledTimes(1);
  });

  it("超过单页上限：继续翻，不会截断", async () => {
    const big = Array.from({ length: MEMBER_FETCH_SIZE }, (_, i) => i + 1);
    const fetchPage = vi.fn(async (page: number) => ({
      ids: page === 1 ? big : page === 2 ? [999] : [],
      total: MEMBER_FETCH_SIZE + 1,
    }));
    const ids = await fetchAllMemberIds(fetchPage);
    expect(ids).toHaveLength(MEMBER_FETCH_SIZE + 1);
    expect(ids.at(-1)).toBe(999);
  });
});
