import { useEffect, useLayoutEffect, useRef } from "react";
import { showToast } from "../../lib/toastStore";

type Jump = { chapterId: number; paraIndex: number } | null;

/** 正文滚动容器（与 useReaderScrollPercent / useReaderRepositionAfterReflow 取的是同一个节点）。 */
const SURFACE_SELECTOR = '[data-testid="reader-swipe-area"]';

/**
 * 分页模式最后一次落位 = 当前页**首段**。
 *
 * 分页的位置真值是页码，滚动的位置真值是段落；两种模式之间只有「段」能对得上。
 * 分页模式把每次落位记在这里（见 ReaderPagedBody），切回滚动时由本 hook 消费：
 * 否则滚动恢复整条路径在分页期间是关着的（enabled=false），切回来只会走「本章已恢复过」
 * 的分支，读者看到的就是「第 11 页 → 章首」。
 */
let pagedAnchor: { chapterId: number; para: number } | null = null;

/** 分页正文翻页 / 落位时记下这一页首段：切模式时滚动恢复按它落位。 */
export function rememberPagedPara(chapterId: number, para: number) {
  if (!Number.isFinite(chapterId) || !Number.isFinite(para) || para < 0) return;
  pagedAnchor = { chapterId, para: Math.floor(para) };
}

/** 清掉记忆：换书 / 用例之间用，避免上一轮的锚点落到新章上。 */
export function clearPagedPara() {
  pagedAnchor = null;
}

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
  enabled = true,
}: {
  /** 分页模式下位置由页码游标负责：滚动恢复整条路径让位。 */
  enabled?: boolean;
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
    if (!enabled || !searchOpen || hitCount === 0) return;
    const node = document.querySelector<HTMLElement>(`[data-hit="${currentHit}"]`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "center" });
  }, [enabled, searchOpen, currentHit, hitCount, chapterDataId]);

  useEffect(() => {
    if (!enabled || !pendingJump || chapterDataId !== pendingJump.chapterId) return;
    const node = document.getElementById(`para-${pendingJump.paraIndex}`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
    else {
      scrollSurfaceToChapter(document.querySelector<HTMLElement>(SURFACE_SELECTOR), chapterId);
      showToast("这本书签的位置变了，先带你到这一章开头");
    }
  }, [enabled, pendingJump, chapterDataId, chapterId]);

  const restoredChapter = useRef(0);
  /** 上一次的开关状态：只有「分页 → 滚动」这一下才消费分页锚点。 */
  const wasEnabled = useRef(enabled);
  // 进章落位用 layoutEffect：必须赶在第一次 IntersectionObserver 判定之前落位，
  // 否则「可见章」会按落位前的位置算成上一章（连续流里上一章就在上方）。
  useLayoutEffect(() => {
    const switchedFromPaged = !wasEnabled.current && enabled;
    wasEnabled.current = enabled;
    if (!enabled) return;
    if (chapterDataId === undefined || chapterDataId !== chapterId) return;
    // 从分页切回来：滚到分页时那一页的首段，而不是「本章已恢复过 → 什么都不做」
    if (switchedFromPaged) {
      restoreFromPaged(chapterId, onRestoreAnchor);
      return;
    }
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
  }, [enabled, chapterDataId, chapterId, pendingJump, storedParaIndex, progressLoading, onRestoreAnchor]);
}

/**
 * 从分页切回滚动：落到分页时看到的那一页的**首段**，而不是章首。
 *
 * 分页期间本 hook 的恢复路径是关着的（enabled=false），切回来时这一章早已「恢复过」，
 * 旧实现直接 return —— 读者看到分页的第 11 页变成滚动模式的章首。
 */
function restoreFromPaged(chapterId: number, onRestoreAnchor: (anchor: { chapterId: number; para: number }) => void) {
  const left = pagedAnchor;
  if (!left || left.chapterId !== chapterId) return;
  pagedAnchor = null;
  const node = document.getElementById(`para-${left.para}`);
  if (node && typeof node.scrollIntoView === "function") {
    node.scrollIntoView({ block: "start" });
    onRestoreAnchor({ chapterId, para: left.para });
    return;
  }
  // 段号来自上一轮排版（段落被重新切过）：退回章首，至少不把读者丢到别处
  scrollSurfaceToChapter(document.querySelector<HTMLElement>(SURFACE_SELECTOR), chapterId);
}