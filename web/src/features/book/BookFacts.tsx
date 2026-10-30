import { Link } from "react-router-dom";
import type { BookDetail, ReadStatus } from "../../api/types";
import { formatCount, formatDate } from "../../lib/format";
import { READ_STATUS_OPTIONS } from "../../lib/readStatus";
import { BookRating } from "./RatingStars";

/**
 * 详情页的书籍事实区（C-06 / C-07 / C-08 + 元信息）：字数、导入时间、系列、阅读状态、评分、
 * 书签、分类、出版信息。
 *
 * 两档都渲染：PC 放在左栏（`BookAside`），H5 放在主栏书名与简介之后。
 * 位置由 `useTier()` 决定（`BookPage` 条件渲染），**不是** CSS 隐藏——
 * 曾经 H5 用 `@media .book-aside .facts{display:none}` 把整块藏掉，手机上就看不到字数/分类/系列了。
 */
export function BookFacts({
  book,
  readStatus,
  savingStatus,
  onReadStatusChange,
}: {
  book: BookDetail;
  readStatus: string;
  savingStatus: boolean;
  onReadStatusChange: (next: ReadStatus) => void;
}) {
  return (
    <dl className="facts" data-testid="book-facts">
      <div>
        <dt>字数</dt>
        <dd className="num">{formatCount(book.wordCount)}</dd>
      </div>
      <div>
        <dt>导入时间</dt>
        <dd className="num">{formatDate(book.importedAt)}</dd>
      </div>
      {book.series ? <SeriesFact series={book.series} seriesIndex={book.seriesIndex} /> : null}
      <div>
        <dt>
          <label htmlFor="book-read-status">阅读状态</label>
        </dt>
        <dd>
          <select
            className={`select${savingStatus ? " is-saving" : ""}`}
            id="book-read-status"
            data-testid="book-read-status"
            data-od-id="C-07"
            value={readStatus}
            disabled={savingStatus}
            onChange={(event) => onReadStatusChange(event.target.value as ReadStatus)}
          >
            {READ_STATUS_OPTIONS.map((option) => (
              <option key={option.value || "none"} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </dd>
      </div>
      {savingStatus ? (
        <p className="inline-note" data-testid="book-read-status-note">
          正在保存上一次选择，读完再改。
        </p>
      ) : null}
      <div>
        <dt>评分</dt>
        <dd>
          <BookRating book={book} />
        </dd>
      </div>
      {typeof book.bookmarkCount === "number" && book.bookmarkCount > 0 ? (
        <div>
          <dt>书签</dt>
          <dd className="num" data-testid="book-bookmark-count">
            {formatCount(book.bookmarkCount)} 条
          </dd>
        </div>
      ) : null}
      {book.categoryL1 ? (
        <div>
          <dt>分类</dt>
          <dd data-testid="book-category">
            {book.categoryL1}
            {book.categoryL2 ? ` / ${book.categoryL2}` : ""}
          </dd>
        </div>
      ) : null}
      {book.publisher || book.pubDate ? (
        <div>
          <dt>出版</dt>
          <dd>{[book.publisher, book.pubDate].filter(Boolean).join(" · ")}</dd>
        </div>
      ) : null}
    </dl>
  );
}

/** 系列行：系列名可点（C-06），部数仍按纯文本跟在后面。 */
function SeriesFact({ series, seriesIndex }: { series: string; seriesIndex?: number }) {
  return (
    <div>
      <dt>系列</dt>
      <dd>
        <Link className="fact-link" to={`/series/${encodeURIComponent(series)}`} data-od-id="C-06">
          {series}
        </Link>
        {seriesIndex ? ` · 第 ${seriesIndex} 部` : ""}
      </dd>
    </div>
  );
}
