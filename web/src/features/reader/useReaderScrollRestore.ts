import { useEffect, useRef } from "react";
import { showToast } from "../../lib/toastStore";

type Jump = { chapterId: number; paraIndex: number } | null;

/**
 * 三类滚动定位：搜索命中滚到当前命中、书签跳转滚到目标段、
 * 首次进入章节恢复上次读到的段落（每章只恢复一次）。
 */
export function useReaderScrollRestore({
  searchOpen,
  currentHit,
  hitCount,
  chapterId,
  chapterDataId,
  pendingJump,
  storedParaIndex,
  progressLoading,
  onRestoreAnchor,
}: {
  searchOpen: boolean;
  currentHit: number;
  hitCount: number;
  chapterId: number;
  chapterDataId: number | undefined;
  pendingJump: Jump;
  storedParaIndex: number;
  progressLoading: boolean;
  onRestoreAnchor: (anchor: { chapterId: number; para: number }) => void;
}) {
  useEffect(() => {
    if (!searchOpen || hitCount === 0) return;
    const node = document.querySelector<HTMLElement>(`[data-hit="${currentHit}"]`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "center" });
  }, [searchOpen, currentHit, hitCount, chapterDataId]);

  useEffect(() => {
    if (!pendingJump || chapterDataId !== pendingJump.chapterId) return;
    const node = document.getElementById(`para-${pendingJump.paraIndex}`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
    else {
      window.scrollTo({ top: 0 });
      showToast("这本书签的位置变了，先带你到这一章开头");
    }
  }, [pendingJump, chapterDataId]);

  const restoredChapter = useRef(0);
  useEffect(() => {
    if (chapterDataId === undefined || chapterDataId !== chapterId) return;
    if (restoredChapter.current === chapterId) return;
    if (progressLoading) return;
    restoredChapter.current = chapterId;
    if (pendingJump && pendingJump.chapterId === chapterId) return;
    if (storedParaIndex <= 0) return;
    window.setTimeout(() => {
      onRestoreAnchor({ chapterId, para: storedParaIndex });
      const node = document.getElementById(`para-${storedParaIndex}`);
      if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
    }, 0);
  }, [chapterDataId, chapterId, pendingJump, storedParaIndex, progressLoading, onRestoreAnchor]);
}
