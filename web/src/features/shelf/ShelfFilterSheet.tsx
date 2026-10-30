import { useRef, type ReactNode } from "react";
import type { ReadStatus } from "../../api/types";
import { useFocusTrap } from "../../lib/focusTrap";
import { READ_STATUS_FILTERS } from "../../lib/readStatus";
import type { CategoryValue } from "./CategoryFilter";
import { leavesOf, useCategoryOptions } from "./categoryOptions";
import { SORTS, type SortValue } from "./shelfSorts";

export interface ShelfFilterSheetProps {
  sort: SortValue;
  readStatus: Exclude<ReadStatus, ""> | null;
  rating: number | null;
  authorId: number | null;
  authors: { id: number; name: string; bookCount: number }[];
  collection: number | null;
  collections: { id: number; name: string; bookCount: number }[];
  category: CategoryValue;
  tag: string | null;
  tags: { id: number; name: string }[];
  filtering: boolean;
  onChooseSort: (next: SortValue) => void;
  onChooseReadStatus: (next: Exclude<ReadStatus, ""> | null) => void;
  onChooseRating: (next: number | null) => void;
  onChooseAuthor: (next: number | null) => void;
  onChooseCollection: (next: number | null) => void;
  onChooseCategory: (next: CategoryValue) => void;
  onChooseTag: (next: string | null) => void;
  onResetFilters: () => void;
  onClose: () => void;
}

/**
 * H5 书架底部弹层（C-21）：排序 + **PC 的全部筛选**（阅读状态 / 评分 / 作者 / 书单 / 分类 / 标签）。
 *
 * 口径：PC 能筛的 H5 也能筛。分类选项与 PC 下拉同源（`useCategoryOptions`），
 * 筛出来的值同样走 `useBookFeed` 的查询参数，不存在「H5 版另起一套」的可能。
 *
 * 交互：焦点陷阱 + Esc + 遮罩关闭（关闭后焦点还给触发按钮），选中即生效并收起。
 * 只在 H5 渲染（调用方按 `useTier()` 判定），不是 CSS 隐藏。
 */
export function ShelfFilterSheet({
  sort,
  readStatus,
  rating,
  authorId,
  authors,
  collection,
  collections,
  category,
  tag,
  tags,
  filtering,
  onChooseSort,
  onChooseReadStatus,
  onChooseRating,
  onChooseAuthor,
  onChooseCollection,
  onChooseCategory,
  onChooseTag,
  onResetFilters,
  onClose,
}: ShelfFilterSheetProps) {
  const sheetRef = useRef<HTMLDivElement | null>(null);
  useFocusTrap(true, sheetRef, onClose);
  const categories = useCategoryOptions();
  const leaves = leavesOf(categories.nodes, category.l1);

  return (
    <div className="sheet-backdrop" data-testid="shelf-sort-sheet" data-od-id="C-21" onClick={onClose}>
      <div
        ref={sheetRef}
        className="sheet"
        role="dialog"
        aria-modal="true"
        aria-label="排序与筛选"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="sheet-head">
          <strong>排序与筛选</strong>
          <button className="btn btn-ghost" type="button" onClick={onClose}>
            关闭
          </button>
        </div>

        <div className="sheet-scroll">
          <Section title="排序">
            {SORTS.map((s) => (
              <Option
                key={s.value}
                testId={`sort-option-${s.value}`}
                active={s.value === sort}
                onClick={() => onChooseSort(s.value)}
              >
                {s.label}
              </Option>
            ))}
          </Section>

          <Section title="阅读状态">
            {READ_STATUS_FILTERS.map((option) => (
              <Option
                key={option.label}
                testId={`read-status-option-${option.value ?? "all"}`}
                active={option.value === readStatus}
                onClick={() => onChooseReadStatus(option.value)}
              >
                {option.label}
              </Option>
            ))}
          </Section>

          <Section title="评分">
            <Option testId="rating-option-all" active={rating === null} onClick={() => onChooseRating(null)}>
              全部评分
            </Option>
            {[5, 4, 3, 2, 1].map((star) => (
              <Option
                key={star}
                testId={`rating-option-${star}`}
                active={rating === star}
                onClick={() => onChooseRating(star)}
              >
                {star} 星
              </Option>
            ))}
          </Section>

          <Section title="分类">
            <label className="sheet-label">
              大分类
              <select
                className="select"
                value={category.l1 ?? ""}
                data-testid="filter-category-l1"
                data-od-id="C-26"
                onChange={(event) => onChooseCategory({ l1: event.target.value || null, l2: null })}
              >
                <option value="">全部分类</option>
                {categories.nodes.map((node) => (
                  <option key={node.l1} value={node.l1}>
                    {node.l1}（{node.count}）
                  </option>
                ))}
              </select>
            </label>
            {leaves.length > 0 ? (
              <label className="sheet-label">
                小分类
                <select
                  className="select"
                  value={category.l2 ?? ""}
                  data-testid="filter-category-l2"
                  data-od-id="C-27"
                  onChange={(event) => onChooseCategory({ l1: category.l1, l2: event.target.value || null })}
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
            {categories.error ? (
              <p className="meta" data-testid="category-hint">
                分类暂时读不到，可以先用其它筛选。
              </p>
            ) : null}
          </Section>

          {authors.length > 0 ? (
            <Section title="作者">
              <Option testId="author-option-all" active={authorId === null} onClick={() => onChooseAuthor(null)}>
                全部作者
              </Option>
              {authors.map((author) => (
                <Option
                  key={author.id}
                  testId={`author-option-${author.id}`}
                  active={authorId === author.id}
                  onClick={() => onChooseAuthor(author.id)}
                >
                  {author.name || "佚名"}（{author.bookCount}）
                </Option>
              ))}
            </Section>
          ) : null}

          {collections.length > 0 ? (
            <Section title="书单">
              <Option
                testId="collection-option-all"
                active={collection === null}
                onClick={() => onChooseCollection(null)}
              >
                全部书单
              </Option>
              {collections.map((item) => (
                <Option
                  key={item.id}
                  testId={`collection-option-${item.id}`}
                  active={collection === item.id}
                  onClick={() => onChooseCollection(item.id)}
                >
                  {item.name}（{item.bookCount}）
                </Option>
              ))}
            </Section>
          ) : null}

          {tags.length > 0 ? (
            <Section title="标签">
              <Option testId="tag-option-all" active={tag === null} onClick={() => onChooseTag(null)}>
                全部标签
              </Option>
              {tags.map((item) => (
                <Option
                  key={item.id}
                  testId={`tag-option-${item.id}`}
                  active={tag === item.name}
                  onClick={() => onChooseTag(item.name)}
                >
                  {item.name}
                </Option>
              ))}
            </Section>
          ) : null}
        </div>

        {filtering ? (
          <div className="sheet-foot">
            <button
              className="btn btn-ghost"
              type="button"
              data-testid="sheet-reset-filters"
              data-od-id="C-29"
              onClick={onResetFilters}
            >
              清除筛选
            </button>
          </div>
        ) : null}
      </div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="sheet-section" role="group" aria-label={`${title}筛选`}>
      <span className="sheet-section-title">{title}</span>
      {children}
    </div>
  );
}

function Option({
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
      className={`sheet-option${active ? " is-active" : ""}`}
      data-testid={testId}
      aria-pressed={active}
      onClick={onClick}
    >
      {children}
    </button>
  );
}
