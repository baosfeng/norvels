import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef } from "react";
import { Link } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";
import { formatWords } from "../../lib/format";
import { buildTocRows, rowIndexOfChapter } from "../../lib/tocRows";

/** 触控行高（H5）：44px 是触摸目标下限，虚拟滚动步进必须与 CSS 完全一致。 */
export const ROW_HEIGHT = 44;
/** 鼠标行高（PC 多列）：目录在桌面上要密，一屏才装得下几十章。 */
export const DENSE_ROW_HEIGHT = 32;
const DRAWER_ROW_HEIGHT = 44;
/** 多列时列间留白（PC 目录用 20px，与原型一致）。 */
const COLUMN_GAP = 20;
/**
 * 无界高度（不传 maxHeight、也不 fill）时，行数超过这个值就退回「限高 + 内部滚动」。
 *
 * 原因：虚拟滚动的视口 = 容器的 clientHeight。容器不设高度时它等于内容高度，
 * 虚拟化就不再裁剪——6000 章会真的渲染 6000 行，手机直接卡住。
 * 800 行以内（常见书的规模，441 章 × 3 列 = 147 行）仍走整页滚动，不产生嵌套滚动。
 */
const UNBOUNDED_ROW_LIMIT = 800;
/** 退回限高模式时给 70vh：够满一屏，不会像 340px 那样外面留一大片死区。 */
const UNBOUNDED_MAX_HEIGHT = "70vh";

/**
 * 目录：虚拟滚动 + 多列。
 *
 * 一行放 N 章（N = columns），行内是「标题 1fr + 字数定宽」两列，所以列的多少只影响
 * 一屏能看多少章，不影响每一行的排版口径（详情页与阅读器抽屉共用同一套行）。
 *
 * 三种高度语义：
 *   - 默认：页面内一段可滚区域（maxHeight 由调用方给）；
 *   - fill：撑满父容器（阅读器抽屉），父容器负责滚动，不再产生第二层滚动；
 *   - 不传 maxHeight 且 fill=false：不限制高度（详情页整页滚动）。
 */
export function ChapterList({
  bookId,
  chapters,
  currentChapterId,
  onNavigate,
  maxHeight,
  ariaLabel,
  layout = "detail",
  columns = 1,
  dense = false,
  fill = false,
  scrollToCurrent = false,
}: {
  bookId: number;
  chapters: ChapterBrief[];
  currentChapterId?: number;
  onNavigate?: () => void;
  maxHeight?: number;
  ariaLabel?: string;
  /** detail：详情页卷内目录（单行省略）；drawer：阅读器抽屉（最多两行） */
  layout?: "detail" | "drawer";
  /** 视觉列数（PC ≥1180 给 3，≥900 给 2，其余 1）；移动端恒为 1 */
  columns?: number;
  /** 紧凑行高（32px），PC 鼠标用 */
  dense?: boolean;
  /** 高度撑满父容器（抽屉里用；配合父级 overflow:auto） */
  fill?: boolean;
  /** 首次渲染后滚到当前章（长目录打开就能看到读到哪了） */
  scrollToCurrent?: boolean;
}) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const cols = Math.max(1, Math.trunc(columns) || 1);
  const rowHeight = layout === "drawer" ? DRAWER_ROW_HEIGHT : dense ? DENSE_ROW_HEIGHT : ROW_HEIGHT;
  const rows = useMemo(() => buildTocRows(chapters, cols), [chapters, cols]);
  // 行数过多时退回限高模式，虚拟化才会真正裁剪（见 UNBOUNDED_ROW_LIMIT）
  const unbounded = !fill && maxHeight === undefined;
  const heightStyle = fill
    ? undefined
    : maxHeight
      ? { maxHeight }
      : unbounded && rows.length > UNBOUNDED_ROW_LIMIT
        ? { maxHeight: UNBOUNDED_MAX_HEIGHT }
        : undefined;
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan: 12,
  });

  const currentRow = rowIndexOfChapter(chapters, cols, currentChapterId);
  useEffect(() => {
    if (!scrollToCurrent || currentRow < 0) return;
    virtualizer.scrollToIndex(currentRow, { align: "center" });
    // virtualizer 身份随行数与行高变化，跟着重建时才需要重新定位
  }, [scrollToCurrent, currentRow, virtualizer, rowHeight]);

  return (
    <div
      ref={scrollRef}
      className={`vol-virtual${fill ? " is-fill" : ""}${dense ? " is-dense" : ""}`}
      style={heightStyle}
      tabIndex={0}
      aria-label={ariaLabel ?? "章节目录"}
      data-testid="chapter-scroll"
      data-layout={layout}
      data-columns={cols}
    >
      <div style={{ height: virtualizer.getTotalSize(), position: "relative", width: "100%" }}>
        {virtualizer.getVirtualItems().map((row) => (
          <div
            key={row.key}
            className="cv-grid"
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              right: 0,
              height: rowHeight,
              transform: `translateY(${row.start}px)`,
              gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
              columnGap: COLUMN_GAP,
            }}
          >
            {rows[row.index].items.map((chapter) => {
              const current = chapter.id === currentChapterId;
              return (
                <Link
                  key={chapter.id}
                  to={`/read/${bookId}/${chapter.id}`}
                  className={`cv-row${layout === "drawer" ? " is-drawer" : ""}${current ? " is-current" : ""}`}
                  style={{ height: rowHeight }}
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
        ))}
      </div>
    </div>
  );
}
