import { useCallback, useEffect, useState } from "react";
import { ApiError, api, type BookQuery } from "../api/client";
import type { BookSummary } from "../api/types";
import { mergeBooks } from "./shelfCache";

/** 书架取数模式：paged 按页替换（PC 分页），infinite 逐页追加（H5 无限滚动）。 */
export type BookFeedMode = "paged" | "infinite";

export interface BookFeedQuery {
  sort: NonNullable<BookQuery["sort"]>;
  authorId?: number;
  tag?: string;
  /** 阅读状态筛选；不传即全部（含未标记） */
  readStatus?: NonNullable<BookQuery["readStatus"]>;
  /** 评分筛选 1–5（0 = 未评，后端不允许用 0 筛「未评」，所以这里也不接受 0） */
  rating?: number;
  /** 书单筛选 */
  collection?: number;
  /** 大分类筛选（0015） */
  categoryL1?: string;
  /** 小分类筛选（0015）：依附于大分类 */
  categoryL2?: string;
  size: number;
  mode: BookFeedMode;
  /**
   * 从 sessionStorage 恢复的列表（返回书架时用）：先按缓存渲染，不阻塞；
   * refresh 为真才在后台拉第 1 页并与缓存合并（按 id 去重、保留已加载数量）。
   */
  restore?: BookFeedRestore;
}

export interface BookFeedRestore {
  items: BookSummary[];
  total: number;
  /** 缓存覆盖到的页码（含） */
  page: number;
  refresh: boolean;
}

export interface BookFeedState {
  items: BookSummary[];
  total: number;
  /** 已成功加载到的页码（0 = 还没拿到数据）；PC 分页用它显示当前页 */
  page: number;
  /** 首屏（或 PC 换页）加载中 */
  loading: boolean;
  /** 无限滚动追加中 */
  loadingMore: boolean;
  error: ApiError | null;
  /** 追加失败：主内容不消失，只在底部提示并可重试 */
  errorMore: ApiError | null;
  hasMore: boolean;
  setPage: (page: number) => void;
  loadMore: () => void;
  reload: () => void;
}

type Snapshot = {
  key: string;
  items: BookSummary[];
  total: number;
  page: number;
  /** 在途请求的页码；null = 空闲 */
  pending: number | null;
  loading: boolean;
  loadingMore: boolean;
  error: ApiError | null;
  errorMore: ApiError | null;
};

function idle(key: string): Snapshot {
  return { key, items: [], total: 0, page: 0, pending: 1, loading: true, loadingMore: false, error: null, errorMore: null };
}

function asApiError(err: unknown): ApiError {
  return err instanceof ApiError ? err : new ApiError("network", err instanceof Error ? err.message : "请求失败");
}

/**
 * 书架取数：一次只允许一个在途请求，结果按「查询键 + 页码」双条件校验后落库。
 * - 换排序 / 筛选 / 档位（H5↔PC）会换键：在途响应作废（effect 清理时 abort，回调里再兜一次），
 *   状态重置发生在渲染期（派生 state），不在 effect 体内 setState。
 * - infinite 模式追加，paged 模式换页替换；到底由 items.length >= total 判定。
 */
export function useBookFeed(query: BookFeedQuery): BookFeedState {
  const key = JSON.stringify([
    query.sort,
    query.authorId ?? null,
    query.tag ?? null,
    query.readStatus ?? null,
    query.rating ?? null,
    query.collection ?? null,
    query.categoryL1 ?? null,
    query.categoryL2 ?? null,
    query.size,
    query.mode,
  ]);
  const [nonce, setNonce] = useState(0);
  const [state, setState] = useState<Snapshot>(() => {
    const restore = query.restore;
    // 缓存先渲染：pending 为 null 表示「这次不阻塞取数」；要刷新时 pending=1 走后台合并
    if (restore && restore.items.length > 0) {
      return {
        key,
        items: restore.items,
        total: restore.total,
        page: restore.page,
        pending: restore.refresh ? 1 : null,
        loading: false,
        loadingMore: false,
        error: null,
        errorMore: null,
      };
    }
    return idle(key);
  });

  // 查询变了：立刻回到第 1 页的加载态（下一次 effect 发请求）
  if (state.key !== key) setState(idle(key));

  useEffect(() => {
    const page = state.pending;
    if (page === null || state.key !== key) return;
    const controller = new AbortController();
    let active = true;

    api
      .books(
        {
          sort: query.sort,
          authorId: query.authorId,
          tag: query.tag,
          readStatus: query.readStatus,
          rating: query.rating,
          collection: query.collection,
          categoryL1: query.categoryL1,
          categoryL2: query.categoryL2,
          page,
          size: query.size,
        },
        { signal: controller.signal },
      )
      .then((data) => {
        if (!active) return;
        setState((prev) => {
          // 世代兜底：查询已切换或已被新请求取代 → 丢弃这次结果
          if (prev.key !== key || prev.pending !== page) return prev;
          // 只有无限滚动在换页时追加；PC 分页是替换
          const append = query.mode === "infinite" && page > 1;
          const base = append ? prev.items : [];
          // 后台刷新第 1 页时与已有（缓存/已加载）列表合并：按 id 去重，保留已加载数量
          const merging = page === 1 && prev.items.length > 0;
          const merged = merging
            ? mergeBooks(data.items, prev.items, Math.min(data.total, Math.max(prev.items.length, data.items.length)))
            : [...base, ...data.items];
          return {
            ...prev,
            items: merged,
            total: data.total,
            page: merging ? prev.page : page,
            pending: null,
            loading: false,
            loadingMore: false,
            error: null,
            errorMore: null,
          };
        });
      })
      .catch((err: unknown) => {
        if (!active) return;
        const apiError = asApiError(err);
        if (apiError.kind === "aborted") return;
        setState((prev) => {
          if (prev.key !== key || prev.pending !== page) return prev;
          return {
            ...prev,
            pending: null,
            loading: false,
            loadingMore: false,
            error: page === 1 ? apiError : prev.error,
            errorMore: page === 1 ? null : apiError,
          };
        });
      });

    return () => {
      active = false;
      controller.abort();
    };
    // 请求语义由 key（= 查询序列化）+ pending + nonce 决定
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, state.pending, nonce]);

  const reload = useCallback(() => {
    setState((prev) => (prev.pending === 1 ? prev : { ...prev, items: [], total: 0, page: 0, pending: 1, loading: true, loadingMore: false, error: null, errorMore: null }));
    setNonce((n) => n + 1);
  }, [setState, setNonce]);

  const setPage = useCallback(
    (page: number) => {
      const next = Math.max(1, Math.trunc(page));
      setState((prev) => {
        if (prev.pending === next) return prev;
        return { ...prev, pending: next, loading: true, loadingMore: false, error: null, errorMore: null };
      });
    },
    [setState],
  );

  const loadMore = useCallback(() => {
    setState((prev) => {
      if (prev.pending !== null) return prev;
      if (prev.items.length >= prev.total) return prev;
      return { ...prev, pending: prev.page + 1, loadingMore: true, errorMore: null };
    });
  }, [setState]);

  const hasMore = state.total > state.items.length;

  return {
    items: state.items,
    total: state.total,
    page: state.page,
    loading: state.loading,
    loadingMore: state.loadingMore,
    error: state.error,
    errorMore: state.errorMore,
    hasMore,
    setPage,
    loadMore,
    reload,
  };
}
