import { api } from "../../api/client";
import type { ReadingProgress } from "../../api/types";
import { useResource } from "../../lib/useResource";

/**
 * 章内位置记忆的取数：后端 /reading/progress 只按「书」保留一条最后位置，
 * 这里取最近 50 条（接口上限）就够认出「这本书上次读到哪一段」。
 */
export const PROGRESS_LOOKBACK = 50;

/**
 * 从进度列表里挑出「本书 + 本章」的段落下标。
 * 进度行记的是上一次读的那一章：章节对不上时返回 0（不要拿别的章的下标乱滚）。
 */
export function pickStoredParaIndex(items: ReadingProgress[], bookId: number, chapterId: number): number {
  const row = items.find((item) => item.bookId === bookId);
  if (!row || row.chapterId !== chapterId) return 0;
  const para = row.paraIndex ?? 0;
  return Number.isFinite(para) && para > 0 ? Math.floor(para) : 0;
}

/** 读者上次读到哪一段：进章节时用它把正文滚回原处。 */
export function useReaderProgress(bookId: number) {
  return useResource(
    (signal: AbortSignal) => api.readingProgress({ signal }, PROGRESS_LOOKBACK),
    [bookId],
    { enabled: Number.isFinite(bookId) },
  );
}
