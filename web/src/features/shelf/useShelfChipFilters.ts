import { useCallback } from "react";
import { buildFilterChips, type FilterChip } from "./shelfFilterChips";
import type { useShelfFilters } from "./useShelfFilters";

type Filters = ReturnType<typeof useShelfFilters>;

/**
 * H5 列表顶部的筛选回显：把筛选状态转成 chip 列表（每项自带一个"只清这一项"的回调）。
 *
 * 单独成 hook 是为了让 ShelfPage 只负责取数与排版：七项筛选的清除口径集中在这里，
 * 单清一项与「清除全部」共用 setter，不存在两套写法。
 */
export function useShelfChipFilters({
  filters,
  authorName,
  collectionName,
  onAfterClear,
}: {
  filters: Filters;
  /** 候选表把 id 翻成人话；还没取回来时传 null，chip 退回通用说法 */
  authorName: string | null;
  collectionName: string | null;
  /** 清掉一项后做的事（回到列表顶部） */
  onAfterClear: () => void;
}): FilterChip[] {
  const { setReadStatus, setRating, setAuthorId, setCollection, setCategory, setTag } = filters;
  const clearOne = useCallback(
    (apply: () => void) => {
      apply();
      onAfterClear();
    },
    [onAfterClear],
  );

  return buildFilterChips({
    readStatus: filters.readStatus,
    rating: filters.rating,
    authorId: filters.authorId,
    authorName,
    collection: filters.collection,
    collectionName,
    category: filters.category,
    tag: filters.tag,
    onClearReadStatus: () => clearOne(() => setReadStatus(null)),
    onClearRating: () => clearOne(() => setRating(null)),
    onClearAuthor: () => clearOne(() => setAuthorId(null)),
    onClearCollection: () => clearOne(() => setCollection(null)),
    onClearCategory: () => clearOne(() => setCategory({ l1: null, l2: null })),
    onClearTag: () => clearOne(() => setTag(null)),
  });
}
