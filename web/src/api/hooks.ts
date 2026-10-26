import { useCallback, useEffect } from "react";
import { api, type BookQuery, type RequestOptions } from "../api/client";
import type { ReadingSessionInput } from "./types";
import { useResource } from "../lib/useResource";
import { subscribeStatsRevision } from "../lib/statsRevision";

/** 各域取数 hook：组件只消费状态，不直接碰 api 模块。 */

/**
 * 全局统计（侧栏 / 驾驶舱）。库存量没有自己的查询参数，所以不靠 deps 失效：
 * 写操作成功后由 lib/statsRevision 广播，这里订阅并 reload。
 * 用 reload 而不是把版本号塞进 deps —— reload 只重跑请求，旧数字留在屏幕上，
 * 侧栏不会在每次保存后闪一下「—」。
 */
export function useStats() {
  const stats = useResource((signal: AbortSignal) => api.stats({ signal }), []);
  const { reload } = stats;
  useEffect(() => subscribeStatsRevision(reload), [reload]);
  return stats;
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

/** 分类树：书架筛选器与分类页共用；书变动后由书架自己 reload。 */
export function useCategories(opts?: { enabled?: boolean }) {
  return useResource((signal: AbortSignal) => api.categories({ signal }), [], opts);
}

export function useBook(id: number | null) {
  return useResource(
    (signal: AbortSignal) => api.book(id as number, { signal }),
    [id],
    { enabled: id !== null && Number.isFinite(id) },
  );
}

/** 本书书签（段落级，按时间倒序）；书签抽屉打开前后都用同一份，加/删后 reload 刷新。 */
export function useBookmarks(bookId: number | null) {
  return useResource(
    (signal: AbortSignal) => api.bookmarks(bookId as number, { signal }),
    [bookId],
    { enabled: bookId !== null && Number.isFinite(bookId) },
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

/**
 * 搜索取数：两栏共用一份结果，按同一 page / size 分页（翻页时两栏一起换）。
 * 后端要求关键词至少 2 个字符，不足时不发请求（避免把 400 当成错误态展示）。
 */
export function useSearch(q: string, type: "all" | "book" | "chapter" = "all", page = 1, size = 8) {
  const trimmed = q.trim();
  const enabled = trimmed.length >= 2;
  return useResource(
    (signal: AbortSignal) => api.search(trimmed, { type, page, size }, { signal }),
    [trimmed, type, page, size],
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

/** 自定义章节识别规则（全局，最多 20 条）。 */
export function useChapterRules() {
  return useResource((signal: AbortSignal) => api.getChapterRules({ signal }), []);
}

/** 词表规则（标签 ← 关键词）：导入时自动打标，只增不删。 */
export function useTagRules() {
  return useResource((signal: AbortSignal) => api.tagRules({ signal }), []);
}

/** 某任务的失败文件清单（重试入口用）；jobId 为空时不发请求。 */
export function useFailedFiles(jobId: number | null) {
  return useResource(
    (signal: AbortSignal) => api.importJobFiles(jobId as number, "failed", { signal }),
    [jobId],
    { enabled: jobId !== null && Number.isFinite(jobId) },
  );
}

/** 进度写入：失败不打断阅读，返回是否落库成功，供 UI 决定是否展示离线提示。 */
export function useSaveProgress() {
  return useCallback(
    async (
      body: { bookId: number; chapterId: number; percent: number; paraIndex?: number },
      options?: RequestOptions,
    ) => {
      try {
        await api.saveProgress(body, options);
        return true;
      } catch {
        return false;
      }
    },
    [],
  );
}

// --- P3 契约（docs/specs/2026-10-25-p3-contract.md）：发现域取数 + 阅读时长写入口 ---

/**
 * 随机一本（书架「换一本」）：每点一次 reload 换一本；params 变即重新取（key 进依赖）。
 * 没有符合条件的书时后端 404，这里照常是 error 态（api.randomBook 的注释），
 * 调用方按「没有符合条件的书」展示，不要当故障重试。
 */
export function useRandomBook(
  params: { authorId?: number; tag?: string; collection?: number } = {},
  options: { enabled?: boolean } = {},
) {
  const key = JSON.stringify(params);
  return useResource((signal: AbortSignal) => api.randomBook(params, { signal }), [key], options);
}

/** 系列列表（PC 专属页）：q 匹配系列名，翻页与书架同一套 page/size 口径。 */
export function useSeries(
  params: { q?: string; page?: number; size?: number },
  options: { enabled?: boolean } = {},
) {
  const key = JSON.stringify(params);
  return useResource((signal: AbortSignal) => api.seriesList(params, { signal }), [key], options);
}

/** 系列详情：name 为空 / 未定时不发请求（路由参数还没解析出来的那一帧）。 */
export function useSeriesDetail(name: string | null) {
  const enabled = !!name;
  return useResource(
    (signal: AbortSignal) => api.seriesDetail(name as string, { signal }),
    [name],
    { enabled },
  );
}

/** 书单列表：改名/增删成员后 reload 刷新（接口不回传聚合值）。 */
export function useCollections(options: { enabled?: boolean } = {}) {
  return useResource((signal: AbortSignal) => api.collections({ signal }), [], options);
}

/** 阅读历史（分页，按书聚合）；page/size 进依赖，翻页即重新取。 */
export function useReadingHistory(params: { page?: number; size?: number }) {
  const key = JSON.stringify(params);
  return useResource((signal: AbortSignal) => api.readingHistory(params, { signal }), [key]);
}

/** 阅读时长统计（今日/本周/累计/连续天数/近 N 天热力图）。 */
export function useReadingStats(days = 90) {
  return useResource((signal: AbortSignal) => api.readingStats(days, { signal }), [days]);
}

/** 上报一次阅读时长：失败不打断阅读（离线只丢这一次的计数），返回是否落库成功。 */
export function useRecordReadingSession() {
  return useCallback(async (body: ReadingSessionInput, options?: RequestOptions) => {
    try {
      await api.addReadingSession(body, options);
      return true;
    } catch {
      return false;
    }
  }, []);
}

/** 追更角标清零（读到最后一章）：失败只影响角标数字，不打断阅读。 */
export function useAckNewChapters() {
  return useCallback(async (bookId: number, options?: RequestOptions) => {
    try {
      await api.ackNewChapters(bookId, options);
      return true;
    } catch {
      return false;
    }
  }, []);
}