import type { ChapterBrief } from "../../api/types";

/**
 * 目录搜索与分块（纯函数）。
 *
 * 两处口径一起放这里，是因为它们回答同一个问题：「目录这一屏到底显示哪些章」。
 * 搜索命中先给 TOC_CHUNK 条；H5 卷内先给 TOC_FIRST_BATCH 章，之后每次下滑补 TOC_CHUNK 章。
 */
export const TOC_CHUNK = 60;

/**
 * H5 首屏先给这么多章：手机上一屏目录就够，剩下的随下滑续载（不点按钮）。
 * PC 不用这个数——PC 一次铺全，虚拟滚动只画视口内的行。
 */
export const TOC_FIRST_BATCH = 20;

/** 首屏显示的章数（本卷不足时全给）。 */
export function firstBatch(total: number): number {
  return Math.min(TOC_FIRST_BATCH, Math.max(0, Math.trunc(total)));
}

export interface TocMatch {
  /** 这一屏要渲染的命中章节（最多 TOC_CHUNK 条） */
  chapters: ChapterBrief[];
  /** 命中总数（用于「找到 n 章」与计数位） */
  total: number;
}

/** 章节是否命中：章节名含关键词，或关键词就是章号（「128」= 第 128 章）。 */
export function matchesChapter(chapter: ChapterBrief, keyword: string): boolean {
  const q = keyword.trim();
  if (!q) return true;
  if (chapter.title.includes(q)) return true;
  return String(chapter.idx + 1) === q;
}

/** 本地过滤，无防抖；命中超过 60 条时只先给 60 条，避免一次铺出几千行。 */
export function searchToc(chapters: ChapterBrief[], keyword: string): TocMatch {
  const q = keyword.trim();
  if (!q) return { chapters: [], total: 0 };
  const hits = chapters.filter((chapter) => matchesChapter(chapter, q));
  return { chapters: hits.slice(0, TOC_CHUNK), total: hits.length };
}

/** 结果区那句「找到 35 章」；命中太多时说明只显示了前 60 条。 */
export function searchSummary(total: number): string {
  if (total <= TOC_CHUNK) return `找到 ${total} 章`;
  return `找到 ${total} 章，先显示前 ${TOC_CHUNK} 章`;
}

/** 搜索框右侧计数：默认「412 章」，搜索时「35 章匹配」。 */
export function tocCountLabel(total: number, match: number | null): string {
  if (match === null) return `${total} 章`;
  return `${match} 章匹配`;
}
