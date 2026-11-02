import { useEffect, useRef } from "react";
import { SURFACE_SELECTOR, scrollMetrics } from "./readerStreamLayout";

/**
 * 越界阈值：距顶 / 距底不足一屏时就把窗口往那一侧多放一章。
 *
 * 只认「阈值」不认「越界」：等到 scrollTop 撞到 0 或撞到 max 再补，读者会先看到一屏空白、
 * 再等一次请求；提前一屏补，读者滚过章界时上一章 / 下一章的正文已经在 DOM 里。
 * 这里只扩渲染窗口，不动可见章——可见章仍由判定带决定，中途的正常滚动不会换章。
 */
const EDGE_SCREENS = 1;
/** 内容本身不足一屏时的下限：没有可滚区间就谈不上越界。 */
const MIN_SCROLLABLE = 8;

/**
 * 阅读流两端：滚到顶 / 滚到底时通知调用方往那一侧扩窗口。
 *
 * 只监听滚动、不做重排：窗口扩出来以后由 useChapterStream 的布局效应把高度补进 scrollTop，
 * 读者眼前的正文一动不动——所以「滚到顶」看到的是上一章的末尾接着往下，
 * 而不是被弹回上一章的章首或整页顶部。
 */
export function useChapterEdges({
  onTop,
  onBottom,
  enabled = true,
}: {
  onTop: () => void;
  onBottom: () => void;
  /** 首屏四态（加载 / 出错）期间不扩窗口：那时还没有可读的正文。 */
  enabled?: boolean;
}) {
  // 回调每次渲染都是新引用：用 ref 持有最新的，监听器只装一次
  const handlers = useRef({ onTop, onBottom });
  useEffect(() => {
    handlers.current = { onTop, onBottom };
  }, [onTop, onBottom]);

  useEffect(() => {
    if (!enabled) return;
    let frame = 0;
    const measure = () => {
      if (frame) return;
      frame = window.requestAnimationFrame(() => {
        frame = 0;
        const { scrollTop, clientHeight, scrollHeight } = scrollMetrics();
        if (clientHeight <= 0) return;
        const scrollable = scrollHeight - clientHeight;
        if (scrollable < MIN_SCROLLABLE) return;
        const margin = clientHeight * EDGE_SCREENS;
        if (scrollTop <= margin) handlers.current.onTop();
        else if (scrollTop + clientHeight >= scrollHeight - margin) handlers.current.onBottom();
      });
    };
    measure();
    const surface = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
    window.addEventListener("scroll", measure, { passive: true });
    surface?.addEventListener("scroll", measure, { passive: true });
    return () => {
      if (frame) window.cancelAnimationFrame(frame);
      window.removeEventListener("scroll", measure);
      surface?.removeEventListener("scroll", measure);
    };
  }, [enabled]);
}
