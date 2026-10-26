import { api } from "../../api/client";
import type { RequestOptions } from "../../api/client";

/**
 * 书单成员读写：契约里只有「全量替换」（PUT /collections/{id}/books），
 * 没有「查某书在哪些书单」「单本增删」的端点。所以要先按 collection 参数分页把成员取全，
 * 再算「加上/去掉这一本」，最后整份回写——否则整份替换会把已有成员一起冲掉。
 *
 * 单页上限 200（后端 maxSize）；成员数超过上限的异常书单直接放弃改动（返回 complete=false），
 * 宁可不给操作，也不写坏别人的收藏夹。
 */
export const MEMBER_PAGE_SIZE = 200;
/** 一个书单最多回写这么多本；超过就不在这里改，避免把没取到的成员抹掉。 */
export const MEMBER_CAP = 1000;

export interface CollectionMembers {
  ids: number[];
  /** false = 成员超过 MEMBER_CAP，成员列表不完整，不能拿来整份回写 */
  complete: boolean;
}

/** 取某个书单的全部成员；分页拉取，到 total 或到 MEMBER_CAP 为止。 */
export async function fetchCollectionMembers(
  collectionId: number,
  options: RequestOptions = {},
): Promise<CollectionMembers> {
  const ids: number[] = [];
  const seen = new Set<number>();
  let page = 1;
  let total: number;
  do {
    const data = await api.books(
      { collection: collectionId, page, size: MEMBER_PAGE_SIZE, sort: "updated" },
      options,
    );
    total = data.total;
    for (const book of data.items) {
      if (seen.has(book.id)) continue;
      seen.add(book.id);
      ids.push(book.id);
    }
    if (data.items.length === 0) break;
    page += 1;
  } while (ids.length < total && ids.length < MEMBER_CAP);

  return { ids, complete: ids.length >= total && ids.length <= MEMBER_CAP };
}

/**
 * 算一次「加上/去掉这一本」后的成员列表。
 * 成员不完整时返回 null：调用方据此禁用改动，不做整份替换。
 */
export function toggleMember(
  members: CollectionMembers,
  bookId: number,
  add: boolean,
): number[] | null {
  if (!members.complete) return null;
  const set = new Set(members.ids);
  if (add) set.add(bookId);
  else set.delete(bookId);
  return [...set];
}
