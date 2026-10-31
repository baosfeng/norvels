import { useCallback, useEffect, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { BookQuery } from "../../api/client";
import type { BookSummary } from "../../api/types";

/** 选择器里一页列多少本候选书：滚到底再追加下一页。 */
export const PICKER_PAGE_SIZE = 20;

/** 添加书籍弹层的候选书分页：已加载的页累积成一份列表，滚动到底再追加。 */
export interface PickerBooks {
  items: BookSummary[];
  /** 后端给的总数：顶部「已显示 M / 共 N 本」与「还有没有下一页」都以它为准。 */
  total: number;
  /** 首屏还没拿到第一页：列表区放整块加载骨架。 */
  loadingFirst: boolean;
  /** 正在追加下一页：列表底部显示「正在加载…」。 */
  loadingMore: boolean;
  /** 还有没加载的页。 */
  hasMore: boolean;
  /** 最近一次取数失败：行内提示 + 重试，已加载的页照常可勾。 */
  error: ApiError | null;
  loadMore: () => void;
  retry: () => void;
}

type Snapshot = {
  /** 这份列表对应的关键词；关键词一变就整份丢弃、回到第 1 页 */
  key: string;
  items: BookSummary[];
  total: number;
  /** 已加载的页数 */
  pages: number;
  /** 现在有一次待取的页（首屏、翻页或重试） */
  loading: boolean;
  error: ApiError | null;
  /** 重试计数：让 effect 再跑一次同一个页码，不动 pages */
  nonce: number;
};

function initialSnapshot(key: string): Snapshot {
  return { key, items: [], total: 0, pages: 0, loading: true, error: null, nonce: 0 };
}

function toApiError(err: unknown): ApiError {
  return err instanceof ApiError ? err : new ApiError("network", err instanceof Error ? err.message : "请求失败");
}

/**
 * 候选书分页取数：一页一页往后取，累积成一份列表。
 *
 * 三个取舍：
 *   - 不做自动补足（首屏没填满就自动续取）：候选书一屏放不下 20 本，滚动一定发生；
 *     真按「容器没滚动条就续取」写，长列表会一次性把几百本全拉下来，反而慢；
 *   - 重试只重发失败的那一页，已加载的页留在屏幕上：失败时读者勾好的书不该消失；
 *   - 关键词变化整份重置（回到第 1 页、清空已加载页）；已勾选集合由调用方持有，不受影响。
 */
export function usePickerBooks(keyword: string): PickerBooks {
  const [snap, setSnap] = useState<Snapshot>(() => initialSnapshot(keyword));

  // 关键词变化：在本次渲染内换一份空快照（与 lib/useResource 同一写法，不在 effect 里同步 setState）
  if (snap.key !== keyword) {
    setSnap(initialSnapshot(keyword));
  }

  useEffect(() => {
    if (!snap.loading) return;
    const target = snap.pages + 1;
    const currentKey = snap.key;
    const controller = new AbortController();
    let active = true;

    const params: BookQuery = currentKey
      ? { q: currentKey, page: target, size: PICKER_PAGE_SIZE, sort: "title" }
      : { page: target, size: PICKER_PAGE_SIZE, sort: "updated" };

    api
      .books(params, { signal: controller.signal })
      .then((res) => {
        if (!active) return;
        setSnap((prev) => {
          if (prev.key !== currentKey) return prev;
          const items = target === 1 ? res.items : [...prev.items, ...res.items];
          // 翻页途中书库被删改时后端可能返回空页：把总数收敛到已加载数量，免得滚动反复取空页
          const total = res.items.length === 0 ? items.length : res.total;
          return { ...prev, items, total, pages: target, loading: false, error: null };
        });
      })
      .catch((err: unknown) => {
        if (!active) return;
        if (err instanceof ApiError && err.kind === "aborted") return;
        setSnap((prev) => (prev.key !== currentKey ? prev : { ...prev, loading: false, error: toApiError(err) }));
      });

    return () => {
      active = false;
      controller.abort();
    };
    // 关键词与页码决定取哪一页；loading 是「现在有一次待取的页」，nonce 是重试信号
  }, [snap.key, snap.pages, snap.loading, snap.nonce]);

  const loadMore = useCallback(() => {
    setSnap((prev) => {
      if (prev.loading || prev.error) return prev;
      if (prev.pages * PICKER_PAGE_SIZE >= prev.total) return prev;
      return { ...prev, loading: true };
    });
  }, []);

  const retry = useCallback(() => {
    setSnap((prev) => (prev.error ? { ...prev, loading: true, error: null, nonce: prev.nonce + 1 } : prev));
  }, []);

  return {
    items: snap.items,
    total: snap.total,
    loadingFirst: snap.loading && snap.pages === 0,
    loadingMore: snap.loading && snap.pages > 0,
    hasMore: snap.pages * PICKER_PAGE_SIZE < snap.total,
    error: snap.error,
    loadMore,
    retry,
  };
}
