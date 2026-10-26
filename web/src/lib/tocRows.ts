import type { ChapterBrief } from "../api/types";

/**
 * 目录的「视觉行」：一行放 N 章（N = 列数），章节按章序从左到右、再下一行。
 *
 * 为什么按行而不是按列排：目录是给人扫章号的，1,2,3 顺着读最省眼；
 * 按列排（先读完整列再读下一列）在长目录里会让人反复找「刚才看到哪」。
 *
 * 抽成纯函数的原因：虚拟滚动需要「总行数」和「第 i 行有哪些章」，
 * 这两件事必须是可单测的数学，而不是散在组件里的下标运算。
 */
export interface TocRow {
  /** 视觉行号（0 起） */
  index: number;
  items: ChapterBrief[];
}

/** 把章节切成视觉行；columns < 1 按 1 处理。 */
export function buildTocRows(chapters: readonly ChapterBrief[], columns: number): TocRow[] {
  const cols = Math.max(1, Math.trunc(columns) || 1);
  const rows: TocRow[] = [];
  for (let i = 0; i < chapters.length; i += cols) {
    rows.push({ index: rows.length, items: chapters.slice(i, i + cols) });
  }
  return rows;
}

/** 当前章所在的视觉行号；找不到（或没有当前章）返回 -1。 */
export function rowIndexOfChapter(
  chapters: readonly ChapterBrief[],
  columns: number,
  chapterId: number | undefined,
): number {
  if (!chapterId) return -1;
  const cols = Math.max(1, Math.trunc(columns) || 1);
  const at = chapters.findIndex((chapter) => chapter.id === chapterId);
  return at < 0 ? -1 : Math.floor(at / cols);
}

/** PC 列数：数字要和 CSS 断点一致，否则虚拟滚动的行高会和实际排版错位。 */
export const TOC_COLUMN_BREAKPOINTS = { three: 1180, two: 900 } as const;

export function columnsForWidth(width: number): number {
  if (width >= TOC_COLUMN_BREAKPOINTS.three) return 3;
  if (width >= TOC_COLUMN_BREAKPOINTS.two) return 2;
  return 1;
}

/**
 * 目录搜索：标题或章号命中即保留（大小写不敏感）。
 * 章号按「第 n 章」里的数字匹配，所以输入 37 也能命中「第 37 章 …」。
 */
export function filterChapters(chapters: readonly ChapterBrief[], query: string): ChapterBrief[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return [...chapters];
  return chapters.filter(
    (chapter) =>
      chapter.title.toLowerCase().includes(needle) ||
      String(chapter.idx + 1).includes(needle),
  );
}
/** 目录分块续显的步长：一次补 60 章（契约 C-29）。 */
export const TOC_CHUNK = 60;

/** 目录里的章节总数：卷内章节之和（卷为空也算 0）。 */
export function countChapters(volumes: readonly { chapters: ChapterBrief[] }[]): number {
  return volumes.reduce((sum, volume) => sum + volume.chapters.length, 0);
}

/** 当前已显示的前 n 章：按卷序、章序切出一段（不改原数组）。 */
export function takeRevealed<T>(items: readonly T[], revealed: number): T[] {
  return items.slice(0, Math.max(0, Math.trunc(revealed)));
}

/** 下一批要显示的章数：一次最多补 TOC_CHUNK 章。 */
export function nextReveal(revealed: number, total: number): number {
  return Math.min(total, Math.max(0, Math.trunc(revealed)) + TOC_CHUNK);
}
