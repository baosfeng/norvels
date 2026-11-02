import { useEffect, useRef } from "react";
import { SURFACE_SELECTOR } from "./readerStreamLayout";

/**
 * 可见章判定带：视口 4%–20% 这条带落在哪一章，就算读到哪一章。
 * 贴着顶沿而不是正中——进入章节时读者停在章首，正中会把「上方还留着的上一章」当成可见章。
 */
const VISIBLE_BAND = "-4% 0px -80% 0px";
const SEGMENT_SELECTOR = "[data-chapter-id]";

/**
 * 订阅流里每一章的交叉状态，把「判定带里压着哪几章」交给 judge。
 *
 * 订阅要按「真的渲染出来了哪几章」重装：窗口是「想要哪几章」，正文到货前 effect
 * 已经跑过一次（那时 DOM 里还没有 section），只按窗口重装会永远观察不到任何章。
 */
export function useVisibleChapter({
  segmentKey,
  renderedKey,
  judge,
}: {
  /** 窗口 key（想要哪几章）：窗口一变就重装，旧的观察目标已经不在流里。 */
  segmentKey: string;
  /** 渲染 key（真的渲染出来了哪几章）：正文到货会让它变化。 */
  renderedKey: string;
  judge: (seen: Set<HTMLElement>, order: Map<HTMLElement, number>) => void;
}) {
  // judge 每次渲染都是新引用：用 ref 持有最新的，避免每次渲染都重装观察器
  const judgeRef = useRef(judge);
  useEffect(() => {
    judgeRef.current = judge;
  }, [judge]);

  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") return;
    if (!renderedKey) return;
    const nodes = Array.from(document.querySelectorAll<HTMLElement>(SEGMENT_SELECTOR));
    if (nodes.length === 0) return;
    // H5 正文自己滚（.reader-main 定高 overflow），判定带就相对这个容器；PC 由整页滚，相对视口
    const surface = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
    const scroller = surface && surface.scrollHeight - surface.clientHeight > 1 ? surface : null;
    const order = new Map(nodes.map((node, at) => [node, at]));
    const seen = new Set<HTMLElement>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const node = entry.target as HTMLElement;
          if (entry.isIntersecting) seen.add(node);
          else seen.delete(node);
        }
        judgeRef.current(seen, order);
      },
      { root: scroller, rootMargin: VISIBLE_BAND },
    );
    nodes.forEach((node) => observer.observe(node));
    return () => observer.disconnect();
  }, [segmentKey, renderedKey]);
}
