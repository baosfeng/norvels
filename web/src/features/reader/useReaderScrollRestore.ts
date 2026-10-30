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
 * 把某一章的章首对到屏幕顶。
 * 连续流里可见章上方还留着上一章，scrollTop = 0 会停在上一章的开头——
 * 进章、翻章、书签兜底都必须按「这一章的 section」定位，而不是滚动容器的 0。
 */
export function scrollSurfaceToChapter(surface: HTMLElement | null, chapterId: number) {
  if (!surface) return;
  const node = document.querySelector<HTMLElement>(`[data-chapter-id="${chapterId}"]`);
  if (!node) {
    surface.scrollTop = 0;
    return;
  }
  surface.scrollTop += node.getBoundingClientRect().top - surface.getBoundingClientRect().top;
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
  consumeAdvance,
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
  /**
   * 这次换章是「滚过章界线」出来的（连续流把可见章换了）。
   * 位置是读者自己滚出来的，恢复逻辑必须什么都不做——既不能回章首，也不能跳回上次读到的段落。
   */
  consumeAdvance: () => boolean;
  onRestoreAnchor: (anchor: { chapterId: number; para: number }) => void;
}) {
  // 依赖数组不能放函数体（每次渲染都是新引用），用 ref 持有最新的消费函数
  const takeFlip = useRef(consumeFlip);
  const takeAdvance = useRef(consumeAdvance);
  useLayoutEffect(() => {
    takeFlip.current = consumeFlip;
    takeAdvance.current = consumeAdvance;
  }, [consumeFlip, consumeAdvance]);
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
      scrollSurfaceToChapter(document.querySelector<HTMLElement>(SURFACE_SELECTOR), chapterId);
      showToast("这本书签的位置变了，先带你到这一章开头");
    }
  }, [pendingJump, chapterDataId, chapterId]);

  const restoredChapter = useRef(0);
  // 进章落位用 layoutEffect：必须赶在第一次 IntersectionObserver 判定之前落位，
  // 否则「可见章」会按落位前的位置算成上一章（连续流里上一章就在上方）。
  useLayoutEffect(() => {
    if (chapterDataId === undefined || chapterDataId !== chapterId) return;
    if (restoredChapter.current === chapterId) return;
    if (progressLoading) return;
    restoredChapter.current = chapterId;
    // 连续流推进：读者滚过章末换到了下一章，正文位置就是他要的位置，别动
    if (takeAdvance.current()) return;
    if (pendingJump && pendingJump.chapterId === chapterId) return;
    const surface = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
    // 从书架 / 历史 / 搜索进来：按段锚点恢复上次读到的地方
    if (!takeFlip.current() && storedParaIndex > 0) {
      onRestoreAnchor({ chapterId, para: storedParaIndex });
      const node = document.getElementById(`para-${storedParaIndex}`);
      if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
      return;
    }
    // 翻章 / 新进章节：从这一章的章首接着读。
    // 容器内部滚动（H5 的 .reader-main 定高 overflow），window.scrollTo 对它无效——
    // 实测滚到 70% 翻章，新章直接停在 56%。这里滚的是容器，且按「本章 section」定位。
    scrollSurfaceToChapter(surface, chapterId);
  }, [chapterDataId, chapterId, pendingJump, storedParaIndex, progressLoading, onRestoreAnchor]);
}