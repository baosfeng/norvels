import { useState, type ReactNode } from "react";
import type { AuthorSummary, CategoryNode, TagSummary } from "../../api/types";

/** 筛选条的取值（受控：页面持有，任何一组变化都从第 1 页重新查）。 */
export interface FilterValues {
  query: string;
  categoryL1: string;
  categoryL2: string;
  authorId: string;
  tag: string;
  serial: string;
}

/** 作者平铺上限：323 位作者全平铺会把筛选条撑成十几行，先给 12 位，其余收进「更多」。 */
const AUTHOR_CHIPS = 12;
const TAG_CHIPS = 16;

const SERIAL_OPTIONS = [
  { value: "", label: "全部" },
  { value: "ongoing", label: "连载中" },
  { value: "completed", label: "已完结" },
  { value: "none", label: "未知" },
] as const;

/**
 * 书籍管理的筛选条：关键词 + 大/小分类 + 作者 + 标签 + 连载状态 + 搜索/重置 + 标签管理入口。
 *
 * 五组筛选统一用平铺 chips（QQ 阅读那套形态）：每组一行、第一个是「全部」、选中项高亮、点一下即筛。
 * 原生 select 一个不留——系统灰下拉和页面视觉不是一套，展开还要多点一次。
 * 小分类依附大分类：换大分类时把已选的小分类清掉，否则会同时筛出「玄幻 › 都市」这种空集。
 * 作者 / 标签数量可能很大：先平铺前 N 位，超出的收进「更多」（限高滚动），已在筛的项即使不在前 N 位也保留一个 chip。
 */
export function AdminFilterBar({
  draftQuery,
  onDraftQuery,
  values,
  onChange,
  onSubmit,
  onReset,
  onTagManage,
  categories,
  authors,
  tags,
  authorQuery,
  onAuthorQuery,
}: {
  draftQuery: string;
  onDraftQuery: (value: string) => void;
  values: FilterValues;
  onChange: (patch: Partial<FilterValues>) => void;
  onSubmit: () => void;
  onReset: () => void;
  onTagManage: () => void;
  categories: CategoryNode[];
  authors: AuthorSummary[];
  tags: TagSummary[];
  /** 作者「更多」里的搜索词：页面据此向服务端查（本地只拿得到前 200 位） */
  authorQuery: string;
  onAuthorQuery: (value: string) => void;
}) {
  const [showAllAuthors, setShowAllAuthors] = useState(false);
  const [showAllTags, setShowAllTags] = useState(false);
  // 只平铺真实细分：l2 为空串表示「该大分类下未细分的书」，不是一个小分类。
  const subCategories =
    (categories.find((node) => node.l1 === values.categoryL1)?.items ?? []).filter((leaf) => leaf.l2 !== "");
  const filtering =
    values.query !== "" ||
    values.categoryL1 !== "" ||
    values.categoryL2 !== "" ||
    values.authorId !== "" ||
    values.tag !== "" ||
    values.serial !== "";

  const chosenAuthor = authors.find((item) => String(item.id) === values.authorId) ?? null;
  const visibleAuthors = visiblePage(authors, AUTHOR_CHIPS, showAllAuthors, chosenAuthor);
  const chosenTag = tags.find((item) => item.name === values.tag) ?? null;
  const visibleTags = visiblePage(tags, TAG_CHIPS, showAllTags, chosenTag);

  return (
    <form
      className="toolbar admin-filter"
      data-testid="admin-filters"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <label className="search-wrap">
        <span className="sr-only">搜索书名或作者</span>
        <input
          className="input"
          type="search"
          value={draftQuery}
          placeholder="搜书名或作者"
          data-testid="admin-search"
          onChange={(event) => onDraftQuery(event.target.value)}
        />
      </label>

      <FilterField id="admin-cat-l1-label" label="大分类" testId="admin-filter-category">
        <OptionChip active={values.categoryL1 === ""} onClick={() => onChange({ categoryL1: "", categoryL2: "" })}>
          全部
        </OptionChip>
        {categories.map((node) => (
          <OptionChip
            key={node.l1}
            active={values.categoryL1 === node.l1}
            onClick={() => onChange({ categoryL1: node.l1, categoryL2: "" })}
          >
            {node.l1}（{node.count}）
          </OptionChip>
        ))}
      </FilterField>

      {values.categoryL1 === "" ? null : (
        <FilterField id="admin-cat-l2-label" label="小分类" testId="admin-filter-subcategory">
          {subCategories.length === 0 ? (
            <span className="inline-note">该分类暂无细分</span>
          ) : (
            <>
              <OptionChip active={values.categoryL2 === ""} onClick={() => onChange({ categoryL2: "" })}>
                全部
              </OptionChip>
              {subCategories.map((leaf) => (
                <OptionChip
                  key={leaf.l2}
                  active={values.categoryL2 === leaf.l2}
                  onClick={() => onChange({ categoryL2: leaf.l2 })}
                >
                  {leaf.l2}（{leaf.count}）
                </OptionChip>
              ))}
            </>
          )}
        </FilterField>
      )}

      <FilterField
        id="admin-author-label"
        label="作者"
        testId="admin-filter-author"
        expanded={showAllAuthors}
        trailing={
          <MoreChip
            hidden={visibleAuthors.items.length}
            total={visibleAuthors.total}
            open={showAllAuthors}
            testId="admin-filter-author-more"
            onToggle={() => setShowAllAuthors((prev) => !prev)}
          />
        }
        under={
          showAllAuthors ? (
            <input
              className="input filter-author-search"
              type="search"
              value={authorQuery}
              placeholder="在作者里找…"
              aria-label="搜索作者"
              data-testid="admin-filter-author-search"
              onChange={(event) => onAuthorQuery(event.target.value)}
            />
          ) : null
        }
      >
        <OptionChip active={values.authorId === ""} onClick={() => onChange({ authorId: "" })}>
          全部
        </OptionChip>
        {visibleAuthors.items.map((item) => (
          <OptionChip
            key={item.id}
            testId={`admin-filter-author-${item.id}`}
            active={values.authorId === String(item.id)}
            onClick={() => onChange({ authorId: String(item.id) })}
          >
            {item.name}（{item.bookCount}）
          </OptionChip>
        ))}
      </FilterField>

      <FilterField
        id="admin-tag-label"
        label="标签"
        testId="admin-filter-tag"
        expanded={showAllTags}
        trailing={
          <MoreChip
            hidden={visibleTags.items.length}
            total={visibleTags.total}
            open={showAllTags}
            testId="admin-filter-tag-more"
            onToggle={() => setShowAllTags((prev) => !prev)}
          />
        }
      >
        <OptionChip active={values.tag === ""} onClick={() => onChange({ tag: "" })} testId="admin-filter-tag-all">
          全部
        </OptionChip>
        {visibleTags.items.map((item) => (
          <OptionChip
            key={item.id}
            testId={`admin-filter-tag-${item.id}`}
            active={values.tag === item.name}
            onClick={() => onChange({ tag: item.name })}
          >
            {item.name}（{item.bookCount}）
          </OptionChip>
        ))}
      </FilterField>

      <FilterField id="admin-serial-label" label="连载状态" testId="admin-filter-serial">
        {SERIAL_OPTIONS.map((option) => (
          <OptionChip
            key={option.value || "all"}
            testId={`admin-filter-serial-${option.value || "all"}`}
            active={values.serial === option.value}
            onClick={() => onChange({ serial: option.value })}
          >
            {option.label}
          </OptionChip>
        ))}
      </FilterField>

      <div className="filter-actions">
        <button className="btn btn-sm" type="button" data-testid="admin-tag-manage" onClick={onTagManage}>
          标签管理…
        </button>
        <button className="btn btn-sm" type="submit" data-testid="admin-search-submit">
          搜索
        </button>
        {filtering ? (
          <button className="btn btn-sm btn-ghost" type="button" data-testid="admin-filter-reset" onClick={onReset}>
            重置筛选
          </button>
        ) : null}
      </div>
    </form>
  );
}

/** 一组平铺筛选：小标题 + chips 容器（限高滚动只在「更多」展开后启用）。 */
function FilterField({
  id,
  label,
  testId,
  expanded = false,
  trailing,
  under,
  children,
}: {
  id: string;
  label: string;
  testId: string;
  /** 展开态：chips 改为折行 + 限高滚动（「更多」全部展开时用） */
  expanded?: boolean;
  /** 行尾固定元素（「更多 / 收起」），不跟着 chips 横向滚动 */
  trailing?: ReactNode;
  /** 展开后才出现的附加控件（作者搜索框），占满 chips 那一列 */
  under?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className={`filter-field${expanded ? " is-open" : ""}`}>
      <span className="filter-label" id={id}>
        {label}
      </span>
      <div
        className={`chips${expanded ? " is-open" : ""}`}
        role="group"
        aria-labelledby={id}
        data-testid={testId}
      >
        {children}
      </div>
      {trailing ? <div className="filter-trailing">{trailing}</div> : null}
      {under ? <div className="filter-under">{under}</div> : null}
    </div>
  );
}

/** 一颗筛选项：选中态走 aria-pressed（视觉规则与大/小分类共用一套）。 */
function OptionChip({
  active,
  onClick,
  testId,
  children,
}: {
  active: boolean;
  onClick: () => void;
  testId?: string;
  children: ReactNode;
}) {
  return (
    <button type="button" className="chip" aria-pressed={active} data-testid={testId} onClick={onClick}>
      {children}
    </button>
  );
}

/** 「更多 N …」/「收起」：只有在还有没平铺的选项时才出现。 */
function MoreChip({
  hidden,
  total,
  open,
  testId,
  onToggle,
}: {
  hidden: number;
  total: number;
  open: boolean;
  testId: string;
  onToggle: () => void;
}) {
  // 收起态且没有更多时整颗不渲染；展开后即使已全铺开也要留「收起」，否则收不回去
  if (!open && total <= hidden) return null;
  const rest = total - hidden;
  return (
    <button type="button" className="chip is-more" data-testid={testId} onClick={onToggle}>
      {open ? "收起" : `更多 ${rest} 项…`}
    </button>
  );
}

/** 平铺列表：前 limit 项；展开后给全部。已选的那一项（若不在前 limit 里）补在最前面，高亮才看得见。 */
function visiblePage<T extends { id: number }>(
  all: T[],
  limit: number,
  open: boolean,
  chosen: T | null,
): { items: T[]; total: number } {
  if (open) return { items: all, total: all.length };
  const head = all.slice(0, limit);
  if (chosen && !head.some((item) => item.id === chosen.id)) {
    return { items: [chosen, ...head], total: all.length };
  }
  return { items: head, total: all.length };
}
