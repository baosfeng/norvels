import type { RefObject } from "react";
import { Link } from "react-router-dom";
import type { BookDetail, ChapterBrief, ReadStatus } from "../../api/types";
import { Cover } from "../../components/Cover";
import { formatCount, formatDate } from "../../lib/format";
import { READ_STATUS_LABELS, READ_STATUS_OPTIONS } from "../../lib/readStatus";
import { BookRating } from "./RatingStars";

/**
 * 详情页左栏（封面 + 操作 + 元信息）。
 * 锚点：C-02 继续阅读、C-03 从第一章开始、C-04 加入书单、C-05 管理这本书（仅 PC）、
 * C-06 系列名链接（新增）、C-07 阅读状态、C-08 评分（在 BookRating 里）。
 */
export function BookAside({
  book,
  resume,
  first,
  isH5,
  readStatus,
  savingStatus,
  collectionButtonRef,
  onOpenCollection,
  onReadStatusChange,
}: {
  book: BookDetail;
  resume: { chapterId: number; chapterTitle: string } | null;
  first: ChapterBrief | null;
  isH5: boolean;
  readStatus: string;
  savingStatus: boolean;
  collectionButtonRef: RefObject<HTMLButtonElement | null>;
  onOpenCollection: () => void;
  onReadStatusChange: (next: ReadStatus) => void;
}) {
  const target = resume ?? (first ? { chapterId: first.id, chapterTitle: first.title } : null);

  return (
    <aside className="book-aside" data-od-id="book-aside">
      <Cover
        title={book.title}
        bookId={book.id}
        updatedAt={book.updatedAt}
        hasCover={book.hasCover}
        showLabel={!isH5}
      />
      <div className="actions">
        {target ? (
          <Link className="btn btn-primary" to={`/read/${book.id}/${target.chapterId}`} data-od-id="C-02">
            继续阅读 · {target.chapterTitle}
          </Link>
        ) : null}
        {first ? (
          <Link className="btn" to={`/read/${book.id}/${first.id}`} data-od-id="C-03">
            从第一章开始
          </Link>
        ) : null}
        <button
          ref={collectionButtonRef}
          className="btn btn-ghost"
          type="button"
          data-testid="book-add-collection"
          data-od-id="C-04"
          onClick={onOpenCollection}
        >
          加入书单
        </button>
        {/* 详情页只读：改信息、传封面、改章节、删书都在独立的书籍管理页（/admin） */}
        {isH5 ? null : (
          <Link
            className="btn btn-ghost"
            to={`/admin/books/${book.id}`}
            data-testid="book-admin-link"
            data-od-id="C-05"
          >
            管理这本书
          </Link>
        )}
      </div>
      <dl className="facts">
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
    </aside>
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

/** 阅读状态的人话（保存提示用）；后端值缺失时不编造。 */
export function readStatusLabel(value: string): string {
  return READ_STATUS_LABELS[value as ReadStatus] ?? "未标记";
}
