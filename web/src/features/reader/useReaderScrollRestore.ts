import { useEffect, useLayoutEffect, useRef } from "react";
import { showToast } from "../../lib/toastStore";

type Jump = { chapterId: number; paraIndex: number } | null;

/** 正文滚动容器（与 useReaderScrollPercent / useReaderRepositionAfterReflow 取的是同一个节点）。 */
const SURFACE_SELECTOR = '[data-testid="reader-swipe-area"]';

/** 回到章首：滚的是正文容器自己，不是窗口。 */
export function scrollSurfaceToTop(surface: HTMLElement | null) {
  if (!surface) return;
  surface.scrollTop = 0;
}

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
  consumeFlip,
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
  /**
   * 这次换章是不是翻章（滑动 / 点按 / 键盘 / 滑块 / 自动滚动）。
   * 判断权归产生动作的一方（goChapter 的唯一出口置位），恢复逻辑不猜方向——
   * 否则「1040→1041→目录点回 1040」会被当成翻章，把读者丢回章首。
   */
  consumeFlip: () => boolean;
  onRestoreAnchor: (anchor: { chapterId: number; para: number }) => void;
}) {
  // 依赖数组不能放函数体（每次渲染都是新引用），用 ref 持有最新的消费函数
  const takeFlip = useRef(consumeFlip);
  useLayoutEffect(() => {
    takeFlip.current = consumeFlip;
  }, [consumeFlip]);
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
      scrollSurfaceToTop(document.querySelector<HTMLElement>(SURFACE_SELECTOR));
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
    // 翻章：从章首接着读，别继承上一章的滚动位置。
    // 容器内部滚动（H5 的 .reader-main 定高 overflow），window.scrollTo 对它无效——
    // 实测滚到 70% 翻章，新章直接停在 56%。这里滚的是容器。
    if (takeFlip.current()) {
      window.setTimeout(() => {
        scrollSurfaceToTop(document.querySelector<HTMLElement>(SURFACE_SELECTOR));
      }, 0);
      return;
    }
    // 从书架 / 历史 / 搜索进来：仍按段锚点恢复上次读到的地方
    if (storedParaIndex <= 0) return;
    window.setTimeout(() => {
      onRestoreAnchor({ chapterId, para: storedParaIndex });
      const node = document.getElementById(`para-${storedParaIndex}`);
      if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
    }, 0);
  }, [chapterDataId, chapterId, pendingJump, storedParaIndex, progressLoading, onRestoreAnchor]);
}
