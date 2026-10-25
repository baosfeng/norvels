import type { ChapterBrief, Volume } from "../api/types";

/** 章节批量操作（合并 / 批量删除 / 按序号重命名）的纯逻辑：组件只做编排。 */

/** 可参与批量操作的章节最小形状：一行数据只要这几个字段。 */
export type ChapterLike = Pick<ChapterBrief, "id" | "idx" | "title" | "volumeId" | "wordCount">;

/** 勾选章节的区间摘要：删除确认框的「将删除 N 章（第 x–y 章，共 M 字）」用它算。 */
export interface ChapterSpan {
  count: number;
  /** 1 起的章号（最小 idx + 1） */
  first: number;
  /** 1 起的章号（最大 idx + 1） */
  last: number;
  words: number;
}

/** 合并结论：ok 为假时 reason 是可以直接展示给用户的原因。 */
export interface MergeVerdict {
  ok: boolean;
  reason: string;
}

/** 合并的三条硬规则：≥2 章 · 同一卷 · 章号相邻连续。 */
export const MERGE_RULE_TEXT = "只能合并同一卷里连续的章节。";
export const MERGE_NEED_TWO_TEXT = "至少要选 2 章才能合并。";

/** 按章号（idx 是全书 0 起的序号）排序，跨卷也是单调的。 */
export function sortByChapter<T extends ChapterLike>(list: T[]): T[] {
  return [...list].sort((a, b) => a.idx - b.idx);
}

/** 区间与字数摘要；空列表返回全 0（调用方据此不渲染）。 */
export function spanOf(list: ChapterLike[]): ChapterSpan {
  if (list.length === 0) return { count: 0, first: 0, last: 0, words: 0 };
  let first = Number.POSITIVE_INFINITY;
  let last = 0;
  let words = 0;
  for (const chapter of list) {
    const no = chapter.idx + 1;
    if (no < first) first = no;
    if (no > last) last = no;
    words += Math.max(0, chapter.wordCount || 0);
  }
  return { count: list.length, first, last, words };
}

/** 合并前先问一次规则：不满足就禁用按钮并说明原因，别等点了才报错。 */
export function mergeVerdict(list: ChapterLike[]): MergeVerdict {
  if (list.length === 0) return { ok: false, reason: "" };
  if (list.length < 2) return { ok: false, reason: MERGE_NEED_TWO_TEXT };
  const sorted = sortByChapter(list);
  const volumeId = sorted[0].volumeId;
  for (let i = 0; i < sorted.length; i += 1) {
    if (sorted[i].volumeId !== volumeId) return { ok: false, reason: MERGE_RULE_TEXT };
    if (i > 0 && sorted[i].idx !== sorted[i - 1].idx + 1) return { ok: false, reason: MERGE_RULE_TEXT };
  }
  return { ok: true, reason: "" };
}

/**
 * 去掉标题里已有的章号前缀：「第十二章」「第 12 章」「第12回」「第 3 节」以及后随的
 * 、/．/：/空格都算前缀。剥完什么都不剩（标题只有一个章号）时返回空串，由调用方补章号。
 */
const PREFIX = /^第\s*[0-9０-９一二三四五六七八九十百千零〇两]+\s*[章回节]\s*[、，,．.。:：·\-—\s]*/;

export function stripChapterPrefix(title: string): string {
  let text = (title ?? "").trim();
  // 最多剥 3 层：「第十二章 第 12 章 巷口的灯」这种连环前缀也收拾干净
  for (let i = 0; i < 3; i += 1) {
    const next = text.replace(PREFIX, "").trim();
    if (next === text) break;
    text = next;
  }
  return text;
}

/** 按序号重命名后的标题：第 N 章 + 去前缀后的原标题，N = idx + 1。 */
export function renamedTitle(title: string, n: number): string {
  const base = stripChapterPrefix(title);
  return base ? `第 ${n} 章 ${base}` : `第 ${n} 章`;
}

/** 预览的一行：改前 → 改后。 */
export interface RenameRow {
  chapterId: number;
  before: string;
  after: string;
}

/** 重命名范围：全书，或某一卷。 */
export type RenumberScope = { kind: "book" } | { kind: "volume"; volumeId: number };

/** 按范围取出要改名的章（已按章号排序）。 */
export function chaptersInScope(volumes: Volume[], scope: RenumberScope): ChapterBrief[] {
  const out: ChapterBrief[] = [];
  for (const volume of [...volumes].sort((a, b) => a.idx - b.idx)) {
    if (scope.kind === "volume" && volume.id !== scope.volumeId) continue;
    out.push(...volume.chapters);
  }
  return sortByChapter(out);
}

/** 改名计划：预览与「本次改 N 章」用同一份，避免两处口径漂移。 */
export function renumberPlan(chapters: ChapterLike[]): RenameRow[] {
  return sortByChapter(chapters).map((chapter) => ({
    chapterId: chapter.id,
    before: chapter.title,
    after: renamedTitle(chapter.title, chapter.idx + 1),
  }));
}

/** 每卷默认渲染的章数：6000 章的书不能一次铺满 DOM。 */
export const CHAPTER_PAGE = 20;

/** 当前要渲染哪些行：搜索时只留命中的（不受分页限制），否则取前 shown 条。 */
export function rowsOf(volume: Volume, query: string, shown: number): ChapterBrief[] {
  if (query) {
    return volume.chapters.filter(
      (chapter) => chapter.title.toLowerCase().includes(query) || String(chapter.idx + 1).includes(query),
    );
  }
  return volume.chapters.slice(0, shown);
}

/** 卷名：未分卷的书 idx 为负；没写卷名就按序号叫「第 N 卷」。 */
export function volumeLabel(volume: Volume): string {
  if (volume.idx < 0) return "未分卷";
  return volume.title || `第 ${volume.idx + 1} 卷`;
}
