/**
 * 连续章节流的滚动几何：正文容器（H5 自己滚）、整页滚（PC）、
 * 以及「上方内容增减多少，就把 scrollTop 挪多少」的补偿。
 *
 * 单独成文件：这些量在 useChapterStream 的布局效应与边缘监听里都要用，
 * 两处各写一份口径就会在容器可滚性变化时量出假的增减量。
 */

/** 正文滚动容器：H5 正文自己滚（.reader-main 定高 overflow），PC 由整页滚。 */
export interface Surface {
  el: HTMLElement;
  scrolls: boolean;
}

export const SURFACE_SELECTOR = '[data-testid="reader-swipe-area"]';

export function readerSurface(): Surface | null {
  const el = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
  if (!el) return null;
  return { el, scrolls: el.scrollHeight - el.clientHeight > 1 };
}

export function pageScrollTop(): number {
  return window.scrollY || document.documentElement.scrollTop || 0;
}

/**
 * 章节在正文内容里的位置：与滚动位置无关，只有「它上方的内容增减」才会让它变。
 *
 * 不写成「视口位置 + scrollTop」：正文容器的可滚性在挂载瞬间会反复变（内容还没铺满
 * 一屏时容器不可滚、由整页滚），容器坐标与窗口坐标混用会量出假的增减量，
 * 于是补一次、判定带翻一章、再补一次，读者被带着往上一路退。
 */
export function contentOffset(node: HTMLElement): number {
  const root = document.querySelector<HTMLElement>(".reader-stream") ?? node;
  return node.getBoundingClientRect().top - root.getBoundingClientRect().top;
}

/** 直接把滚动位置设到某个值（换章补偿用：浏览器可能已按新高度钳过一次，不能基于当前值累加）。 */
export function setScrollTop(surface: Surface, value: number): void {
  const next = Math.max(0, value);
  if (surface.scrolls) {
    surface.el.scrollTop = next;
    return;
  }
  window.scrollTo(0, next);
}

/** 把滚动位置挪 delta：上方涨了多少就多滚多少、缩了多少就少滚多少，读者眼前的正文不动。 */
export function nudgeScroll(surface: Surface, delta: number): void {
  if (surface.scrolls) {
    surface.el.scrollTop = Math.max(0, surface.el.scrollTop + delta);
    return;
  }
  window.scrollTo(0, Math.max(0, pageScrollTop() + delta));
}

/** 某一章在流里的位置与高度（位置与滚动无关，只有上方内容增减才会变）。 */
export interface ChapterAnchor {
  offset: number;
  height: number;
}

/** 量出流里每一章当前的位置与高度：换章前后的位移差用它算补偿量。 */
export function measureChapters(): Map<number, ChapterAnchor> {
  const map = new Map<number, ChapterAnchor>();
  document.querySelectorAll<HTMLElement>("[data-chapter-id]").forEach((node) => {
    const id = Number(node.dataset.chapterId);
    if (!Number.isFinite(id)) return;
    map.set(id, { offset: contentOffset(node), height: node.getBoundingClientRect().height });
  });
  return map;
}

/**
 * 把某一章的章首对到正文区顶：显式翻章（按钮 / 键盘 / 滑动 / 滑块）落在这里。
 *
 * 连续流里可见章上方还留着上一章，scrollTop = 0 停在的是上一章的开头——
 * 显式翻章必须按「这一章的 section」定位，否则读者点了「下一章」，屏幕还停在旧章里，
 * 判定带把可见章判回旧章，URL 立刻被 replace 回来：看起来就是「点了没反应」。
 * 返回是否真的落位（目标章还没渲染出来时为 false，调用方下个提交再试）。
 */
export function scrollToChapterTop(surface: Surface | null, chapterId: number): boolean {
  if (!surface) return false;
  const node = document.querySelector<HTMLElement>(`[data-chapter-id="${chapterId}"]`);
  if (!node) return false;
  const delta = node.getBoundingClientRect().top - surface.el.getBoundingClientRect().top;
  if (Math.abs(delta) < 0.5) return true;
  if (surface.scrolls) surface.el.scrollTop = Math.max(0, surface.el.scrollTop + delta);
  else window.scrollBy(0, delta);
  return true;
}

/** 当前滚动位置与可滚区间的三个量：容器可滚就用容器，否则用整页。 */
export function scrollMetrics(): { scrollTop: number; clientHeight: number; scrollHeight: number } {
  const surface = readerSurface();
  if (surface?.scrolls) {
    return {
      scrollTop: surface.el.scrollTop,
      clientHeight: surface.el.clientHeight,
      scrollHeight: surface.el.scrollHeight,
    };
  }
  const doc = document.documentElement;
  return {
    scrollTop: pageScrollTop(),
    clientHeight: window.innerHeight || 0,
    scrollHeight: doc.scrollHeight || 0,
  };
}
