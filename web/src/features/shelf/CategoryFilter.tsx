import { leavesOf, useCategoryOptions } from "./categoryOptions";

/** 分类筛选值：两级都可为空（null = 不过滤）。小分类依附于大分类。 */
export interface CategoryValue {
  l1: string | null;
  l2: string | null;
}

/**
 * 分类筛选器（0015）：大分类下拉 + 联动的小分类下拉（PC 工具栏用）。
 * 选项来自 `useCategoryOptions`——与 H5 弹层同一份，见 categoryOptions.ts。
 *
 * 大分类切换时小分类自动清空（后端只给 L2 时按不过滤处理，前端不该造出这种组合）。
 */
export function CategoryFilter({
  value,
  onChange,
}: {
  value: CategoryValue;
  onChange: (next: CategoryValue) => void;
}) {
  const { nodes, error } = useCategoryOptions();
  const l1 = value.l1 ?? "";
  const leaves = leavesOf(nodes, value.l1);

  return (
    <>
      <label>
        分类
        <select
          className="select"
          value={l1}
          data-testid="filter-category-l1"
          data-od-id="C-26"
          onChange={(e) => onChange({ l1: e.target.value || null, l2: null })}
        >
          <option value="">全部分类</option>
          {nodes.map((node) => (
            <option key={node.l1} value={node.l1}>
              {node.l1}（{node.count}）
            </option>
          ))}
        </select>
      </label>
      {leaves.length > 0 ? (
        <label>
          小分类
          <select
            className="select"
            value={value.l2 ?? ""}
            data-testid="filter-category-l2"
            data-od-id="C-27"
            onChange={(e) => onChange({ l1, l2: e.target.value || null })}
          >
            <option value="">全部小分类</option>
            {leaves.map((leaf) => (
              <option key={leaf.l2} value={leaf.l2}>
                {leaf.l2}（{leaf.count}）
              </option>
            ))}
          </select>
        </label>
      ) : null}
      {error ? (
        <span className="meta filter-hints" data-testid="category-hint">
          分类暂时读不到，可以先用其它筛选。
        </span>
      ) : null}
    </>
  );
}
