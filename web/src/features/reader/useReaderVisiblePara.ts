import { useEffect } from "react";

/**
 * 用可见段落推算当前位置：取屏幕中上部第一个可见段落作为 anchor，
 * 滚动时更新、离屏时回退，供进度上报与书签使用。
 */
export function useReaderVisiblePara({
  paragraphs,
  chapterDataId,
  onAnchor,
}: {
  paragraphs: string[];
  chapterDataId: number | undefined;
  onAnchor: (anchor: { chapterId: number; para: number }) => void;
}) {
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined" || paragraphs.length === 0) return;
    const nodes = Array.from(document.querySelectorAll<HTMLElement>("[data-para]"));
    if (nodes.length === 0) return;
    const visible = new Set<number>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const paraIndex = Number((entry.target as HTMLElement).dataset.para ?? "0");
          if (entry.isIntersecting) visible.add(paraIndex);
          else visible.delete(paraIndex);
        }
        if (visible.size > 0) onAnchor({ chapterId: chapterDataId ?? 0, para: Math.min(...visible) });
      },
      { rootMargin: "-10% 0px -60% 0px" },
    );
    nodes.forEach((node) => observer.observe(node));
    return () => observer.disconnect();
  }, [paragraphs, chapterDataId, onAnchor]);
}
