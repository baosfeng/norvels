import { useEffect, useMemo } from "react";
import { api } from "../../api/client";
import type { ReadingProgress } from "../../api/types";
import { subscribeReadingRevision } from "../../lib/readingRevision";
import { useResource } from "../../lib/useResource";

/** 书架卡的阅读进度：与「继续阅读」同一份数据源（最近在读取 50 条），一次取回按 bookId 建索引。 */
const PROGRESS_LIMIT = 50;

/**
 * H5 书卡要显示「读到哪」，数据来自 GET /api/reading/progress。
 * 只在 H5 启用：PC 有「继续阅读」卡，再为每张卡拉一遍进度是多余请求。
 * 读不到（未登录 / 接口失败）就当没有进度，卡片不显示进度行，不阻塞列表。
 *
 * 订阅 lib/readingRevision：从阅读器返回书架时，卡片上的「已读 N · 第 M 章」
 * 要跟上刚读到的地方，否则同一屏里「继续阅读」是新章、书卡还是旧章。
 */
export function useShelfProgress(enabled: boolean): Map<number, ReadingProgress> {
  const resource = useResource(
    (signal: AbortSignal) => api.readingProgress({ signal }, PROGRESS_LIMIT),
    [],
    { enabled },
  );
  const { data, reload } = resource;
  useEffect(() => subscribeReadingRevision(reload), [reload]);
  return useMemo(() => {
    const map = new Map<number, ReadingProgress>();
    for (const item of data?.items ?? []) map.set(item.bookId, item);
    return map;
  }, [data]);
}
