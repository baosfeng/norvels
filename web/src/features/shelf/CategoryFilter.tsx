import { useCategories } from "../../api/hooks";

/** 分类筛选值：两级都可为空（null = 不过滤）。小分类依附于大分类。 */
export interface CategoryValue {
  l1: string | null;
  l2: string | null;
}

/**
 * 分类筛选器（0015）：大分类下拉 + 联动的小分类下拉。
 *
 * 大分类切换时小分类自动清空（后端只给 L2 时按不过滤处理，前端不该造出这种组合）。
 * 未分类（l1 为空串）的书不进下拉——它们只能从「全部分类」里看到，
 * 因为后端的空串已经被「不过滤」占用。
 */
export function CategoryFilter({
  value,
  onChange,
}: {
  value: CategoryValue;
  onChange: (next: CategoryValue) => void;
}) {
  const { data, error } = useCategories();
  const l1 = value.l1 ?? "";
  const nodes = (data?.items ?? []).filter((node) => node.l1 !== "");
  const current = nodes.find((node) => node.l1 === l1);
  // 有命名的小分类才渲染第二级：选中小分类会排除「该大分类下未细分」的书，
  // 所以即使只有一个选项也有意义（未分类的书仍只能从「全部分类」里看到）。
  const leaves = (current?.items ?? []).filter((leaf) => leaf.l2 !== "");

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
