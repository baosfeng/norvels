import type { ReadStatus } from "../../api/types";
import { READ_STATUS_FILTERS } from "../../lib/readStatus";
import type { CategoryValue } from "./CategoryFilter";
import { SORTS, type SortValue } from "./shelfSorts";

/** 分组标题行右侧的「当前值」文案：收起时也要能答出「现在选的是什么」。 */
export function sortLabelOf(sort: SortValue): string {
  return SORTS.find((option) => option.value === sort)?.label ?? "";
}

export function readStatusLabelOf(status: Exclude<ReadStatus, ""> | null): string {
  return READ_STATUS_FILTERS.find((option) => option.value === status)?.label ?? "全部";
}

export function ratingLabelOf(rating: number | null): string {
  return rating === null ? "全部" : rating + " 星";
}

export function categoryLabelOf(category: CategoryValue): string {
  if (category.l1 === null) return "全部";
  return category.l2 ? category.l1 + " / " + category.l2 : category.l1;
}

export function collectionNameOf(collections: { id: number; name: string }[], collection: number | null): string {
  if (collection === null) return "全部";
  return collections.find((item) => item.id === collection)?.name || "指定书单";
}
