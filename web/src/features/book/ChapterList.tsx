import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef } from "react";
import { Link } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";

/** 固定行高与 CSS 里 .cv-row 的 height:36px 对齐，虚拟滚动才能算准。 */
const ROW_HEIGHT = 36;

/**
 * 目录虚拟滚动：6000+ 章只渲染视口内的行。
 * 用 @tanstack/react-virtual（单一实现），详情页卷内列表与阅读器抽屉共用。
 */
export function ChapterList({
  bookId,
  chapters,
  currentChapterId,
  onNavigate,
  maxHeight = 360,
  ariaLabel,
}: {
  bookId: number;
  chapters: ChapterBrief[];
  currentChapterId?: number;
  onNavigate?: () => void;
  maxHeight?: number;
  ariaLabel?: string;
}) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const virtualizer = useVirtualizer({
    count: chapters.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 20,
  });

  return (
    <div
      ref={scrollRef}
      className="vol-virtual"
      style={{ maxHeight }}
      tabIndex={0}
      aria-label={ariaLabel ?? "章节目录"}
      data-testid="chapter-scroll"
    >
      <div style={{ height: virtualizer.getTotalSize(), position: "relative", width: "100%" }}>
        {virtualizer.getVirtualItems().map((row) => {
          const chapter = chapters[row.index];
          const current = chapter.id === currentChapterId;
          return (
            <Link
              key={chapter.id}
              to={`/read/${bookId}/${chapter.id}`}
              className={`cv-row${current ? " is-current" : ""}`}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                right: 0,
                transform: `translateY(${row.start}px)`,
                height: ROW_HEIGHT,
              }}
              aria-current={current ? "true" : undefined}
              onClick={onNavigate}
            >
              <span className="t">{chapter.title}</span>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
