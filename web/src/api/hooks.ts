import { useCallback } from "react";
import { api, type BookQuery, type RequestOptions } from "../api/client";
import { useResource } from "../lib/useResource";

/** 各域取数 hook：组件只消费状态，不直接碰 api 模块。 */

export function useStats() {
  return useResource((signal: AbortSignal) => api.stats({ signal }), []);
}

export function useHealthReport() {
  return useResource((signal: AbortSignal) => api.healthReport({ signal }), []);
}

export function useImportTrend(days: number) {
  return useResource((signal: AbortSignal) => api.importTrend(days, { signal }), [days]);
}

export function useBooks(params: BookQuery) {
  const key = JSON.stringify(params);
  return useResource(
    (signal: AbortSignal) => api.books(params, { signal }),
    [key],
  );
}

export function useBook(id: number | null) {
  return useResource(
    (signal: AbortSignal) => api.book(id as number, { signal }),
    [id],
    { enabled: id !== null && Number.isFinite(id) },
  );
}

export function useChapter(bookId: number | null, chapterId: number | null) {
  const enabled = bookId !== null && chapterId !== null;
  return useResource(
    (signal: AbortSignal) => api.chapter(bookId as number, chapterId as number, { signal }),
    [bookId, chapterId],
    { enabled },
  );
}

export function useAuthors(
  params: { q?: string; sort?: "books" | "words" | "name"; page?: number; size?: number },
  options: { enabled?: boolean } = {},
) {
  const key = JSON.stringify(params);
  return useResource((signal: AbortSignal) => api.authors(params, { signal }), [key], options);
}

export function useSearch(q: string, type: "all" | "book" | "chapter" = "all") {
  const trimmed = q.trim();
  // 后端要求关键词至少 2 个字符，不足时不发请求（避免把 400 当成错误态展示）
  const enabled = trimmed.length >= 2;
  return useResource(
    (signal: AbortSignal) => api.search(trimmed, { type }, { signal }),
    [trimmed, type],
    { enabled },
  );
}

export function useImportJobs() {
  return useResource((signal: AbortSignal) => api.importJobs({ signal }), []);
}

/** 驾驶舱用：最多取 50 条（接口上限），用于按书聚合阅读三态。 */
export function useReadingProgressAll() {
  return useResource((signal: AbortSignal) => api.readingProgress({ signal }, 50), []);
}

export function useReadingProgress() {
  return useResource((signal: AbortSignal) => api.readingProgress({ signal }), []);
}

/** 字数 Top N 作者（驾驶舱横条图）。 */
export function useTopAuthors(size = 10) {
  return useResource(
    (signal: AbortSignal) => api.authors({ sort: "words", size }, { signal }),
    [size],
  );
}

export function useTags(options: { enabled?: boolean } = {}) {
  return useResource((signal: AbortSignal) => api.tags({ signal }), [], options);
}

/** 进度写入：失败不打断阅读，返回是否落库成功，供 UI 决定是否展示离线提示。 */
export function useSaveProgress() {
  return useCallback(async (body: { bookId: number; chapterId: number; percent: number }, options?: RequestOptions) => {
    try {
      await api.saveProgress(body, options);
      return true;
    } catch {
      return false;
    }
  }, []);
}