import { useEffect, useState } from "react";

/** 滚动停手后浮层淡出的延迟（ms）：需求口径 1.2 秒，不再更短。 */
export const PROGRESS_HIDE_MS = 1200;

/** 正文滚动容器：阅读页只有一个正文手势层（与 dismissHint 取的是同一个节点）。 */
const SURFACE_SELECTOR = '[data-testid="reader-swipe-area"]';

export interface ScrollMetrics {
  scrollTop: number;
  clientHeight: number;
  scrollHeight: number;
}

/**
 * 已读比例 = 已滚距离 / 可滚距离。
 * 内容不足一屏（可滚距离 ≤ 0）时按 0 处理：读者已经在章末，条子不该显示成半截。
 */
export function scrollPercentOf(metrics: ScrollMetrics): number {
  const range = metrics.scrollHeight - metrics.clientHeight;
  if (!(range > 0)) return 0;
  const ratio = metrics.scrollTop / range;
  if (!Number.isFinite(ratio)) return 0;
  return Math.min(1, Math.max(0, ratio));
}

/**
 * 取当前生效的滚动指标：H5 正文自己滚（.reader-main 定高 overflow），
 * PC 正文不内部滚、由整页滚（窗口），所以容器滚不动时退回窗口的指标。
 */
export function activePercent(surface: ScrollMetrics, page: ScrollMetrics): number {
  return surface.scrollHeight - surface.clientHeight > 1 ? scrollPercentOf(surface) : scrollPercentOf(page);
}

function pageMetrics(): ScrollMetrics {
  const root = document.documentElement;
  return {
    scrollTop: window.scrollY || root.scrollTop || 0,
    clientHeight: window.innerHeight || root.clientHeight || 0,
    scrollHeight: root.scrollHeight || 0,
  };
}

/** 可见章那一段在正文里的位置与高度；量不到（未布局 / 不在连续流里）时为 null。 */
function chapterBox(chapterId: number, scrolls: boolean, surface: HTMLElement) {
  const node = document.querySelector<HTMLElement>(`[data-chapter-id="${chapterId}"]`);
  if (!node) return null;
  const rect = node.getBoundingClientRect();
  if (!(rect.height > 0)) return null;
  const top = scrolls
    ? rect.top - surface.getBoundingClientRect().top + surface.scrollTop
    : rect.top + (window.scrollY || 0);
  return { top, height: rect.height };
}

/**
 * 「本章」已读比例：连续流里同屏压着好几章，整条流的比例没有意义，只算可见章那一段。
 * 回到章首是 0、读到章末是 1，滚过章界线换章后条子跟着从头开始。
 */
export function chapterPercentOf(chapterId: number, surface: HTMLElement, page: ScrollMetrics): number | null {
  const scrolls = surface.scrollHeight - surface.clientHeight > 1;
  const box = chapterBox(chapterId, scrolls, surface);
  if (!box) return null;
  const scrollTop = scrolls ? surface.scrollTop : page.scrollTop;
  const view = scrolls ? surface.clientHeight : page.clientHeight;
  const range = box.height - view;
  if (!(range > 0)) return 0;
  const ratio = (scrollTop - box.top) / range;
  if (!Number.isFinite(ratio)) return 0;
  return Math.min(1, Math.max(0, ratio));
}

export interface ReaderScrollProgressState {
  /** 本章已读比例 0~1。 */
  percent: number;
  /** 是否正在滚动：停手 {@link PROGRESS_HIDE_MS} 后转 false。 */
  scrolling: boolean;
}

/**
 * 跟踪正文的滚动比例。滚动事件按 rAF 合并（一次绘制最多算一次），
 * 浮层的显隐由一个 1.2s 定时器收口——定时器不随事件重建，也不再随卸载清理：
 * 换章时上一次的定时器自然到期收起浮层，不会把「正在滚动」留在新章上。
 */
export function useReaderScrollPercent(resetKey: number, enabled = true): ReaderScrollProgressState {
  const [percent, setPercent] = useState(0);
  const [scrolling, setScrolling] = useState(false);

  useEffect(() => {
    // 分页模式下位置真值是页码：这里不读滚动，浮标的显隐由页码变化驱动
    if (!enabled) return;
    const surface = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
    if (!surface) return;

    let frame: number | null = null;
    let hideTimer: number | null = null;
    let last = -1;

    const apply = () => {
      frame = null;
      const page = pageMetrics();
      const next = chapterPercentOf(resetKey, surface, page) ?? activePercent(surface, page);
      if (Math.abs(next - last) < 0.001) return;
      last = next;
      setPercent(next);
    };

    const onScroll = () => {
      if (frame === null) frame = requestAnimationFrame(apply);
      setScrolling(true);
      if (hideTimer !== null) window.clearTimeout(hideTimer);
      hideTimer = window.setTimeout(() => {
        hideTimer = null;
        setScrolling(false);
      }, PROGRESS_HIDE_MS);
    };

    // H5 滚正文容器、PC 滚窗口：两边都听，比值由 activePercent 决定听谁的
    surface.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("scroll", onScroll, { passive: true });
    // 换章后正文回到章首：挂上先量一次，别让上一章的百分比留在条上
    frame = requestAnimationFrame(apply);

    return () => {
      surface.removeEventListener("scroll", onScroll);
      window.removeEventListener("scroll", onScroll);
      if (frame !== null) cancelAnimationFrame(frame);
    };
  }, [enabled, resetKey]);

  return { percent, scrolling };
}