import { useEffect } from "react";
import type { StreamSegment } from "./useChapterStream";

/**
 * 用可见段落推算当前位置：取屏幕中上部第一个可见段落作为 anchor，
 * 滚动时更新、离屏时回退，供进度上报与书签使用。
 *
 * 连续流里同屏可能压着相邻两章，所以章节一律从段落所属的 section 读，
 * 不能拿「当前章 id」硬套——否则滚过章末时锚点还挂在上一章上。
 */
export function useReaderVisiblePara({
  segments,
  onAnchor,
}: {
  /** 渲染中的章节窗口：段落节点变化时重新观察。 */
  segments: StreamSegment[];
  onAnchor: (anchor: { chapterId: number; para: number }) => void;
}) {
  const key = segments.map((segment) => segment.id).join(",");
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") return;
    const nodes = Array.from(document.querySelectorAll<HTMLElement>("[data-para]"));
    if (nodes.length === 0) return;
    const order = new Map(nodes.map((node, at) => [node, at]));
    const visible = new Set<HTMLElement>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const node = entry.target as HTMLElement;
          if (entry.isIntersecting) visible.add(node);
          else visible.delete(node);
        }
        if (visible.size === 0) return;
        // 文档顺序最靠前的那一段就是读者正在看的那一行
        let pick: HTMLElement | null = null;
        for (const node of visible) {
          if (!pick || (order.get(node) ?? 0) < (order.get(pick) ?? 0)) pick = node;
        }
        if (!pick) return;
        const section = pick.closest<HTMLElement>("[data-chapter-id]");
        onAnchor({ chapterId: Number(section?.dataset.chapterId ?? "0"), para: Number(pick.dataset.para ?? "0") });
      },
      { rootMargin: "-10% 0px -60% 0px" },
    );
    nodes.forEach((node) => observer.observe(node));
    return () => observer.disconnect();
  }, [key, onAnchor]);
}
