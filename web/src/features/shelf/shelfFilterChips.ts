import type { ReadStatus } from "../../api/types";
import { readStatusFilterLabel } from "../../lib/readStatus";
import type { CategoryValue } from "./CategoryFilter";

export interface FilterChip {
  key: string;
  label: string;
  onClear: () => void;
}

export interface FilterChipInput {
  readStatus: Exclude<ReadStatus, ""> | null;
  rating: number | null;
  authorId: number | null;
  /** 作者名要等候选列表回来才知道；不知道时只写「指定作者」，不编名字 */
  authorName: string | null;
  collection: number | null;
  collectionName: string | null;
  category: CategoryValue;
  tag: string | null;
  onClearReadStatus: () => void;
  onClearRating: () => void;
  onClearAuthor: () => void;
  onClearCollection: () => void;
  onClearCategory: () => void;
  onClearTag: () => void;
}

/** 筛选回显：只列真正生效的筛选条件。排序不进这一行——它改变顺序、不改变结果集合。 */
export function buildFilterChips(input: FilterChipInput): FilterChip[] {
  const chips: FilterChip[] = [];
  const add = (key: string, active: boolean, label: string, onClear: () => void) => {
    if (active) chips.push({ key, label, onClear });
  };
  add("status", input.readStatus !== null, readStatusFilterLabel(input.readStatus), input.onClearReadStatus);
  add("rating", input.rating !== null, `${input.rating ?? 0} 星`, input.onClearRating);
  add("author", input.authorId !== null, input.authorName ?? "指定作者", input.onClearAuthor);
  add("collection", input.collection !== null, input.collectionName ?? "指定书单", input.onClearCollection);
  add("category", input.category.l1 !== null, categoryLabel(input.category), input.onClearCategory);
  add("tag", input.tag !== null, `标签：${input.tag ?? ""}`, input.onClearTag);
  return chips;
}

function categoryLabel(category: CategoryValue): string {
  if (category.l1 === null) return "";
  return category.l2 ? `${category.l1} / ${category.l2}` : category.l1;
}
