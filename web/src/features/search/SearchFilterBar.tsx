import type { ReactNode } from "react";
import type { AuthorSummary, CategoryNode, TagSummary } from "../../api/types";
import { READ_STATUS_FILTERS } from "../../lib/readStatus";
import type { SearchFilterValues } from "./searchFilters";

/** 连载状态：none 是「未知」的哨兵值（空串已被「全部」占用），与后端同口径。 */
const SERIAL_OPTIONS = [
  { value: "ongoing", label: "连载中" },
  { value: "completed", label: "已完结" },
  { value: "none", label: "未知" },
];

/** 评分 5→1 倒序：多数人先看高分。0 = 未评，不是一个可见筛选项。 */
const RATING_OPTIONS = ["5", "4", "3", "2", "1"].map((value) => ({ value, label: value + " 星" }));

/**
 * 阅读状态复用 lib/readStatus 的筛选候选，不再写第二份标签表。
 * 去掉候选里的 null（那是「全部」）——OptionRow 自己会补一颗「全部」，
 * 两份叠在一起会出现两个都叫「全部」的 chips（真机截图抓到过）。
 */
const READ_OPTIONS = READ_STATUS_FILTERS.flatMap((option) =>
  option.value === null ? [] : [{ value: option.value, label: option.label }],
);

/**
 * 搜索结果上方的筛选区：**渐进展开**——初始只有「分类」一组；选了某个大分类后，
 * 下面依次出现 小分类（有细分时）→ 评分 → 阅读状态 → 连载状态 → 作者 → 标签。
 *
 * 为什么不一次铺满：六组同时摊开要占掉半屏，首屏结果被挤到下一屏；而「先定大类再细化」
 * 是读者真实的取书顺序（QQ 阅读一类移动阅读器也是这个形态）。点一下立刻重查（每一击都写进
 * 地址栏），不需要先展开下拉再选、也不需要再点一次「搜索」；再点一次已选中的 chip = 取消该项。
 *
 * 分类是唯一的「前置」维度：点分类行的「全部」= 收起并清空其余组（页面负责清 URL），
 * 避免出现「没有分类却带着评分」这种看不见的筛选。
 */
export function SearchFilters({
  values,
  onChange,
  activeCount,
  categories,
  authors,
  tags,
  loading = false,
}: {
  values: SearchFilterValues;
  onChange: (patch: Partial<SearchFilterValues>) => void;
  activeCount: number;
  categories: CategoryNode[];
  authors: AuthorSummary[];
  tags: TagSummary[];
  /** 作者 / 标签 / 分类还在取数：chips 先照常显示（只有「全部」），不整块闪没 */
  loading?: boolean;
}) {
  const expanded = values.categoryL1 !== "";
  return (
    <section className="search-filters" data-testid="search-filters" aria-label="筛选条件">
      <CategoryRows values={values} onChange={onChange} categories={categories} />
      {expanded ? (
        <>
          <OptionRow
            label="评分"
            testId="rating"
            value={values.rating}
            options={RATING_OPTIONS}
            onPick={(value) => onChange({ rating: value })}
          />
          <OptionRow
            label="阅读"
            testId="status"
            value={values.status}
            options={READ_OPTIONS}
            onPick={(value) => onChange({ status: value })}
          />
          <OptionRow
            label="连载"
            testId="serial"
            value={values.serial}
            options={SERIAL_OPTIONS}
            onPick={(value) => onChange({ serial: value })}
          />
          <OptionRow
            label="作者"
            testId="author"
            value={values.author}
            options={authors.map((author) => ({
              value: String(author.id),
              label: `${author.name}（${author.bookCount}）`,
            }))}
            onPick={(value) => onChange({ author: value })}
          />
          <OptionRow
            label="标签"
            testId="tag"
            value={values.tag}
            options={tags.map((tag) => ({ value: tag.name, label: `${tag.name}（${tag.bookCount}）` }))}
            onPick={(value) => onChange({ tag: value })}
          />
          {loading ? <p className="inline-note search-filters-note">作者与标签还在取数…</p> : null}
          {activeCount > 0 ? null : (
            <p className="inline-note search-filters-note">点任意一项立即筛选，再点一次取消。</p>
          )}
        </>
      ) : (
        <p className="inline-note search-filters-note" data-testid="search-filters-hint">
          先选一个分类，也可以直接看全部结果。
        </p>
      )}
    </section>
  );
}

/** 分类两行：大分类常驻；选中大分类后立刻展开它的小分类。 */
function CategoryRows({
  values,
  onChange,
  categories,
}: {
  values: SearchFilterValues;
  onChange: (patch: Partial<SearchFilterValues>) => void;
  categories: CategoryNode[];
}) {
  const leaves = (categories.find((node) => node.l1 === values.categoryL1)?.items ?? []).filter(
    (leaf) => leaf.l2 !== "",
  );
  return (
    <>
      <FilterRow label="分类" testId="category">
        <FilterChip
          active={values.categoryL1 === ""}
          testId="search-filter-category-all"
          onClick={() => onChange({ categoryL1: "", categoryL2: "" })}
        >
          全部
        </FilterChip>
        {categories.map((node) => (
          <FilterChip
            key={node.l1 || "uncategorized"}
            active={values.categoryL1 === node.l1}
            testId={`search-filter-category-${node.l1 || "uncategorized"}`}
            onClick={() =>
              onChange(
                values.categoryL1 === node.l1
                  ? { categoryL1: "", categoryL2: "" }
                  : { categoryL1: node.l1, categoryL2: "" },
              )
            }
          >
            {node.l1 || "未分类"}（{node.count}）
          </FilterChip>
        ))}
      </FilterRow>
      {values.categoryL1 === "" ? null : (
        <FilterRow label="细分" testId="subcategory">
          {leaves.length === 0 ? (
            <span className="inline-note">该分类暂无细分</span>
          ) : (
            <>
              <FilterChip
                active={values.categoryL2 === ""}
                testId="search-filter-subcategory-all"
                onClick={() => onChange({ categoryL2: "" })}
              >
                全部
              </FilterChip>
              {leaves.map((leaf) => (
                <FilterChip
                  key={leaf.l2}
                  active={values.categoryL2 === leaf.l2}
                  testId={`search-filter-subcategory-${leaf.l2}`}
                  onClick={() => onChange({ categoryL2: values.categoryL2 === leaf.l2 ? "" : leaf.l2 })}
                >
                  {leaf.l2}（{leaf.count}）
                </FilterChip>
              ))}
            </>
          )}
        </FilterRow>
      )}
    </>
  );
}

/** 一行筛选：标签在左，chips 在右（H5 折成上下两行并横向滚动）。 */
function FilterRow({ label, testId, children }: { label: string; testId: string; children: ReactNode }) {
  return (
    <div className="filter-row" data-testid={`search-filter-${testId}`}>
      <span className="filter-row-label">{label}</span>
      <div className="chips" role="group" aria-label={label}>
        {children}
      </div>
    </div>
  );
}

/** 「全部 + N 个取值」的一行；点已选中的那个 = 回到全部。 */
function OptionRow({
  label,
  testId,
  value,
  options,
  onPick,
}: {
  label: string;
  testId: string;
  value: string;
  options: { value: string; label: string }[];
  onPick: (value: string) => void;
}) {
  return (
    <FilterRow label={label} testId={testId}>
      <FilterChip active={value === ""} testId={`search-filter-${testId}-all`} onClick={() => onPick("")}>
        全部
      </FilterChip>
      {options.map((option) => (
        <FilterChip
          key={option.value}
          active={value === option.value}
          testId={`search-filter-${testId}-${option.value}`}
          onClick={() => onPick(value === option.value ? "" : option.value)}
        >
          {option.label}
        </FilterChip>
      ))}
    </FilterRow>
  );
}

/** 单个 chip：选中态用 aria-pressed 表达（样式与读屏共用同一个事实）。 */
function FilterChip({
  active,
  testId,
  onClick,
  children,
}: {
  active: boolean;
  testId: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button type="button" className="chip" aria-pressed={active} data-testid={testId} onClick={onClick}>
      {children}
    </button>
  );
}
