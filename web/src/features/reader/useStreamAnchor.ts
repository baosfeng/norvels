import { useLayoutEffect, useRef } from "react";
import { contentOffset, nudgeScroll, readerSurface, scrollToChapterTop, type Surface } from "./readerStreamLayout";

/** 上一次提交时可见章在流里的位置与高度：补偿的基准。 */
interface Anchor {
  id: number;
  offset: number;
  height: number;
}

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
}

/**
 * 正文流的布局锚定，赶在绘制之前做完，读者看不到跳变。
 *
 * 两件事共用一次测量：
 * - 显式翻章（按钮 / 键盘 / 滑动 / 滑块）把目标章章首对到正文区顶；
 * - 其余每一次提交都做「上方内容增减」补偿：增多少就往 scrollTop 里补多少，
 *   读者眼前的正文一动不动。基准是上一次提交时可见章的位置与高度；换章了就用
 *   上一个可见章的上沿或下沿算当前章本该在哪——相邻两章的几何是同一帧量出来的，
 *   不用猜方向。锚点章已经不在流里（目录 / 书签跳远）就只重取基准，落点交给恢复逻辑。
 *
 * 不设依赖数组：每一次提交都要量。恢复提示条收起、上一章正文迟到都发生在
 * 「可见章没变」的提交里，漏量的那 80px 就足以让判定带翻到隔壁章、再被补偿推回来。
 */
export function useStreamAnchor({ activeId, pendingFlipRef, flipLandedRef, onFlipPlaced, flipFailed }: StreamAnchorOptions) {
  const anchorOffset = useRef<Anchor | null>(null);

  useLayoutEffect(() => {
    const surface = readerSurface();
    if (!surface) return;
    // 显式翻章：目标章一进 DOM 就把章首对到正文区顶
    const target = pendingFlipRef.current;
    if (target !== null) {
      if (scrollToChapterTop(surface, target)) {
        pendingFlipRef.current = null;
        flipLandedRef.current = true;
        onFlipPlaced();
        anchorOffset.current = measure(activeId);
        return;
      }
      if (flipFailed) pendingFlipRef.current = null;
      return;
    }
    const activeNode = document.querySelector<HTMLElement>(`[data-chapter-id="${activeId}"]`);
    if (!activeNode) {
      anchorOffset.current = null;
      return;
    }
    // 刚才那次翻章的提交不再补偿一次（会把读者推过章首）
    const justFlipped = flipLandedRef.current;
    flipLandedRef.current = false;
    if (!justFlipped) compensate(surface, activeNode, anchorOffset.current);
    anchorOffset.current = measure(activeId);
  });
}

function measure(activeId: number): Anchor | null {
  const node = document.querySelector<HTMLElement>(`[data-chapter-id="${activeId}"]`);
  if (!node) return null;
  return { id: activeId, offset: contentOffset(node), height: node.getBoundingClientRect().height };
}

/** 上方内容涨了多少就多滚多少、缩了多少就少滚多少。 */
function compensate(surface: Surface, activeNode: HTMLElement, anchor: Anchor | null): void {
  if (!anchor) return;
  if (anchor.id === Number(activeNode.dataset.chapterId)) {
    // 可见章没变：只有「它上方的内容」变了，差值就是要补的滚动量
    const growth = contentOffset(activeNode) - anchor.offset;
    if (Math.abs(growth) >= 0.5) nudgeScroll(surface, growth);
    return;
  }
  // 换章了：上一页的可见章通常还留在流里，与当前章相邻，
  // 用它的下沿（它在上）或上沿（它在下）算出当前章本该在哪
  const prevNode = document.querySelector<HTMLElement>(`[data-chapter-id="${anchor.id}"]`);
  if (!prevNode) return;
  const height = activeNode.getBoundingClientRect().height;
  const prevAbove = Boolean(prevNode.compareDocumentPosition(activeNode) & Node.DOCUMENT_POSITION_FOLLOWING);
  const base = prevAbove ? anchor.offset + anchor.height : anchor.offset - height;
  const growth = contentOffset(activeNode) - base;
  if (Math.abs(growth) >= 0.5) nudgeScroll(surface, growth);
}