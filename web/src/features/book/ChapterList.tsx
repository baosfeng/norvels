import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef } from "react";
import { Link } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";
import { formatWords } from "../../lib/format";

/**
 * 固定行高与 CSS 里 .cv-row 的行高对齐，虚拟滚动才能算准。
 * H5 的触屏底子给 .cv-row 抬到 min-height:44px，所以两边统一用 44：
 * 行高一致才不会被「36px 步进 + 44px 盒子」叠出 8px 行间重叠。
 */
const ROW_HEIGHT = 44;
const DRAWER_ROW_HEIGHT = 44;

/**
 * 目录虚拟滚动：6000+ 章只渲染视口内的行。
 * 用 @tanstack/react-virtual（单一实现），详情页卷内列表与阅读器抽屉共用。
 *
 * 每行是稳定的两列：标题区 flex:1 + min-width:0（详情单行省略、抽屉最多两行），
 * 字数区固定宽度（左边缘逐行对齐、右对齐、不换行），两列间 12px 间距。
 */
export function ChapterList({
  bookId,
  chapters,
  currentChapterId,
  onNavigate,
  maxHeight = 360,
  ariaLabel,
  layout = "detail",
}: {
  bookId: number;
  chapters: ChapterBrief[];
  currentChapterId?: number;
  onNavigate?: () => void;
  maxHeight?: number;
  ariaLabel?: string;
  /** detail：详情页卷内目录（单行省略）；drawer：阅读器抽屉（最多两行） */
  layout?: "detail" | "drawer";
}) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const rowHeight = layout === "drawer" ? DRAWER_ROW_HEIGHT : ROW_HEIGHT;
  const virtualizer = useVirtualizer({
    count: chapters.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
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
      data-layout={layout}
    >
      <div style={{ height: virtualizer.getTotalSize(), position: "relative", width: "100%" }}>
        {virtualizer.getVirtualItems().map((row) => {
          const chapter = chapters[row.index];
          const current = chapter.id === currentChapterId;
          return (
            <Link
              key={chapter.id}
              to={`/read/${bookId}/${chapter.id}`}
              className={`cv-row${layout === "drawer" ? " is-drawer" : ""}${current ? " is-current" : ""}`}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                right: 0,
                transform: `translateY(${row.start}px)`,
                height: rowHeight,
              }}
              aria-current={current ? "true" : undefined}
              onClick={onNavigate}
            >
              <span className="cv-title">{chapter.title}</span>
              <span className="cv-word" data-testid="chapter-word">
                {formatWords(chapter.wordCount)}
              </span>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
