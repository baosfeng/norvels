import type { Bookmark } from "../api/types";

/** 书签默认标签：该段前 30 字，去空白（空格 / 换行 / 全角空格一律压掉）。 */
export function bookmarkLabel(text: string, max = 30): string {
  return text.replace(/\s+/g, "").slice(0, max);
}

/** 同一章同一段是否已有书签：后端 POST 幂等，前端据此不重复发请求。 */
export function findBookmarkAt(list: Bookmark[], chapterId: number, paraIndex: number): Bookmark | null {
  return list.find((item) => item.chapterId === chapterId && item.paraIndex === paraIndex) ?? null;
}

/** 抽屉里的一行标题：label 留空时退回「第 N 段」。 */
export function bookmarkTitle(bookmark: Bookmark): string {
  const label = bookmark.label.trim();
  return label || `第 ${bookmark.paraIndex + 1} 段`;
}

/** 书签入参校验：章与段都必须是非负整数，否则不发请求（fail-closed）。 */
export function isValidPosition(chapterId: number, paraIndex: number): boolean {
  return Number.isInteger(chapterId) && chapterId > 0 && Number.isInteger(paraIndex) && paraIndex >= 0;
}
/** 删除确认框正文：写明对象名与「不可恢复」，对象名优先取章节标题（契约 C-58）。 */
export function bookmarkConfirmBody(bookmark: Bookmark): string {
  const title = bookmark.chapterTitle?.trim() || `第 ${bookmark.chapterIdx + 1} 章`;
  return `删除后立即移除，不可恢复。这一条是「${title}」。`;
}
