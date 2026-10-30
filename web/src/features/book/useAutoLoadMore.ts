import { useEffect, useRef } from "react";
import type { RefObject } from "react";
import { TOC_AUTO_LOAD_GAP } from "../../lib/tocRows";

interface AutoLoadOptions {
  /** 只在 H5 档位开启：PC 一次铺全，不需要续载 */
  enabled: boolean;
  /** 还有没铺出来的章节 */
  hasMore: boolean;
  /** 这一卷的列表容器：整页滚动时判定看它的底边 */
  rootRef: RefObject<HTMLDivElement | null>;
  /** 触底时补下一批 */
  onMore: () => void;
  /** 提前量（px）：列表底离视口底还剩这么多就开始补 */
  gap?: number;
}

/**
 * 滚到列表底自动续载（H5 目录）。
 *
 * 详情页目录不设内部滚动（整页滚），所以判定看「这一卷列表的底边离视口底还剩多少」，
 * 提前量默认 240px——下一批在手指还没抬起时就接上，不出现「滑到底再等加载」的停顿。
 * ChapterList 在行数超过 800 时退回 70vh 限高自滚（几千章的书），那种情况下滚动事件
 * 来自列表容器本身，判定改用容器自己的 scrollTop / clientHeight / scrollHeight。
 *
 * 一次性守卫：补完一批要重新「上膛」才允许再补（见 armed）。
 */
export function useAutoLoadMore({ enabled, hasMore, rootRef, onMore, gap = TOC_AUTO_LOAD_GAP }: AutoLoadOptions) {
  const moreRef = useRef(onMore);
  /**
   * 一次性守卫：列表重新离底够远（长高了，或用户滚开了）才「上膛」。
   * 少了它，一次触底会被连续滚动事件放大成连补好几批。
   */
  const armed = useRef(true);

  // 回调进 ref：滚动监听只装一次，不必随每次渲染重装
  useEffect(() => {
    moreRef.current = onMore;
  });

  useEffect(() => {
    if (!enabled || !hasMore) return;
    armed.current = true;
    const check = (event?: Event) => {
      const root = rootRef.current;
      if (!root) return;
      const near = nearBottom(root, innerScroller(root, event?.target ?? null), Math.max(0, gap));
      if (near === null) return;
      if (!near) {
        armed.current = true;
        return;
      }
      if (!armed.current) return;
      armed.current = false;
      moreRef.current();
    };
    const onScroll = (event: Event) => check(event);
    const onResize = () => check();
    window.addEventListener("scroll", onScroll, { passive: true, capture: true });
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("scroll", onScroll, { capture: true });
      window.removeEventListener("resize", onResize);
    };
  }, [enabled, hasMore, rootRef, gap]);
}

/** 滚动事件来自列表自己的滚动容器（ChapterList 行数超 800 时限高自滚）。 */
function innerScroller(root: HTMLElement, target: EventTarget | null): HTMLElement | null {
  if (!(target instanceof HTMLElement) || target === root || !root.contains(target)) return null;
  return target.scrollHeight > 0 ? target : null;
}

/** 滚到列表底了吗；量不出来（还没排版）返回 null，交给下一次滚动事件。 */
function nearBottom(root: HTMLElement, inner: HTMLElement | null, gap: number): boolean | null {
  if (inner) return inner.scrollTop + inner.clientHeight + gap >= inner.scrollHeight;
  const rect = root.getBoundingClientRect();
  if (rect.height <= 0) return null; // 还没排版：不猜
  // 列表整个滚到视口上方（用户在看别处）不算触底
  return rect.bottom > 0 && rect.bottom - window.innerHeight <= gap;
}
