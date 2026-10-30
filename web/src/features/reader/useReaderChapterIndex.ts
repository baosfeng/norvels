import { useMemo } from "react";
import type { BookDetail, ChapterBrief, Volume } from "../../api/types";

/**
 * 把书籍详情摊平成阅读器需要的「平铺章节 + 当前下标 + 前后章 + 全书百分比」。
 * 摊平与排序只在这里做一次：顶栏进度、进度滑块、章节切换、进度上报读的是同一份。
 */
export function useReaderChapterIndex(book: BookDetail | null, chapterId: number) {
  const chapters = useMemo<ChapterBrief[]>(
    () => (book?.volumes ?? []).flatMap((volume) => volume.chapters).sort((a, b) => a.idx - b.idx),
    [book],
  );
  const volumes = useMemo<Volume[]>(() => book?.volumes ?? [], [book]);
  const index = chapters.findIndex((chapter) => chapter.id === chapterId);
  const prev = index > 0 ? chapters[index - 1] : null;
  const next = index >= 0 && index < chapters.length - 1 ? chapters[index + 1] : null;
  const percent = chapters.length > 0 && index >= 0 ? (index + 1) / chapters.length : 0;
  return { chapters, volumes, index, prev, next, percent };
}
