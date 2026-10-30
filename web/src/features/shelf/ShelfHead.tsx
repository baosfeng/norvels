import type { RefObject } from "react";
import { formatCount } from "../../lib/format";
import { RandomBookButton } from "./RandomBookButton";

/**
 * 书架页头：标题 + 一行操作。
 *
 * H5 不再重复「书架 · N 本 / 书架」两行字：标题只出现一次，数量与排序状态并进下方操作行；
 * 「排序：xx」与「筛选」合并成一个按钮（有筛选时带计数），每次打开弹层只换一个入口。
 * PC 保留「最近入库」标题与副标题——宽屏有位置写清排序与总量，不必压缩。
 */
export function ShelfHead({
  isH5,
  loading,
  total,
  titleRef,
  sortLabel,
  filtering,
  authorId,
  tag,
  collection,
  selecting,
  selectedCount,
  sheetOpen,
  filterCount,
  onToggleSelecting,
  onOpenBatch,
  onOpenSheet,
  mine,
}: {
  isH5: boolean;
  loading: boolean;
  total: number;
  titleRef: RefObject<HTMLHeadingElement | null>;
  sortLabel: string;
  filtering: boolean;
  authorId: number | null;
  tag: string | null;
  collection: number | null;
  selecting: boolean;
  selectedCount: number;
  sheetOpen: boolean;
  /** 已启用的筛选项数（H5 入口按钮据此显示「筛选 n」；PC 工具栏自己展示，不需要） */
  filterCount: number;
  onToggleSelecting: () => void;
  onOpenBatch: () => void;
  onOpenSheet: () => void;
  /** 当前是「我的书架」：标题与副标题换一套，避免把个人书架说成"最近入库" */
  mine: boolean;
}) {
  const countText = loading ? "读取中" : `${formatCount(total)} 本`;

  return (
    <div className="page-head shelf-head">
      <div>
        {isH5 ? null : (
          <p className="eyebrow">
            {mine ? "我的书架" : "书架"} · {countText}
          </p>
        )}
        <h1 ref={titleRef} tabIndex={-1}>
          {mine ? "我的书架" : isH5 ? "书架" : "最近入库"}
        </h1>
        {isH5 ? (
          <p className="sub shelf-head-sub">
            {countText}
            {filtering ? " · 已筛选" : ` · 按${sortLabel}`}
          </p>
        ) : (
          <p className="sub">
            {mine ? "主动加入书架的书，按加入时间排列" : `按${sortLabel}排序`} · 共 {countText}
            {filtering ? "（已筛选）" : ""}
          </p>
        )}
      </div>
      <div className="shelf-head-actions">
        {isH5 ? null : (
          <RandomBookButton authorId={authorId ?? undefined} tag={tag ?? undefined} collection={collection ?? undefined} />
        )}
        {/* 我的书架不做批量改元信息：那是书库管理动作，与"我挑出来的书"是两件事 */}
        {mine ? null : (
          <button
            className="btn"
            type="button"
            data-testid="shelf-select-toggle"
            data-od-id="C-32"
            aria-pressed={selecting}
            onClick={onToggleSelecting}
          >
            {selecting ? "退出选择" : "选择"}
          </button>
        )}
        {selecting ? (
          <button
            className="btn btn-primary"
            type="button"
            data-testid="shelf-batch-author"
            data-od-id="C-33"
            disabled={selectedCount === 0}
            onClick={onOpenBatch}
          >
            批量设作者（{selectedCount}）
          </button>
        ) : null}
        {selecting && selectedCount === 0 ? (
          <span className="inline-note" data-testid="shelf-batch-hint">
            勾选至少一本书后可用。
          </span>
        ) : null}
        {isH5 && !mine ? (
          <button
            className="btn shelf-sort-trigger"
            type="button"
            data-testid="shelf-sort-trigger"
            data-od-id="C-20"
            aria-haspopup="dialog"
            aria-expanded={sheetOpen}
            aria-label={
              filterCount > 0 ? `排序与筛选，已启用 ${filterCount} 项筛选` : "排序与筛选"
            }
            onClick={onOpenSheet}
          >
            筛选{filterCount > 0 ? ` ${filterCount}` : ""} · {sortLabel}
          </button>
        ) : null}
      </div>
    </div>
  );
}
