import type { FilterChip } from "./shelfFilterChips";

/**
 * H5 列表顶部的筛选回显：每个条件一个 chip（带 × 单独清除）+「清除全部」。
 * 只在有筛选时出现；没有筛选时这一行不占位。
 * 为什么需要它：筛选后列表变短，用户看不出"为什么只剩这几本"，回显是唯一的解释入口。
 */
export function FilterChipsRow({
  chips,
  onClearAll,
}: {
  chips: FilterChip[];
  onClearAll: () => void;
}) {
  if (chips.length === 0) return null;
  return (
    <div className="filter-chips" role="group" aria-label="当前筛选条件" data-testid="shelf-filter-chips">
      {chips.map((chip) => (
        <button
          key={chip.key}
          type="button"
          className="filter-chip"
          data-testid={`filter-chip-${chip.key}`}
          aria-label={`清除筛选：${chip.label}`}
          onClick={chip.onClear}
        >
          <span className="filter-chip-label">{chip.label}</span>
          <span className="filter-chip-x" aria-hidden="true">
            ×
          </span>
        </button>
      ))}
      <button
        type="button"
        className="chip filter-chips-clear"
        data-testid="filter-chips-clear"
        onClick={onClearAll}
      >
        清除全部
      </button>
    </div>
  );
}
