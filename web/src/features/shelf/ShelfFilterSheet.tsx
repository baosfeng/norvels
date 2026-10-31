import { useEffect, useRef, type ReactNode, type RefObject } from "react";
import type { ReadStatus } from "../../api/types";
import { FOCUSABLE_SELECTOR, useFocusTrap } from "../../lib/focusTrap";
import { READ_STATUS_FILTERS } from "../../lib/readStatus";
import type { CategoryValue } from "./CategoryFilter";
import { leavesOf, useCategoryOptions } from "./categoryOptions";
import { SORTS, type SortValue } from "./shelfSorts";

/** 弹层打开时先停在哪一屏：入口「排序」落排序分区，「筛选」落第一个筛选分区 */
export type ShelfSheetSection = "sort" | "filters";

export interface ShelfFilterSheetProps {
  /** 从哪个入口打开：决定初始滚动位置与焦点落点 */
  initialSection: ShelfSheetSection;
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
 * 形态：每个分区一行标题 + **换行铺满的 chip**——选项少的一眼看完，不用先展开才知道有什么。
 * 分类两级都铺开（大分类全量 + 计数，选了大分类后紧跟它的全部小分类），不占用原生下拉。
 * 作者块在 H5 去掉：几十个作者 chip 横着滑很久也找不到人；只有地址里带着 authorId 时，
 * 才给一行「作者：名字 ×」，让读者能就地取消（否则深链进来的作者筛选在窄屏上没有出口）。
 *
 * 交互：选中即生效（列表在背后实时更新），弹层保持打开，让用户连续调好几项；
 * 底部「查看 N 本」收工。Esc / 遮罩 / × 关闭，关闭后焦点回到入口按钮（`useFocusTrap`）。
 * 从哪个入口点开就停在哪一屏（`initialSection`）：点「筛选」直接落在阅读状态，不用先滑过排序。
 */
export function ShelfFilterSheet({
  initialSection,
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
  useInitialSection(sheetRef, initialSection);
  const categories = useCategoryOptions();
  const leaves = leavesOf(categories.nodes, category.l1);
  const showLeaves = category.l1 !== null;
  const authorName = authorId === null ? null : authorLabel(authors, authorId);

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
          <SheetSection title="排序" anchor="sort">
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

          <SheetSection title="阅读状态" anchor="filters">
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
            <div className="sheet-options is-wrap" data-testid="category-l1-options">
              <Chip
                testId="category-option-l1-all"
                active={category.l1 === null}
                onClick={() => onChooseCategory({ l1: null, l2: null })}
              >
                全部
              </Chip>
              {categories.nodes.map((node) => (
                <Chip
                  key={node.l1}
                  testId={`category-option-l1-${node.l1}`}
                  active={category.l1 === node.l1}
                  onClick={() => onChooseCategory({ l1: node.l1, l2: null })}
                >
                  {node.l1}（{node.count}）
                </Chip>
              ))}
            </div>
            {/* 小分类从属于选中的大分类：左侧一道竖线缩进，表示它只对上面那一项生效 */}
            {showLeaves && leaves.length > 0 ? (
              <div className="sheet-sub" data-testid="category-l2-options">
                <span className="sheet-sub-title">小分类</span>
                <div className="sheet-options is-wrap">
                  <Chip
                    testId="category-option-l2-all"
                    active={category.l2 === null}
                    onClick={() => onChooseCategory({ l1: category.l1, l2: null })}
                  >
                    全部
                  </Chip>
                  {leaves.map((leaf) => (
                    <Chip
                      key={leaf.l2}
                      testId={`category-option-l2-${leaf.l2}`}
                      active={category.l2 === leaf.l2}
                      onClick={() => onChooseCategory({ l1: category.l1, l2: leaf.l2 })}
                    >
                      {leaf.l2}（{leaf.count}）
                    </Chip>
                  ))}
                </div>
              </div>
            ) : null}
            {showLeaves && leaves.length === 0 ? (
              <p className="meta sheet-sub" data-testid="category-l2-empty">
                该分类暂无细分
              </p>
            ) : null}
            {categories.error ? (
              <p className="meta" data-testid="category-hint">
                分类暂时读不到，可以先用其它筛选。
              </p>
            ) : null}
          </SheetSection>

          {/* 作者只留「已选」的清除入口：H5 上几十个作者横向滑很难找到人，候选留给 PC 工具栏 */}
          {authorName === null ? null : (
            <div className="sheet-selected">
              <button
                type="button"
                className="sheet-option is-active"
                data-testid="author-selected-clear"
                aria-label={`清除作者筛选：${authorName}`}
                onClick={() => onChooseAuthor(null)}
              >
                作者：{authorName} ×
              </button>
            </div>
          )}

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

/** 已选作者的显示名：候选表还没回来（或该作者不在候选里）时不编名字，如实写「已选作者」。 */
function authorLabel(authors: ShelfFilterSheetProps["authors"], authorId: number): string {
  return authors.find((author) => author.id === authorId)?.name || "已选作者";
}

/**
 * 打开时把目标分区滚到顶、焦点交给它第一个可点控件。
 * 焦点陷阱默认停在头部的「清除筛选 / ×」，从那里起步等于让用户自己往下找——入口已经说明了意图，直接跳过去。
 */
function useInitialSection(ref: RefObject<HTMLDivElement | null>, section: ShelfSheetSection) {
  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    const target = root.querySelector<HTMLElement>(`[data-sheet-section="${section}"]`);
    if (!target) return;
    target.scrollIntoView?.({ block: "start" });
    target.querySelector<HTMLElement>(FOCUSABLE_SELECTOR)?.focus();
  }, [ref, section]);
}

function SheetSection({
  title,
  anchor,
  children,
}: {
  title: string;
  anchor?: ShelfSheetSection;
  children: ReactNode;
}) {
  return (
    <div
      className="sheet-section"
      role="group"
      aria-label={`${title}筛选`}
      data-sheet-section={anchor}
      data-testid={anchor ? `sheet-section-${anchor}` : undefined}
    >
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
