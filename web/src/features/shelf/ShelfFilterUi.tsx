import type { ReactNode } from "react";

/**
 * H5 书架筛选条的两块最小 UI：可折叠分组 + 胶囊选项。
 *
 * 与底部弹层里的 `.sheet-*` 是两套样式（弹层仍服务于桌面档与既有路径）：
 * 平铺面板的 chip 在页面流里，热区与行高按 H5 口径；弹层的 chip 在定高滚动区里。
 */
export function FilterGroup({
  id,
  title,
  value,
  open,
  onToggle,
  children,
}: {
  id: string;
  title: string;
  /** 当前值：收起时也常驻在标题行右侧，不用展开就知道选了什么 */
  value: string;
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <div
      className="shelf-filter-group"
      data-testid={"filter-group-" + id}
      role="group"
      aria-label={title + "筛选"}
    >
      <button
        type="button"
        className="shelf-filter-group-head"
        data-testid={"filter-group-toggle-" + id}
        aria-expanded={open}
        aria-controls={"shelf-filter-body-" + id}
        onClick={onToggle}
      >
        <span className="shelf-filter-group-title">{title}</span>
        <span className="shelf-filter-group-value">{value}</span>
        <span className="shelf-filter-group-chevron" aria-hidden="true" />
      </button>
      {/* 收起用 hidden 而不是从树上摘掉：键盘 Tab 与读屏都跳过 hidden 子树，与视觉一致 */}
      <div id={"shelf-filter-body-" + id} className="shelf-filter-group-body" hidden={!open}>
        {children}
      </div>
    </div>
  );
}

export function FilterChip({
  testId,
  active,
  onClick,
  children,
}: {
  testId: string;
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      className={"shelf-filter-chip" + (active ? " is-active" : "")}
      data-testid={testId}
      aria-pressed={active}
      onClick={onClick}
    >
      {children}
    </button>
  );
}
