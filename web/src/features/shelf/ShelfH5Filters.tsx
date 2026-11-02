import type { useAuthors, useCollections, useTags } from "../../api/hooks";
import { READ_STATUS_FILTERS } from "../../lib/readStatus";
import { ShelfFilterPanel } from "./ShelfFilterPanel";
import { useShelfChipFilters } from "./useShelfChipFilters";
import { useShelfFacetCounts, type FacetCountSpec } from "./useShelfFacetCounts";
import type { useShelfFilters } from "./useShelfFilters";

/** 评分候选：与 PC 工具栏下拉同序（5 → 1 星）。 */
const RATING_STARS = [5, 4, 3, 2, 1] as const;

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
  const facetCounts = useShelfFacetCounts(facetSpecs(filters), !loading);

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
      facetCounts={facetCounts.counts}
      resultCount={loading ? null : total}
      chips={chips}
      onClearAll={onClearAll}
      onViewResults={onViewResults}
    />
  );
}

/**
 * 要计数的两组（阅读状态 / 评分）：每项都带上**其它**维度的筛选、唯独不带自己这一维——
 * 「在读（37）」里的 37 与点「在读」之后列表的总数来自同一个查询，两个数字不可能对不上。
 *
 * 为什么不去数分类 / 书单 / 标签：它们自带接口给的计数（分类树与标签、书单接口都带 count），
 * 再发一轮请求既多余又可能与标签页上的数字打架。
 *
 * 全部（不筛这一维）的数目就是结果总数，由底部「查看 N 本」负责，这里不重复发一次。
 */
function facetSpecs(filters: Filters): FacetCountSpec[] {
  const base = otherDimensions(filters);
  const statuses: FacetCountSpec[] = READ_STATUS_FILTERS.filter((option) => option.value !== null).map((option) => ({
    key: `status:${option.value}`,
    query: { ...base, readStatus: option.value as NonNullable<FacetCountSpec["query"]["readStatus"]> },
  }));
  const ratings: FacetCountSpec[] = RATING_STARS.map((star) => ({
    key: `rating:${star}`,
    query: { ...base, rating: star },
  }));
  return [...statuses, ...ratings];
}

/** 除阅读状态与评分之外的筛选：状态与评分的计数都要带上它们，才与列表口径一致。 */
function otherDimensions(filters: Filters): FacetCountSpec["query"] {
  return {
    sort: filters.sort,
    authorId: filters.authorId ?? undefined,
    tag: filters.tag ?? undefined,
    collection: filters.collection ?? undefined,
    categoryL1: filters.category.l1 ?? undefined,
    categoryL2: filters.category.l2 ?? undefined,
  };
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
