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
  /** 当前筛选命中的书数：底部「查看 N 本」用；加载中拿不到就给 null，改显示「完成」 */
  resultCount: number | null;
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

const RATINGS = [5, 4, 3, 2, 1] as const;

/**
 * H5 书架底部弹层：排序 + **PC 的全部筛选**（阅读状态 / 评分 / 作者 / 书单 / 分类 / 标签）。
 *
 * 口径：PC 能筛的 H5 也能筛。分类选项与 PC 下拉同源（`useCategoryOptions`），
 * 筛出来的值同样走 `useBookFeed` 的查询参数，不存在「H5 版另起一套」的可能。
 *
 * 形态：每个分区一行标题 + **横排 chip**（排序 / 状态 / 评分单行左右滑，作者 / 书单 / 标签换行），
 * 所以七类筛选不再堆成一条要滚很久的长列表；分类仍是两个原生下拉，两级联动一步到位。
 *
 * 交互：选中即生效（列表在背后实时更新），弹层保持打开，让用户连续调好几项；
 * 底部「查看 N 本」收工。Esc / 遮罩 / × 关闭，关闭后焦点回到入口按钮（`useFocusTrap`）。
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
  resultCount,
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
        className="sheet shelf-filter-sheet"
        role="dialog"
        aria-modal="true"
        aria-label="排序与筛选"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="sheet-head">
          <strong>排序与筛选</strong>
          <div className="sheet-head-actions">
            <button
              className="btn btn-ghost"
              type="button"
              data-testid="sheet-reset-filters"
              data-od-id="C-29"
              disabled={!filtering}
              onClick={onResetFilters}
            >
              清除筛选
            </button>
            <button className="icon-btn sheet-close" type="button" aria-label="关闭筛选" onClick={onClose}>
              ×
            </button>
          </div>
        </div>

        <div className="sheet-scroll">
          <SheetSection title="排序">
            <div className="sheet-options is-triple">
              {SORTS.map((option) => (
                <Chip
                  key={option.value}
                  testId={`sort-option-${option.value}`}
                  active={option.value === sort}
                  onClick={() => onChooseSort(option.value)}
                >
                  {option.label}
                </Chip>
              ))}
            </div>
          </SheetSection>

          <SheetSection title="阅读状态">
            <div className="sheet-options is-wrap">
              {READ_STATUS_FILTERS.map((option) => (
                <Chip
                  key={option.label}
                  testId={`read-status-option-${option.value ?? "all"}`}
                  active={option.value === readStatus}
                  onClick={() => onChooseReadStatus(option.value)}
                >
                  {option.label}
                </Chip>
              ))}
            </div>
          </SheetSection>

          <SheetSection title="评分">
            <div className="sheet-options is-grid">
              <Chip testId="rating-option-all" active={rating === null} onClick={() => onChooseRating(null)}>
                全部
              </Chip>
              {RATINGS.map((star) => (
                <Chip
                  key={star}
                  testId={`rating-option-${star}`}
                  active={rating === star}
                  onClick={() => onChooseRating(star)}
                >
                  {star} 星
                </Chip>
              ))}
            </div>
          </SheetSection>

          <SheetSection title="分类">
            <div className="sheet-pair">
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
            </div>
            {categories.error ? (
              <p className="meta" data-testid="category-hint">
                分类暂时读不到，可以先用其它筛选。
              </p>
            ) : null}
          </SheetSection>

          {authors.length > 0 ? (
            <SheetSection title="作者">
              <div className="sheet-options is-row">
                <Chip testId="author-option-all" active={authorId === null} onClick={() => onChooseAuthor(null)}>
                  全部
                </Chip>
                {authors.map((author) => (
                  <Chip
                    key={author.id}
                    testId={`author-option-${author.id}`}
                    active={authorId === author.id}
                    onClick={() => onChooseAuthor(author.id)}
                  >
                    {author.name || "佚名"}（{author.bookCount}）
                  </Chip>
                ))}
              </div>
            </SheetSection>
          ) : null}

          {collections.length > 0 ? (
            <SheetSection title="书单">
              <div className="sheet-options is-row">
                <Chip
                  testId="collection-option-all"
                  active={collection === null}
                  onClick={() => onChooseCollection(null)}
                >
                  全部
                </Chip>
                {collections.map((item) => (
                  <Chip
                    key={item.id}
                    testId={`collection-option-${item.id}`}
                    active={collection === item.id}
                    onClick={() => onChooseCollection(item.id)}
                  >
                    {item.name}（{item.bookCount}）
                  </Chip>
                ))}
              </div>
            </SheetSection>
          ) : null}

          {tags.length > 0 ? (
            <SheetSection title="标签">
              <div className="sheet-options is-row">
                <Chip testId="tag-option-all" active={tag === null} onClick={() => onChooseTag(null)}>
                  全部
                </Chip>
                {tags.map((item) => (
                  <Chip
                    key={item.id}
                    testId={`tag-option-${item.id}`}
                    active={tag === item.name}
                    onClick={() => onChooseTag(item.name)}
                  >
                    {item.name}
                  </Chip>
                ))}
              </div>
            </SheetSection>
          ) : null}
        </div>

        <div className="sheet-foot">
          <button className="btn btn-primary sheet-done" type="button" data-testid="sheet-done" onClick={onClose}>
            {resultCount === null ? "完成" : `查看 ${resultCount} 本`}
          </button>
        </div>
      </div>
    </div>
  );
}

function SheetSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="sheet-section" role="group" aria-label={`${title}筛选`}>
      <span className="sheet-section-title">{title}</span>
      {children}
    </div>
  );
}

function Chip({
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
