import type { RefObject } from "react";
import type { ReadStatus } from "../../api/types";
import { READ_STATUS_FILTERS } from "../../lib/readStatus";
import { CategoryFilter, type CategoryValue } from "./CategoryFilter";
import { CollectionFilter } from "./CollectionFilter";
import { SORTS, type SortValue } from "./shelfSorts";

type AuthorOption = { id: number; name: string; bookCount: number };
type TagOption = { id: number; name: string };

/** PC 工具栏（C-19/C-22/C-23/C-24/C-28/C-29）：排序、四类筛选、标签 chips、清除筛选。 */
export function ShelfToolbar({
  sortRef,
  sort,
  onChooseSort,
  readStatus,
  onChooseReadStatus,
  rating,
  onRatingChange,
  authorId,
  onAuthorChange,
  authors,
  collection,
  onCollectionChange,
  category,
  onCategoryChange,
  tag,
  onTagChange,
  tags,
  filtering,
  onResetFilters,
}: {
  sortRef: RefObject<HTMLSelectElement | null>;
  sort: SortValue;
  onChooseSort: (next: SortValue) => void;
  readStatus: Exclude<ReadStatus, ""> | null;
  onChooseReadStatus: (next: Exclude<ReadStatus, ""> | null) => void;
  rating: number | null;
  onRatingChange: (next: number | null) => void;
  authorId: number | null;
  onAuthorChange: (next: number | null) => void;
  authors: AuthorOption[];
  collection: number | null;
  onCollectionChange: (next: number | null) => void;
  category: CategoryValue;
  onCategoryChange: (next: CategoryValue) => void;
  tag: string | null;
  onTagChange: (next: string | null) => void;
  tags: TagOption[];
  filtering: boolean;
  onResetFilters: () => void;
}) {
  return (
    <div className="toolbar" data-testid="shelf-toolbar">
      <label>
        排序
        <select
          ref={sortRef}
          className="select"
          value={sort}
          data-od-id="C-19"
          onChange={(e) => onChooseSort(e.target.value as SortValue)}
        >
          {SORTS.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </select>
      </label>

      <label>
        阅读状态
        <select
          className="select"
          value={readStatus ?? ""}
          data-testid="filter-read-status"
          data-od-id="C-22"
          onChange={(e) => onChooseReadStatus((e.target.value || null) as Exclude<ReadStatus, ""> | null)}
        >
          {READ_STATUS_FILTERS.map((option) => (
            <option key={option.label} value={option.value ?? ""}>
              {option.label}
            </option>
          ))}
        </select>
      </label>

      <label>
        评分
        <select
          className="select"
          value={rating ?? ""}
          data-testid="filter-rating"
          data-od-id="C-23"
          onChange={(e) => onRatingChange(e.target.value ? Number(e.target.value) : null)}
        >
          <option value="">全部评分</option>
          {[5, 4, 3, 2, 1].map((star) => (
            <option key={star} value={star}>
              {star} 星
            </option>
          ))}
        </select>
      </label>

      <label>
        作者
        <select
          className="select"
          value={authorId ?? ""}
          onChange={(e) => onAuthorChange(e.target.value ? Number(e.target.value) : null)}
          data-testid="filter-author"
          data-od-id="C-24"
        >
          <option value="">全部作者</option>
          {authors.map((author) => (
            <option key={author.id} value={author.id}>
              {author.name || "佚名"}（{author.bookCount}）
            </option>
          ))}
        </select>
      </label>

      <label>
        书单
        <CollectionFilter value={collection} onChange={onCollectionChange} />
      </label>

      <CategoryFilter value={category} onChange={onCategoryChange} />

      <div className="chips" role="group" aria-label="标签筛选" data-testid="filter-tags" data-od-id="C-28">
        <button type="button" className={`chip${tag === null ? " is-active" : ""}`} onClick={() => onTagChange(null)}>
          全部
        </button>
        {tags.map((item) => (
          <button
            key={item.id}
            type="button"
            className={`chip${tag === item.name ? " is-active" : ""}`}
            onClick={() => onTagChange(item.name)}
          >
            {item.name}
          </button>
        ))}
      </div>

      {filtering ? (
        <button className="btn btn-ghost" type="button" data-od-id="C-29" onClick={onResetFilters}>
          清除筛选
        </button>
      ) : null}
      {rating !== null ? (
        <span className="meta" data-testid="rating-filter-hint">
          只列已评分的书（0 星 = 未评，不参与评分筛选与按评分排序）。
        </span>
      ) : null}
    </div>
  );
}
