import type { AuthorSummary, CategoryNode, TagSummary } from "../../api/types";

/** 筛选条的取值（受控：页面持有，全部从第 1 页重新查）。 */
export interface FilterValues {
  query: string;
  categoryL1: string;
  categoryL2: string;
  authorId: string;
  tag: string;
  serial: string;
}

/**
 * 书籍管理的筛选条：关键词 + 大/小分类 + 作者 + 标签 + 连载状态 + 搜索/重置 + 标签管理入口。
 *
 * 小分类依附大分类：换大分类时把已选的小分类清掉，否则会同时筛出「玄幻 › 都市」这种空集。
 * 没有分类 / 作者数据时下拉仍可用但只有「全部」——不隐藏控件，避免布局跳动。
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
}) {
  const subCategories = categories.find((node) => node.l1 === values.categoryL1)?.items ?? [];
  const filtering =
    values.query !== "" ||
    values.categoryL1 !== "" ||
    values.categoryL2 !== "" ||
    values.authorId !== "" ||
    values.tag !== "" ||
    values.serial !== "";

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
          data-od-id="C-26"
          onChange={(event) => onDraftQuery(event.target.value)}
        />
      </label>
      <label>
        大分类
        <select
          className="select"
          value={values.categoryL1}
          data-testid="admin-filter-category"
          data-od-id="C-27"
          onChange={(event) => onChange({ categoryL1: event.target.value, categoryL2: "" })}
        >
          <option value="">全部</option>
          {categories.map((node) => (
            <option key={node.l1} value={node.l1}>
              {node.l1}（{node.count}）
            </option>
          ))}
        </select>
      </label>
      <label>
        小分类
        <select
          className="select"
          value={values.categoryL2}
          disabled={values.categoryL1 === ""}
          data-testid="admin-filter-subcategory"
          data-od-id="C-28"
          onChange={(event) => onChange({ categoryL2: event.target.value })}
        >
          <option value="">全部</option>
          {subCategories.map((leaf) => (
            <option key={leaf.l2} value={leaf.l2}>
              {leaf.l2}（{leaf.count}）
            </option>
          ))}
        </select>
      </label>
      {values.categoryL1 === "" ? <span className="inline-note">先选一个大分类</span> : null}
      <label>
        作者
        <select
          className="select"
          value={values.authorId}
          data-testid="admin-filter-author"
          data-od-id="C-29"
          onChange={(event) => onChange({ authorId: event.target.value })}
        >
          <option value="">全部</option>
          {authors.map((item) => (
            <option key={item.id} value={String(item.id)}>
              {item.name}（{item.bookCount}）
            </option>
          ))}
        </select>
      </label>
      <label>
        标签
        <select
          className="select"
          value={values.tag}
          data-testid="admin-filter-tag"
          data-od-id="C-30"
          onChange={(event) => onChange({ tag: event.target.value })}
        >
          <option value="">全部</option>
          {tags.map((item) => (
            <option key={item.id} value={item.name}>
              {item.name}（{item.bookCount}）
            </option>
          ))}
        </select>
      </label>
      <label>
        连载状态
        <select
          className="select"
          value={values.serial}
          data-testid="admin-filter-serial"
          data-od-id="C-31"
          onChange={(event) => onChange({ serial: event.target.value })}
        >
          <option value="">全部</option>
          <option value="ongoing">连载中</option>
          <option value="completed">已完结</option>
          <option value="none">未知</option>
        </select>
      </label>
      <button className="btn btn-sm" type="button" data-testid="admin-tag-manage" data-od-id="C-34" onClick={onTagManage}>
        标签管理…
      </button>
      <button className="btn btn-sm" type="submit" data-testid="admin-search-submit" data-od-id="C-32">
        搜索
      </button>
      {filtering ? (
        <button className="btn btn-sm btn-ghost" type="button" data-testid="admin-filter-reset" data-od-id="C-33" onClick={onReset}>
          重置筛选
        </button>
      ) : null}
    </form>
  );
}
