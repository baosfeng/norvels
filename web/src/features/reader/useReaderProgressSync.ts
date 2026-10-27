import { useCallback, useEffect, useRef } from "react";
import type { ChapterBrief } from "../../api/types";

const PROGRESS_DEBOUNCE_MS = 1000;

type Anchor = { chapterId: number; para: number };
type SaveProgress = (input: { bookId: number; chapterId: number; percent: number; paraIndex: number }) => unknown;

/**
 * 阅读进度上报：章节变化立即落一次、滚动停下后 debounce 落、离开章节与切后台补齐。
 * 只负责「什么时候把位置送出去」，位置本身（anchor）由页面持有。
 */
export function useReaderProgressSync({
  bookId,
  chapterId,
  chapters,
  index,
  chapterDataId,
  storedParaIndex,
  anchor,
  saveProgress,
}: {
  bookId: number;
  chapterId: number;
  chapters: ChapterBrief[];
  index: number;
  chapterDataId: number | undefined;
  storedParaIndex: number;
  anchor: Anchor;
  saveProgress: SaveProgress;
}) {
  const lastSent = useRef({ chapterId: 0, para: -1 });
  const anchorRef = useRef(anchor);
  useEffect(() => {
    anchorRef.current = anchor;
  });

  const sendProgress = useCallback(
    (targetChapter: number, paraIndex: number) => {
      if (!Number.isFinite(bookId)) return;
      const at = chapters.findIndex((item) => item.id === targetChapter);
      if (at < 0) return;
      lastSent.current = { chapterId: targetChapter, para: paraIndex };
      void saveProgress({ bookId, chapterId: targetChapter, percent: (at + 1) / chapters.length, paraIndex });
    },
    [bookId, chapters, saveProgress],
  );

  useEffect(() => {
    if (!Number.isFinite(bookId) || !Number.isFinite(chapterId)) return;
    if (chapters.length === 0 || index < 0) return;
    if (chapterDataId !== chapterId) return;
    if (lastSent.current.chapterId === chapterId && lastSent.current.para === storedParaIndex) return;
    sendProgress(chapterId, storedParaIndex);
  }, [bookId, chapterId, chapterDataId, chapters.length, index, storedParaIndex, sendProgress]);

  useEffect(() => {
    if (anchor.chapterId !== chapterId || !Number.isFinite(bookId)) return;
    const para = anchor.para;
    const timer = window.setTimeout(() => {
      if (lastSent.current.chapterId === chapterId && lastSent.current.para === para) return;
      sendProgress(chapterId, para);
    }, PROGRESS_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [anchor, chapterId, bookId, sendProgress]);

  useEffect(() => {
    const watching = chapterId;
    return () => {
      const latest = anchorRef.current;
      if (latest.chapterId !== watching) return;
      if (lastSent.current.chapterId === latest.chapterId && lastSent.current.para === latest.para) return;
      sendProgress(latest.chapterId, latest.para);
    };
  }, [chapterId, sendProgress]);

  useEffect(() => {
    const onHide = () => {
      if (document.visibilityState !== "hidden") return;
      const latest = anchorRef.current;
      if (!latest.chapterId) return;
      if (lastSent.current.chapterId === latest.chapterId && lastSent.current.para === latest.para) return;
      sendProgress(latest.chapterId, latest.para);
    };
    document.addEventListener("visibilitychange", onHide);
    return () => document.removeEventListener("visibilitychange", onHide);
  }, [sendProgress]);

  return { sendProgress };
}
