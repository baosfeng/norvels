import { api } from "../../api/client";
import type { BookDetail, ChapterBrief } from "../../api/types";
import { showToast } from "../../lib/toastStore";

/**
 * 追更新章角标（books.newChapters）的清零时机：读到**整本书的最后一章**。
 * 追更链路（导入 --update）只累加、不清零，所以这里按「最后一章被打开」作为读到底的判据，
 * 优先用后端给的 lastChapterIdx（没有章节的书不带这个字段），缺省时按目录里最大的 idx 兜底。
 */
export function isLastChapter(book: BookDetail, chapter: ChapterBrief): boolean {
  if (typeof book.lastChapterIdx === "number" && book.lastChapterIdx >= 0) {
    return chapter.idx === book.lastChapterIdx;
  }
  const maxIdx = book.volumes.reduce(
    (max, volume) => volume.chapters.reduce((inner, item) => Math.max(inner, item.idx), max),
    -1,
  );
  return maxIdx >= 0 && chapter.idx === maxIdx;
}

/** 打开最后一章时清零角标；没有待清角标、或不是最后一章就什么都不做。 */
export async function ackNewChaptersIfLast(
  book: BookDetail,
  chapter: ChapterBrief,
  onCleared?: () => void,
): Promise<void> {
  if (!(book.newChapters && book.newChapters > 0)) return;
  if (!isLastChapter(book, chapter)) return;
  try {
    await api.ackNewChapters(book.id);
    onCleared?.();
  } catch {
    showToast("追更提示没能清零，下次再试。", "error");
  }
}
