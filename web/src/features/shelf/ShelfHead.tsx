import type { RefObject } from "react";
import type { ReadStatus } from "../../api/types";
import { formatCount } from "../../lib/format";
import { readStatusFilterLabel } from "../../lib/readStatus";
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
  readStatus,
  onToggleSelecting,
  onOpenBatch,
  onOpenSheet,
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
  readStatus: Exclude<ReadStatus, ""> | null;
  onToggleSelecting: () => void;
  onOpenBatch: () => void;
  onOpenSheet: () => void;
}) {
  return (
    <div className="page-head">
      <div>
        <p className="eyebrow">书架 · {loading ? "读取中" : `${formatCount(total)} 本`}</p>
        <h1 ref={titleRef} tabIndex={-1}>
          {isH5 ? "书架" : "最近入库"}
        </h1>
        {isH5 ? null : (
          <p className="sub">
            按{sortLabel}排序 · 共 {formatCount(total)} 本
            {filtering ? "（已筛选）" : ""}
          </p>
        )}
      </div>
      <div className="shelf-head-actions">
        {isH5 ? null : (
          <RandomBookButton authorId={authorId ?? undefined} tag={tag ?? undefined} collection={collection ?? undefined} />
        )}
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
        {isH5 ? (
          <button
            className="btn shelf-sort-trigger"
            type="button"
            data-testid="shelf-sort-trigger"
            data-od-id="C-20"
            aria-haspopup="dialog"
            aria-expanded={sheetOpen}
            onClick={onOpenSheet}
          >
            {readStatus === null ? `排序：${sortLabel}` : `${readStatusFilterLabel(readStatus)} · ${sortLabel}`}
          </button>
        ) : null}
      </div>
    </div>
  );
}
