import { Link } from "react-router-dom";
import type { BookSummary, ReadStatus, ReadingProgress } from "../../api/types";
import { Cover } from "../../components/Cover";
import { formatCount } from "../../lib/format";
import { READ_STATUS_LABELS } from "../../lib/readStatus";
import type { ShelfFromState } from "../../lib/shelfCache";
import { stripTags } from "../../lib/text";
import { CardProgress } from "./CardProgress";

/**
 * 书卡：阅读状态是标题行右侧的小 chip（点击区仍 44px，视觉只占一行），点一下循环切换。
 * 角标不套在链接里会破坏"整卡可点"，所以仍放在链接内容里，用 preventDefault 拦住跳转。
 * 多选模式下整张卡片变成选择按钮（同样不嵌套链接），右上角显示勾选状态。
 *
 * 阅读进度只在调用方给了 progress 时出现（H5）：没读过（percent=0）或读不到都不占位。
 */
export function ShelfCard({
  book,
  compact,
  status,
  progress,
  onCycle,
  selecting,
  selected,
  onSelect,
  onOpen,
}: {
  book: BookSummary;
  compact: boolean;
  status: ReadStatus;
  /** 本书的阅读进度（H5 才有）；没有就不显示进度行 */
  progress?: ReadingProgress;
  onCycle: () => void;
  selecting: boolean;
  selected: boolean;
  onSelect: () => void;
  onOpen: () => void;
}) {
  const badge = (
    <button
      className="book-status"
      type="button"
      data-testid={`book-status-${book.id}`}
      data-od-id="C-31"
      data-status={status}
      aria-label={`阅读状态：${READ_STATUS_LABELS[status]}，点击切换`}
      onClick={(event) => {
        // 角标在书卡链接的内容里：不拦默认与冒泡就会连带跳到详情页。
        event.preventDefault();
        event.stopPropagation();
        onCycle();
      }}
    >
      <span className="book-status-chip">{READ_STATUS_LABELS[status]}</span>
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
        <div className="book-title-row">
          <h3 className="book-title">{book.title}</h3>
          {selecting ? null : badge}
        </div>
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
        </p>
        {progress && progress.percent > 0 ? <CardProgress progress={progress} /> : null}
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
