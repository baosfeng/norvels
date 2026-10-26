import type { ChapterBrief } from "../../api/types";

/**
 * 目录搜索与卷内分块（纯函数，契约 C-11 / C-12 / C-16）。
 *
 * 两处口径一起放这里，是因为它们回答同一个问题：「目录这一屏到底显示哪些章」。
 * 搜索命中先给 TOC_CHUNK 条，卷内也按 TOC_CHUNK 步进，两个数都是 60。
 */
export const TOC_CHUNK = 60;

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

/** 卷内按钮文案：一次补 60 行，剩余数同步减少。 */
export function moreChaptersLabel(remaining: number): string {
  const step = Math.min(TOC_CHUNK, remaining);
  return `在本卷再显示 ${step} 章（本卷还有 ${remaining} 章）`;
}

/** 卷内第一次渲染显示多少章（本卷不足 60 章时全铺开，按钮不出现）。 */
export function initialShown(total: number): number {
  return Math.min(TOC_CHUNK, total);
}
