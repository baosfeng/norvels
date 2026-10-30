import type { RefObject } from "react";
import { formatCount } from "../../lib/format";
import { RandomBookButton } from "./RandomBookButton";

/** 书架页头（C-20/C-32/C-33）：标题、随机一本、选择模式、H5 排序入口。 */
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
  return (
    <div className="page-head">
      <div>
        <p className="eyebrow">
          {mine ? "我的书架" : "书架"} · {loading ? "读取中" : `${formatCount(total)} 本`}
        </p>
        <h1 ref={titleRef} tabIndex={-1}>
          {mine ? "我的书架" : isH5 ? "书架" : "最近入库"}
        </h1>
        {isH5 ? null : (
          <p className="sub">
            {mine ? "主动加入书架的书，按加入时间排列" : `按${sortLabel}排序`} · 共 {formatCount(total)} 本
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
            aria-label={filterCount > 0 ? `排序与筛选，已启用 ${filterCount} 项筛选` : "排序与筛选"}
            onClick={onOpenSheet}
          >
            {filterCount > 0 ? `筛选 ${filterCount} · ${sortLabel}` : `排序：${sortLabel}`}
          </button>
        ) : null}
      </div>
    </div>
  );
}
