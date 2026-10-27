import { Link } from "react-router-dom";
import type { BookSummary, ReadStatus } from "../../api/types";
import { Cover } from "../../components/Cover";
import { formatCount } from "../../lib/format";
import { READ_STATUS_LABELS } from "../../lib/readStatus";
import type { ShelfFromState } from "../../lib/shelfCache";
import { stripTags } from "../../lib/text";

/**
 * 书卡：角标是独立按钮（不套在链接里，避免交互元素嵌套），点一下循环切换阅读状态。
 * 多选模式下整张卡片变成选择按钮（同样不嵌套链接），右上角显示勾选状态。
 */
export function ShelfCard({
  book,
  compact,
  status,
  onCycle,
  selecting,
  selected,
  onSelect,
  onOpen,
}: {
  book: BookSummary;
  compact: boolean;
  status: ReadStatus;
  onCycle: () => void;
  selecting: boolean;
  selected: boolean;
  onSelect: () => void;
  onOpen: () => void;
}) {
  // 角标放在底部元信息行里（静态排布），不再绝对定位压书名
  const badge = (
    <button
      className="book-status"
      type="button"
      data-testid={`book-status-${book.id}`}
      data-od-id="C-31"
      data-status={status}
      aria-label={`阅读状态：${READ_STATUS_LABELS[status]}，点击切换`}
      onClick={(event) => {
        // 角标在书卡链接的内容里：不拦默认与冒泡就会连带跳到详情页（走查 P1）。
        event.preventDefault();
        event.stopPropagation();
        onCycle();
      }}
    >
      {READ_STATUS_LABELS[status]}
    </button>
  );

  const content = (
    <>
      <Cover
        title={book.title}
        bookId={book.id}
        updatedAt={book.updatedAt}
        hasCover={book.hasCover}
        showLabel={!compact}
      />
      <div className="book-card-body">
        <h3 className="book-title">{book.title}</h3>
        <p className="book-author">{book.authorName || "佚名"}</p>
        <p className="book-meta">
          <span className="num">{formatCount(book.chapterCount)} 章</span>
          <span className="num">{formatCount(book.wordCount)} 字</span>
          {book.volumeCount > 0 ? <span>{book.volumeCount} 卷</span> : null}
          {(book.newChapters ?? 0) > 0 ? (
            <span className="pill is-live" data-testid={`book-new-chapters-${book.id}`}>
              新章 +{book.newChapters}
            </span>
          ) : null}
          {selecting ? null : badge}
        </p>
        {compact ? null : <p className="book-intro">{stripTags(book.intro) || "这本书还没有简介。"}</p>}
      </div>
    </>
  );

  return (
    <article
      className={`book-card${selecting ? " is-selecting" : ""}${selected ? " is-selected" : ""}`}
      data-status={status}
      data-book-id={book.id}
      data-od-id="C-30"
    >
      {selecting ? (
        <button
          className="book-link"
          type="button"
          aria-pressed={selected}
          aria-label={`选择《${book.title}》`}
          data-testid={`book-select-${book.id}`}
          onClick={onSelect}
        >
          {content}
        </button>
      ) : (
        <Link
          className="book-link"
          to={`/book/${book.id}`}
          state={{ from: "shelf", scrollTop: Math.round(typeof window === "undefined" ? 0 : window.scrollY || 0) } satisfies ShelfFromState}
          onClick={onOpen}
        >
          {content}
        </Link>
      )}
      {selecting ? (
        <span className="book-check" aria-hidden="true">
          {selected ? "✓" : ""}
        </span>
      ) : null}
    </article>
  );
}
