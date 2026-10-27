import { useEffect, useRef, useState } from "react";

/**
 * 章节正文的重试：点「重试」后等请求真的换过 data/error 才收状态，
 * 连续失败次数按章累计（换章清零），用于给「多次失败」的提示。
 */
export function useChapterRetry({
  chapter,
  chapterId,
}: {
  chapter: { data: unknown; error: unknown; reload: () => void };
  chapterId: number;
}) {
  const [retryingChapter, setRetryingChapter] = useState(false);
  const [chapterFailures, setChapterFailures] = useState(0);
  const chapterRetryBase = useRef<{ data: unknown; error: unknown }>({ data: null, error: null });

  function retryChapter() {
    if (retryingChapter) return;
    chapterRetryBase.current = { data: chapter.data, error: chapter.error };
    setRetryingChapter(true);
    chapter.reload();
  }

  useEffect(() => {
    if (!retryingChapter) return;
    const base = chapterRetryBase.current;
    if (chapter.data === base.data && chapter.error === base.error) return;
    // 请求完成后同步一次本地重试状态（外部系统的完成回调不在 React 事件里）
    setRetryingChapter(false);
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (chapter.error) setChapterFailures((count) => count + 1);
    else setChapterFailures(0);
  }, [retryingChapter, chapter.data, chapter.error]);

  useEffect(() => {
    // 换章即重置本章的重试计数（跨章状态不串）
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setChapterFailures(0);
    setRetryingChapter(false);
  }, [chapterId]);

  return { retryingChapter, chapterFailures, retryChapter };
}
