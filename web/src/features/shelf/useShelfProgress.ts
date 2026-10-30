import { useMemo } from "react";
import { api } from "../../api/client";
import type { ReadingProgress } from "../../api/types";
import { useResource } from "../../lib/useResource";

/** 书架卡的阅读进度：与「继续阅读」同一份数据源（最近在读取 50 条），一次取回按 bookId 建索引。 */
const PROGRESS_LIMIT = 50;

/**
 * H5 书卡要显示「读到哪」，数据来自 GET /api/reading/progress。
 * 只在 H5 启用：PC 有「继续阅读」卡，再为每张卡拉一遍进度是多余请求。
 * 读不到（未登录 / 接口失败）就当没有进度，卡片不显示进度行，不阻塞列表。
 */
export function useShelfProgress(enabled: boolean): Map<number, ReadingProgress> {
  const { data } = useResource(
    (signal: AbortSignal) => api.readingProgress({ signal }, PROGRESS_LIMIT),
    [],
    { enabled },
  );
  return useMemo(() => {
    const map = new Map<number, ReadingProgress>();
    for (const item of data?.items ?? []) map.set(item.bookId, item);
    return map;
  }, [data]);
}
