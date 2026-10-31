import { ApiError } from "../../api/client";

/** 成员面板一页多少本：一屏能看完，翻页不至于太碎。 */
export const MEMBER_PAGE_SIZE = 24;
/** 写回前重取全量成员时的单页上限（与后端 size 上限 200 一致）。 */
export const MEMBER_FETCH_SIZE = 200;

/** 一页书的精简形状：只取 id 与 total，方便单测注入。 */
export interface MemberPage {
  ids: number[];
  total: number;
}

/**
 * 合并成员：保留原有顺序，新增的按传入顺序追加，重复 id 只留一次。
 *
 * 添加书籍的写回是「全量替换」，所以合并结果就是最终成员；顺序稳定意味着
 * 反复添加不会把书单顺序洗牌。
 */
export function mergeMemberIds(existing: number[], added: number[]): number[] {
  const seen = new Set(existing);
  const merged = [...existing];
  for (const id of added) {
    if (seen.has(id)) continue;
    seen.add(id);
    merged.push(id);
  }
  return merged;
}

/** 从成员里移出一本；不在其中时原样返回（幂等，不报错）。 */
export function withoutMember(existing: number[], bookId: number): number[] {
  return existing.filter((id) => id !== bookId);
}

/**
 * 翻页取全量成员 id。
 *
 * 为什么必须翻完：PUT 是全量替换，若只拿第一页（后端 size 上限 200）就写回，
 * 成员多的书单会被悄悄截断。fetchPage 由调用方注入，真实实现走 api.books。
 * 退出条件看 total（不依赖「本页是否满」），后端分页口径变化时才不会多请求。
 */
export async function fetchAllMemberIds(
  fetchPage: (page: number, size: number) => Promise<MemberPage>,
): Promise<number[]> {
  const ids: number[] = [];
  for (let page = 1; ; page += 1) {
    const { ids: pageIds, total } = await fetchPage(page, MEMBER_FETCH_SIZE);
    ids.push(...pageIds);
    if (pageIds.length === 0 || ids.length >= total) return ids;
  }
}

/**
 * 成员写操作失败的文案：400 是「书 id 不合法」，404 是书单没了，
 * 其余与书单管理的兜底口径一致（同一后端错误只有一种说法）。
 */
export function memberErrorText(error: unknown, action: string): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "这个书单已经不在了，刷新看看。";
    if (error.status === 400) return "这几本书里有的已经不在书库了，刷新后重试。";
    if (error.kind === "timeout") return `${action}超时了，稍后再试。`;
  }
  return `${action}没能完成，稍后再试。`;
}
