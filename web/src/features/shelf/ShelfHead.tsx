import type { RefObject } from "react";
import { formatCount } from "../../lib/format";
import { RandomBookButton } from "./RandomBookButton";

/**
 * 书架页头：标题 + 一行操作。
 *
 * H5 不再重复「书架 · N 本 / 书架」两行字：标题只出现一次，数量与排序状态并进下方副标题。
 * 页头不再放「排序 / 筛选」按钮：筛选条平铺在列表正上方（ShelfH5Filters），分组标题常驻、
 * 排序组默认展开——多一个跳到同一处的按钮只会让页头更挤，不增加任何可达性。
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
  onToggleSelecting,
  onOpenBatch,
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
  onToggleSelecting: () => void;
  onOpenBatch: () => void;
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
      </div>
    </div>
  );
}
