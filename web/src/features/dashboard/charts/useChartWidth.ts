import { useEffect, useRef, useState } from "react";

/** 量不到宽度时用的兜底（jsdom 里元素没有布局，getBoundingClientRect 全是 0）。 */
export const FALLBACK_WIDTH = 520;

/**
 * 图体宽度：SVG 按真实像素画，字号与线宽才不会被拉伸。
 *
 * 用 ResizeObserver 跟踪容器宽度（不用 viewBox 缩放：非等比缩放下 9px 的刻度会变形）。
 * 环境没有 ResizeObserver（jsdom / 老浏览器）时退回监听窗口 resize，宽度取兜底值。
 */
export function useChartWidth(fallback = FALLBACK_WIDTH) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () => {
      const next = Math.round(element.getBoundingClientRect().width) || fallback;
      setWidth((prev) => (prev === next ? prev : next));
    };
    measure();
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", measure);
      return () => window.removeEventListener("resize", measure);
    }
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [fallback]);

  return [ref, width || fallback] as const;
}
