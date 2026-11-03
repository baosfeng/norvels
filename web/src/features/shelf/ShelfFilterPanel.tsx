import { useState } from "react";
import type { ReadStatus } from "../../api/types";
import { READ_STATUS_FILTERS } from "../../lib/readStatus";
import type { CategoryValue } from "./CategoryFilter";
import { leavesOf, useCategoryOptions } from "./categoryOptions";
import { FilterChipsRow } from "./FilterChipsRow";
import type { FilterChip as FilterChipModel } from "./shelfFilterChips";
import { categoryLabelOf, collectionNameOf, ratingLabelOf, readStatusLabelOf, sortLabelOf } from "./shelfFilterLabels";
import { FilterChip, FilterGroup } from "./ShelfFilterUi";
import { SORTS, type SortValue } from "./shelfSorts";

const RATINGS = [5, 4, 3, 2, 1] as const;
/** 默认展开的分组：只有排序。其余分组标题与当前值常驻可见，点一次才铺选项。 */
const OPEN_BY_DEFAULT: readonly string[] = ["sort"];

type AuthorOption = { id: number; name: string; bookCount: number };
type CountedOption = { id: number; name: string; bookCount: number };

/**
 * 分组计数：`status:reading` → 37、`rating:4` → 12。
 * 缺项（还没取回来 / 取失败）就不显示数字——不拿 0 冒充「没有」。
 */
export type FacetCounts = Record<string, number>;

export interface ShelfFilterPanelProps {
  sort: SortValue;
  onChooseSort: (next: SortValue) => void;
  readStatus: Exclude<ReadStatus, ""> | null;
  onChooseReadStatus: (next: Exclude<ReadStatus, ""> | null) => void;
  rating: number | null;
  onChooseRating: (next: number | null) => void;
  authorId: number | null;
  /** 当前作者的显示名：由调用方解析（候选表 + 作者详情），没解析到就用候选表 / 「已选作者」兜底 */
  authorName?: string | null;
  authors: AuthorOption[];
  onChooseAuthor: (next: number | null) => void;
  collection: number | null;
  collections: CountedOption[];
  onChooseCollection: (next: number | null) => void;
  category: CategoryValue;
  onChooseCategory: (next: CategoryValue) => void;
  tag: string | null;
  tags: CountedOption[];
  onChooseTag: (next: string | null) => void;
  filtering: boolean;
  /** 已启用的筛选项数：写进面板标题，用户不用自己数 */
  filterCount: number;
  /** 阅读状态与评分的分组计数（分类 / 书单 / 标签自带接口计数，不走这里） */
  facetCounts: FacetCounts | null;
  /** 当前命中数：底部「查看 N 本」用；加载中拿不到给 null，改显示「查看结果」 */
  resultCount: number | null;
  /** 已选条件（每项自带「只清这一项」的回调）：点 × 单独取消 */
  chips: FilterChipModel[];
  onClearAll: () => void;
  /** 「查看 N 本」：把结果列表滚进视口（面板留在原位，能接着调） */
  onViewResults: () => void;
}

/**
 * H5 书架的筛选条：**直接平铺在页面上**，不再藏进底部弹层。
 *
 * 为什么外置：弹层要「先点开才知道有什么」，点开之前筛选条件不可见，窄屏用户常常
 * 不知道还有评分 / 分类这些条件。平铺后分组标题常驻，选了什么、还剩什么一眼可见。
 *
 * 结构：标题行（已选 N 项 + 常驻「清除筛选」）→ 已选条件 chips（可单独 ×）→ 分组
 * （排序默认展开，其余点标题展开、标题右侧写着当前值）→ 底部「查看 N 本」把列表滚进视口。
 * 每组都是「标题 + 换行铺满的 chip」；分类两级同组，不占原生下拉。
 *
 * 计数口径：分类 / 书单 / 标签用接口自带的计数；阅读状态与评分后端没有分组计数接口，
 * 由 `useShelfFacetCounts` 按「其它维度 + 该项」各问一次 total（与点下去之后的列表同源，
 * 所以标签上的数字必然等于「查看 N 本」的数字）。取不到就不显示数字，不编造。
 */
export function ShelfFilterPanel(props: ShelfFilterPanelProps) {
  const {
    sort,
    onChooseSort,
    readStatus,
    onChooseReadStatus,
    rating,
    onChooseRating,
    authorId,
    authorName,
    authors,
    onChooseAuthor,
    collection,
    collections,
    onChooseCollection,
    category,
    onChooseCategory,
    tag,
    tags,
    onChooseTag,
    filtering,
    filterCount,
    facetCounts,
    resultCount,
    chips,
    onClearAll,
    onViewResults,
  } = props;
  const [open, setOpen] = useState<ReadonlySet<string>>(() => new Set(OPEN_BY_DEFAULT));
  const categories = useCategoryOptions();
  const leaves = leavesOf(categories.nodes, category.l1);
  const authorLabel =
    authorId === null ? null : authorName ?? (authors.find((item) => item.id === authorId)?.name || "已选作者");

  function toggle(id: string) {
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <section className="shelf-filter-panel" aria-label="排序与筛选" data-testid="shelf-filter-panel">
      <div className="shelf-filter-panel-head">
        <span className="shelf-filter-panel-title">筛选{filterCount > 0 ? " · 已选 " + filterCount + " 项" : ""}</span>
        <button
          type="button"
          className="btn btn-ghost shelf-filter-clear"
          data-testid="panel-clear-filters"
          disabled={!filtering}
          onClick={onClearAll}
        >
          清除筛选
        </button>
      </div>

      <FilterChipsRow chips={chips} onClearAll={onClearAll} />

      <FilterGroup id="sort" title="排序" value={sortLabelOf(sort)} open={open.has("sort")} onToggle={() => toggle("sort")}>
        <div className="shelf-filter-options is-triple">
          {SORTS.map((option) => (
            <FilterChip
              key={option.value}
              testId={"sort-option-" + option.value}
              active={option.value === sort}
              onClick={() => onChooseSort(option.value)}
            >
              {option.label}
            </FilterChip>
          ))}
        </div>
      </FilterGroup>

      <FilterGroup
        id="status"
        title="阅读状态"
        value={readStatusLabelOf(readStatus)}
        open={open.has("status")}
        onToggle={() => toggle("status")}
      >
        <div className="shelf-filter-options">
          {/* 「全部」的数目就是当前结果总数，由底部「查看 N 本」回答，这里不重复一次 */}
          <FilterChip
            testId="read-status-option-all"
            active={readStatus === null}
            onClick={() => onChooseReadStatus(null)}
          >
            全部
          </FilterChip>
          {READ_STATUS_FILTERS.filter((option) => option.value !== null).map((option) => (
            <FilterChip
              key={option.label}
              testId={"read-status-option-" + option.value}
              active={option.value === readStatus}
              onClick={() => onChooseReadStatus(option.value)}
            >
              {withCount(option.label, facetCounts?.["status:" + option.value])}
            </FilterChip>
          ))}
        </div>
      </FilterGroup>

      <FilterGroup id="rating" title="评分" value={ratingLabelOf(rating)} open={open.has("rating")} onToggle={() => toggle("rating")}>
        <div className="shelf-filter-options is-grid">
          <FilterChip testId="rating-option-all" active={rating === null} onClick={() => onChooseRating(null)}>
            全部
          </FilterChip>
          {RATINGS.map((star) => (
            <FilterChip
              key={star}
              testId={"rating-option-" + star}
              active={rating === star}
              onClick={() => onChooseRating(star)}
            >
              {withCount(star + " 星", facetCounts?.["rating:" + star])}
            </FilterChip>
          ))}
        </div>
      </FilterGroup>

      <FilterGroup
        id="category"
        title="分类"
        value={categoryLabelOf(category)}
        open={open.has("category")}
        onToggle={() => toggle("category")}
      >
        <div className="shelf-filter-options" data-testid="category-l1-options">
          <FilterChip
            testId="category-option-l1-all"
            active={category.l1 === null}
            onClick={() => onChooseCategory({ l1: null, l2: null })}
          >
            全部
          </FilterChip>
          {categories.nodes.map((node) => (
            <FilterChip
              key={node.l1}
              testId={"category-option-l1-" + node.l1}
              active={category.l1 === node.l1}
              onClick={() => onChooseCategory({ l1: node.l1, l2: null })}
            >
              {node.l1}（{node.count}）
            </FilterChip>
          ))}
        </div>
        {/* 小分类从属于选中的大分类：左侧竖线缩进一档，表示它只对上面那一项生效 */}
        {category.l1 !== null && leaves.length > 0 ? (
          <div className="shelf-filter-sub" data-testid="category-l2-options">
            <span className="shelf-filter-sub-title">小分类</span>
            <div className="shelf-filter-options">
              <FilterChip
                testId="category-option-l2-all"
                active={category.l2 === null}
                onClick={() => onChooseCategory({ l1: category.l1, l2: null })}
              >
                全部
              </FilterChip>
              {leaves.map((leaf) => (
                <FilterChip
                  key={leaf.l2}
                  testId={"category-option-l2-" + leaf.l2}
                  active={category.l2 === leaf.l2}
                  onClick={() => onChooseCategory({ l1: category.l1, l2: leaf.l2 })}
                >
                  {leaf.l2}（{leaf.count}）
                </FilterChip>
              ))}
            </div>
          </div>
        ) : null}
        {category.l1 !== null && leaves.length === 0 ? (
          <p className="meta" data-testid="category-l2-empty">
            该分类暂无细分
          </p>
        ) : null}
        {categories.error ? (
          <p className="meta" data-testid="category-hint">
            分类暂时读不到，可以先用其它筛选。
          </p>
        ) : null}
      </FilterGroup>

      {/* 作者只留「已选」的清除入口：几十个作者横向滑很难找人，候选留给 PC 工具栏 */}
      {authorLabel === null ? null : (
        <div className="shelf-filter-selected">
          <button
            type="button"
            className="shelf-filter-chip is-active"
            data-testid="author-selected-clear"
            aria-label={"清除作者筛选：" + authorLabel}
            onClick={() => onChooseAuthor(null)}
          >
            作者：{authorLabel} ×
          </button>
        </div>
      )}

      {collections.length === 0 ? null : (
        <FilterGroup
          id="collection"
          title="书单"
          value={collectionNameOf(collections, collection)}
          open={open.has("collection")}
          onToggle={() => toggle("collection")}
        >
          <CountedChips
            allTestId="collection-option-all"
            allActive={collection === null}
            onAll={() => onChooseCollection(null)}
            options={collections}
            activeKey={collection}
            testIdPrefix="collection-option-"
            onChoose={onChooseCollection}
          />
        </FilterGroup>
      )}

      {tags.length === 0 ? null : (
        <FilterGroup
          id="tag"
          title="标签"
          value={tag === null ? "全部" : "标签：" + tag}
          open={open.has("tag")}
          onToggle={() => toggle("tag")}
        >
          <div className="shelf-filter-options">
            <FilterChip testId="tag-option-all" active={tag === null} onClick={() => onChooseTag(null)}>
              全部
            </FilterChip>
            {tags.map((item) => (
              <FilterChip
                key={item.id}
                testId={"tag-option-" + item.id}
                active={tag === item.name}
                onClick={() => onChooseTag(item.name)}
              >
                {item.name}（{item.bookCount}）
              </FilterChip>
            ))}
          </div>
        </FilterGroup>
      )}

      <div className="shelf-filter-panel-foot">
        <button
          className="btn btn-primary shelf-filter-view"
          type="button"
          data-testid="panel-view-results"
          onClick={onViewResults}
        >
          {resultCount === null ? "查看结果" : "查看 " + resultCount + " 本"}
        </button>
      </div>
    </section>
  );
}

/**
 * 带计数的标签：「在读」→「在读 37」。计数缺失（还没取回 / 取失败）就只写标签本身，
 * 补一个占位的 0 会把「不知道」说成「没有」。
 */
function withCount(label: string, count: number | undefined): string {
  return count === undefined ? label : label + " " + count;
}

/** 定长候选（书单）：一条「全部」+ 每项带接口计数，选中用 aria-pressed 暴露给读屏。 */
function CountedChips({
  allTestId,
  allActive,
  onAll,
  options,
  activeKey,
  testIdPrefix,
  onChoose,
}: {
  allTestId: string;
  allActive: boolean;
  onAll: () => void;
  options: CountedOption[];
  activeKey: number | null;
  testIdPrefix: string;
  onChoose: (next: number | null) => void;
}) {
  return (
    <div className="shelf-filter-options">
      <FilterChip testId={allTestId} active={allActive} onClick={onAll}>
        全部
      </FilterChip>
      {options.map((item) => (
        <FilterChip
          key={item.id}
          testId={testIdPrefix + item.id}
          active={activeKey === item.id}
          onClick={() => onChoose(item.id)}
        >
          {item.name}（{item.bookCount}）
        </FilterChip>
      ))}
    </div>
  );
}
