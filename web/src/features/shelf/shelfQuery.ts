import type { ReadStatus } from "../../api/types";
import { SORTS, type SortValue } from "./shelfSorts";

/**
 * 书架筛选状态 ⇄ 地址栏（`?sort=&author=&tag=&status=&rating=&collection=&l1=&l2=`）。
 *
 * 为什么进 URL 而不只放 sessionStorage：分类筛选后进详情再返回，会话缓存能兜住「同一个标签页」，
 * 但刷新、后退、把链接发给别人都会丢。地址是唯一能自证状态的载体，缓存只作兜底
 * （缓存还要负责列表、页码与滚动位置，那部分不进 URL）。
 *
 * 口径：**只写非默认值**。默认状态（最近更新 / 无筛选）不产生任何查询参数，
 * 空地址与「全部书」是同一件事，链接短且可读。
 */
export interface ShelfFilters {
  sort: SortValue;
  authorId: number | null;
  tag: string | null;
  readStatus: Exclude<ReadStatus, ""> | null;
  rating: number | null;
  collection: number | null;
  categoryL1: string | null;
  categoryL2: string | null;
}

export const DEFAULT_FILTERS: ShelfFilters = {
  sort: "updated",
  authorId: null,
  tag: null,
  readStatus: null,
  rating: null,
  collection: null,
  categoryL1: null,
  categoryL2: null,
};

/** 地址上的参数名：集中一处，解析与序列化不会各写一套。 */
export const SHELF_QUERY_KEYS = ["sort", "author", "tag", "status", "rating", "collection", "l1", "l2"] as const;

const READ_STATUS_VALUES = ["want", "reading", "done"] as const;

function positiveInt(raw: string | null): number | null {
  if (!raw) return null;
  const value = Number(raw);
  return Number.isInteger(value) && value > 0 ? value : null;
}

function text(raw: string | null): string | null {
  const trimmed = (raw ?? "").trim();
  return trimmed === "" ? null : trimmed;
}

/** 解析地址：只回填地址上**明确给出**且合法的键，非法值一律当没给（不报错、不猜）。 */
export function parseShelfQuery(params: URLSearchParams): Partial<ShelfFilters> {
  const out: Partial<ShelfFilters> = {};
  const sort = params.get("sort");
  if (sort && SORTS.some((option) => option.value === sort)) out.sort = sort as SortValue;

  const authorId = positiveInt(params.get("author"));
  if (authorId !== null) out.authorId = authorId;

  const tag = text(params.get("tag"));
  if (tag !== null) out.tag = tag;

  const status = params.get("status") ?? "";
  if ((READ_STATUS_VALUES as readonly string[]).includes(status)) {
    out.readStatus = status as Exclude<ReadStatus, "">;
  }

  const rating = positiveInt(params.get("rating"));
  if (rating !== null && rating <= 5) out.rating = rating;

  const collection = positiveInt(params.get("collection"));
  if (collection !== null) out.collection = collection;

  const l1 = text(params.get("l1"));
  if (l1 !== null) out.categoryL1 = l1;

  // 小分类依附大分类：没有 l1 的 l2 是无主值（后端也会忽略），直接丢掉
  const l2 = l1 === null ? null : text(params.get("l2"));
  if (l2 !== null) out.categoryL2 = l2;

  return out;
}

/** 地址上有没有任何书架筛选参数：有 = 以地址为准，不沿用会话缓存里的筛选。 */
export function hasShelfQuery(params: URLSearchParams): boolean {
  return SHELF_QUERY_KEYS.some((key) => params.has(key));
}

/**
 * 地址上有没有「共享书库的筛选」（排序不算）。
 *
 * 作者 / 标签 / 状态 / 评分 / 书单 / 分类只属于「全部书」：「我的书架」是按加入时间的个人清单，
 * 那些筛选器在那边既不渲染也不生效。地址一旦带了其中之一，视图就必须落在全部书——
 * 否则从详情页点作者（/shelf?author=8）会停在空的「我的书架」，筛选条件连回显都没有。
 */
export function hasShelfFilterQuery(params: URLSearchParams): boolean {
  return SHELF_QUERY_KEYS.some((key) => key !== "sort" && params.has(key));
}

/** 序列化：默认值不落地址；l2 只在有 l1 时写出。 */
export function writeShelfQuery(filters: ShelfFilters): URLSearchParams {
  const out = new URLSearchParams();
  if (filters.sort !== DEFAULT_FILTERS.sort) out.set("sort", filters.sort);
  if (filters.authorId !== null) out.set("author", String(filters.authorId));
  if (filters.tag !== null) out.set("tag", filters.tag);
  if (filters.readStatus !== null) out.set("status", filters.readStatus);
  if (filters.rating !== null) out.set("rating", String(filters.rating));
  if (filters.collection !== null) out.set("collection", String(filters.collection));
  if (filters.categoryL1 !== null) {
    out.set("l1", filters.categoryL1);
    if (filters.categoryL2 !== null) out.set("l2", filters.categoryL2);
  }
  return out;
}

/**
 * 把筛选写进当前地址，保留不属于书架的参数（不整段覆盖别人的查询串）。
 * 返回 null = 地址无需改动，调用方据此避免多余的历史压栈。
 */
export function mergeShelfQuery(prev: URLSearchParams, filters: ShelfFilters): URLSearchParams | null {
  const next = new URLSearchParams(prev);
  for (const key of SHELF_QUERY_KEYS) next.delete(key);
  for (const [key, value] of writeShelfQuery(filters)) next.set(key, value);
  return next.toString() === prev.toString() ? null : next;
}
