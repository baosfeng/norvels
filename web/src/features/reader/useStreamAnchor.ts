import { useLayoutEffect, useRef } from "react";
import {
  measureChapters,
  nudgeScroll,
  readerSurface,
  scrollToChapterTop,
  setScrollTop,
  type ChapterAnchor,
} from "./readerStreamLayout";

/** 一次布局要处置的两件事：显式翻章落位、上方内容增减补偿。 */
export interface StreamAnchorOptions {
  activeId: number;
  /** 显式翻章的目标章；落位后由本 hook 置空。 */
  pendingFlipRef: { current: number | null };
  /** 刚由显式翻章落位：这一提交里的补偿让路。 */
  flipLandedRef: { current: boolean };
  /** 落位成功后回调（调用方重置判定冷却）。 */
  onFlipPlaced: () => void;
  /** 目标章打不开，别再等落位（否则判定带会被永久冻住）。 */
  flipFailed: boolean;
  /** 每一章在流里的位置：换章前由调用方刷新，提交后拿同一批章节的位置对比。 */
  snapshotRef: { current: Map<number, ChapterAnchor> };
  /** 换章前的滚动位置：见下方 setScrollTop 的说明。 */
  scrollTopBeforeRef: { current: number | null };
}

/**
 * 正文流的布局锚定，赶在绘制之前做完，读者看不到跳变。
 *
 * 两件事共用一次测量：
 * - 显式翻章（按钮 / 键盘 / 滑动 / 滑块）把目标章章首对到正文区顶；
 * - 其余提交做「上方内容增减」补偿，读者眼前的正文一动不动。
 *
 * 三条规则，各自挡住一个实测过的坑：
 * 1. 补偿量按「同一章在提交前后的 offset 差」算，不用相邻章几何推算——
 *    滚动跨章时同一提交里会移除最上面一章，几何基准一旦被移除就量不出真实增减量
 *    （实测会少补约 1900px，读者看到的就是「往上顶一下」）。
 * 2. 换章那一刻，新章可能刚进窗口（提交前还没有位置可对比），这时改用**上一个可见章**
 *    的 offset 差：它一定还在窗口里，它上方的增减就是读者眼前内容的位移。
 * 3. 移除一章会让流先变矮，浏览器按新高度把 scrollTop 钳一次；此时「在当前值上累加」
 *    会少补被钳掉的那段。换章前记下滚动位置，补偿时按「提交前位置 + 位移差」直接设值。
 *
 * 不设依赖数组：每一次提交都要量。恢复提示条收起、上一章正文迟到都发生在
 * 「可见章没变」的提交里，漏量的那 80px 就足以让判定带翻到隔壁章、再被补偿推回来。
 */
export function useStreamAnchor({
  activeId,
  pendingFlipRef,
  flipLandedRef,
  onFlipPlaced,
  flipFailed,
  snapshotRef,
  scrollTopBeforeRef,
}: StreamAnchorOptions) {
  /** 上一次提交时的可见章：换章提交里它通常还留在窗口，位移差可以从它身上读。 */
  const lastActive = useRef(activeId);

  useLayoutEffect(() => {
    const surface = readerSurface();
    if (!surface) return;
    // 显式翻章：目标章一进 DOM 就把章首对到正文区顶
    const target = pendingFlipRef.current;
    if (target !== null) {
      if (scrollToChapterTop(surface, target)) {
        pendingFlipRef.current = null;
        flipLandedRef.current = true;
        scrollTopBeforeRef.current = null;
        lastActive.current = activeId;
        onFlipPlaced();
        snapshotRef.current = measureChapters();
        return;
      }
      if (flipFailed) pendingFlipRef.current = null;
      return;
    }
    const now = measureChapters();
    // 刚才那次翻章的提交不再补偿一次（会把读者推过章首）
    const justFlipped = flipLandedRef.current;
    flipLandedRef.current = false;
    // 只有「可见章变了」的提交才用换章基准；别的提交（窗口扩展、恢复提示收起）
    // 不该把这份基准消费掉，否则真正换章那一拍拿不到提交前的位置。
    const flipping = activeId !== lastActive.current;
    const growth = pickGrowth(snapshotRef.current, now, activeId, lastActive.current);
    lastActive.current = activeId;
    snapshotRef.current = now;
    const base = flipping ? scrollTopBeforeRef.current : null;
    if (flipping) scrollTopBeforeRef.current = null;
    if (justFlipped || growth === null) return;
    if (Math.abs(growth) < 0.5) return;
    if (base !== null) setScrollTop(surface, base + growth);
    else nudgeScroll(surface, growth);
  });
}

/** 补偿量：优先用可见章自己的位移；换章时新章可能没有旧位置，退回上一个可见章。 */
function pickGrowth(
  before: Map<number, ChapterAnchor>,
  after: Map<number, ChapterAnchor>,
  activeId: number,
  prevActiveId: number,
): number | null {
  const candidates = activeId === prevActiveId ? [activeId] : [activeId, prevActiveId];
  for (const id of candidates) {
    const was = before.get(id);
    const is = after.get(id);
    if (was && is) return is.offset - was.offset;
  }
  return null;
}
