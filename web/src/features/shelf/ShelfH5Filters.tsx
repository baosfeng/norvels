import type { useAuthors, useCollections, useTags } from "../../api/hooks";
import { ShelfFilterPanel } from "./ShelfFilterPanel";
import { useShelfChipFilters } from "./useShelfChipFilters";
import type { useShelfFilters } from "./useShelfFilters";

type Filters = ReturnType<typeof useShelfFilters>;
type Query<T> = { data: T | null };

/**
 * H5 的筛选区：面板本体在 `ShelfFilterPanel`，这里只做接线——把候选表里的 id 翻成名字
 * （回显 chip 要用）、把「清掉一项之后做什么」交给同一处，并把当前命中数递给底部按钮。
 *
 * 单独成文件是为了让 ShelfPage 只负责取数与排版：七项筛选的清除口径集中在一处，
 * 单清一项与「清除全部」共用 setter，不存在两套写法。
 */
export function ShelfH5Filters({
  filters,
  authors,
  tags,
  collections,
  loading,
  total,
  onAfterChange,
  onClearAll,
  onViewResults,
}: {
  filters: Filters;
  authors: ReturnType<typeof useAuthors>;
  tags: ReturnType<typeof useTags>;
  collections: ReturnType<typeof useCollections>;
  loading: boolean;
  total: number;
  /** 改完筛选之后要做的事（回到列表顶部） */
  onAfterChange: () => void;
  /** 一次清空全部筛选（含「只看有更新」），由 ShelfPage 统一给：清筛选的口径只能有一处 */
  onClearAll: () => void;
  onViewResults: () => void;
}) {
  const authorName = authorOf(authors, filters.authorId);
  const collectionName = collectionOf(collections, filters.collection);
  const chips = useShelfChipFilters({ filters, authorName, collectionName, onAfterClear: onAfterChange });

  return (
    <ShelfFilterPanel
      sort={filters.sort}
      onChooseSort={(next) => choose(filters.setSort, next, onAfterChange)}
      readStatus={filters.readStatus}
      onChooseReadStatus={(next) => choose(filters.setReadStatus, next, onAfterChange)}
      rating={filters.rating}
      onChooseRating={(next) => choose(filters.setRating, next, onAfterChange)}
      authorId={filters.authorId}
      authors={authors.data?.items ?? []}
      onChooseAuthor={(next) => choose(filters.setAuthorId, next, onAfterChange)}
      collection={filters.collection}
      collections={collections.data?.items ?? []}
      onChooseCollection={(next) => choose(filters.setCollection, next, onAfterChange)}
      category={filters.category}
      onChooseCategory={(next) => choose(filters.setCategory, next, onAfterChange)}
      tag={filters.tag}
      tags={tags.data?.items ?? []}
      onChooseTag={(next) => choose(filters.setTag, next, onAfterChange)}
      filtering={filters.filtering}
      filterCount={filters.filterCount}
      resultCount={loading ? null : total}
      chips={chips}
      onClearAll={onClearAll}
      onViewResults={onViewResults}
    />
  );
}

/** 选中即生效：先落值，再把列表带回顶部（换筛选等价于换一屏结果）。 */
function choose<T>(set: (next: T) => void, next: T, onAfterChange: () => void) {
  set(next);
  onAfterChange();
}

/** 候选表把 id 翻成人话；还没取回来（或不在候选里）时给 null，chip 退回通用说法。 */
function authorOf(authors: Query<{ items: { id: number; name: string }[] }>, authorId: number | null): string | null {
  if (authorId === null) return null;
  return authors.data?.items.find((item) => item.id === authorId)?.name ?? null;
}

function collectionOf(collections: Query<{ items: { id: number; name: string }[] }>, collection: number | null): string | null {
  if (collection === null) return null;
  return collections.data?.items.find((item) => item.id === collection)?.name ?? null;
}
