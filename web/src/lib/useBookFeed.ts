import { useCallback, useEffect, useState } from "react";
import { ApiError, api, type BookQuery } from "../api/client";
import type { BookSummary } from "../api/types";

/** 书架取数模式：paged 按页替换（PC 分页），infinite 逐页追加（H5 无限滚动）。 */
export type BookFeedMode = "paged" | "infinite";

export interface BookFeedQuery {
  sort: NonNullable<BookQuery["sort"]>;
  authorId?: number;
  tag?: string;
  size: number;
  mode: BookFeedMode;
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
  const key = JSON.stringify([query.sort, query.authorId ?? null, query.tag ?? null, query.size, query.mode]);
  const [nonce, setNonce] = useState(0);
  const [state, setState] = useState<Snapshot>(() => idle(""));

  // 查询变了：立刻回到第 1 页的加载态（下一次 effect 发请求）
  if (state.key !== key) setState(idle(key));

  useEffect(() => {
    const page = state.pending;
    if (page === null || state.key !== key) return;
    const controller = new AbortController();
    let active = true;

    api
      .books(
        { sort: query.sort, authorId: query.authorId, tag: query.tag, page, size: query.size },
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
          return {
            ...prev,
            items: [...base, ...data.items],
            total: data.total,
            page,
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
