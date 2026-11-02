import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import type { ReadStatus } from "../../api/types";
import type { CategoryValue } from "./CategoryFilter";
import { mergeShelfQuery, parseShelfQuery } from "./shelfQuery";
import { SORTS, type SortValue } from "./shelfSorts";

/**
 * 书架的筛选状态（两档同一套），以及它与会话缓存 / 地址栏的关系。
 *
 * 优先级：地址 > 会话缓存 > 默认值。
 * - 地址是唯一能自证状态的载体：刷新、后退、把链接发给别人都能复现同一屏（`shelfQuery.ts`）。
 * - 缓存只解释「上次看到哪」：从详情/阅读器返回时把筛选与列表一起带回来，但换链接进来不该沿用。
 * - 带 `resetFilters` state 进来（驾驶舱「全部书架 →」）时连缓存也不读，落到全部书。
 */
export function useShelfFilters(cached: {
  sort?: string;
  authorId?: number | null;
  tag?: string | null;
  readStatus?: Exclude<ReadStatus, ""> | null;
  rating?: number | null;
  collection?: number | null;
  categoryL1?: string | null;
  categoryL2?: string | null;
} | null) {
  const [searchParams, setSearchParams] = useSearchParams();
  // 首帧捕获地址上的筛选；之后地址跟随筛选变化（下面的 effect），不回读
  const [urlFilters] = useState(() => parseShelfQuery(searchParams));
  // 调用方在「地址权威 / 要求重置筛选」时传 null 作为 cached，所以这里只需按地址覆盖缓存
  const initial = { ...cached, ...urlFilters };

  const [sort, setSort] = useState<SortValue>(() =>
    SORTS.some((option) => option.value === initial.sort) ? (initial.sort as SortValue) : "updated",
  );
  const [authorId, setAuthorId] = useState<number | null>(initial.authorId ?? null);
  const [tag, setTag] = useState<string | null>(initial.tag ?? null);
  const [readStatus, setReadStatus] = useState<Exclude<ReadStatus, ""> | null>(initial.readStatus ?? null);
  /** 评分筛选 1–5；0 星 = 未评，后端不提供「只有未评」，所以这里没有「未评」档 */
  const [rating, setRating] = useState<number | null>(initial.rating ?? null);
  /** 书单筛选（扁平收藏夹，一本书可属多个书单） */
  const [collection, setCollection] = useState<number | null>(initial.collection ?? null);
  /** 分类筛选（0015）：大分类 + 联动的小分类 */
  const [category, setCategory] = useState<CategoryValue>({
    l1: initial.categoryL1 ?? null,
    l2: initial.categoryL2 ?? null,
  });

  // 筛选变化写回地址：刷新、后退、复制链接都能复现同一屏（replace 不往后退栈里塞历史）
  useEffect(() => {
    setSearchParams(
      (prev) =>
        mergeShelfQuery(prev, {
          sort,
          authorId,
          tag,
          readStatus,
          rating,
          collection,
          categoryL1: category.l1,
          categoryL2: category.l2,
        }) ?? prev,
      { replace: true },
    );
  }, [sort, authorId, tag, readStatus, rating, collection, category.l1, category.l2, setSearchParams]);

  const filtering =
    authorId !== null ||
    tag !== null ||
    readStatus !== null ||
    rating !== null ||
    collection !== null ||
    category.l1 !== null;
  const filterCount = [authorId, tag, readStatus, rating, collection, category.l1, category.l2].filter(
    (value) => value !== null,
  ).length;

  /** 一次清空七项筛选，只清筛选不动排序。 */
  const resetFilters = useCallback(() => {
    setAuthorId(null);
    setTag(null);
    setReadStatus(null);
    setRating(null);
    setCollection(null);
    setCategory({ l1: null, l2: null });
  }, []);

  return {
    sort,
    setSort,
    authorId,
    setAuthorId,
    tag,
    setTag,
    readStatus,
    setReadStatus,
    rating,
    setRating,
    collection,
    setCollection,
    category,
    setCategory,
    filtering,
    filterCount,
    resetFilters,
  };
}
