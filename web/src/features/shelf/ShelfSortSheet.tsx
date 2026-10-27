import type { ReadStatus } from "../../api/types";
import { READ_STATUS_FILTERS } from "../../lib/readStatus";
import { SORTS, type SortValue } from "./shelfSorts";

/** H5 底部排序弹层（C-21）：排序 + 阅读状态两段，点遮罩或「关闭」收起。 */
export function ShelfSortSheet({
  sort,
  readStatus,
  onChooseSort,
  onChooseReadStatus,
  onClose,
}: {
  sort: SortValue;
  readStatus: Exclude<ReadStatus, ""> | null;
  onChooseSort: (next: SortValue) => void;
  onChooseReadStatus: (next: Exclude<ReadStatus, ""> | null) => void;
  onClose: () => void;
}) {
  return (
    <div className="sheet-backdrop" data-testid="shelf-sort-sheet" data-od-id="C-21" onClick={onClose}>
      <div
        className="sheet"
        role="dialog"
        aria-modal="true"
        aria-label="排序方式"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="sheet-head">
          <strong>排序方式</strong>
          <button className="btn btn-ghost" type="button" onClick={onClose}>
            关闭
          </button>
        </div>
        <div className="sheet-section" role="group" aria-label="阅读状态筛选">
          <span className="sheet-section-title">阅读状态</span>
          {READ_STATUS_FILTERS.map((option) => (
            <button
              key={option.label}
              type="button"
              className={`sheet-option${option.value === readStatus ? " is-active" : ""}`}
              data-testid={`read-status-option-${option.value ?? "all"}`}
              aria-pressed={option.value === readStatus}
              onClick={() => onChooseReadStatus(option.value)}
            >
              {option.label}
            </button>
          ))}
        </div>
        <div className="sheet-list">
          {SORTS.map((s) => (
            <button
              key={s.value}
              type="button"
              className={`sheet-option${s.value === sort ? " is-active" : ""}`}
              data-testid={`sort-option-${s.value}`}
              aria-pressed={s.value === sort}
              onClick={() => onChooseSort(s.value)}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
