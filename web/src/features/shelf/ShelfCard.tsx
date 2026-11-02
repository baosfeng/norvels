import { Link } from "react-router-dom";
import type { BookSummary, ReadStatus, ReadingProgress } from "../../api/types";
import { Cover } from "../../components/Cover";
import { formatCount } from "../../lib/format";
import { READ_STATUS_LABELS } from "../../lib/readStatus";
import type { ShelfFromState } from "../../lib/shelfCache";
import { stripTags } from "../../lib/text";
import { CardProgress } from "./CardProgress";

/** 追更角标：章数太多时「+1200」比卡片还宽，退化成「更新」两个字。 */
function updateBadgeText(newChapters: number): string {
  return newChapters > 99 ? "更新" : `+${newChapters}`;
}

/**
 * 书卡。两档同一套 DOM，只由 CSS 换形态：
 * - PC：封面在左、书名/作者/章字/简介在右的横排卡（角标是标题行右端的小 chip，点一下循环切换状态）。
 * - H5：三列封面网格，封面铺满格子，下面就是三要素——书名（2 行截断）、作者（1 行）、进度（1 行）。
 *   阅读状态 chip 与追更角标都浮在封面顶角（各自 44px 热区），不占正文那三行。
 *
 * 角标不套在链接里会破坏"整卡可点"，所以状态 chip 仍放在链接内容里，用 preventDefault 拦住跳转；
 * 追更角标是纯信息，放在链接外面并 pointer-events: none，点在它上面照样进详情页。
 * 多选模式下整张卡片变成选择按钮（同样不嵌套链接），右上角显示勾选状态，追更角标让位。
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
  /** 本书的阅读进度（H5 才有）；没有进度时卡片只报章数 */
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

  const newChapters = book.newChapters ?? 0;
  // 追更角标只在 H5 网格卡出现：PC 卡的信息行里已经有「新章 +N」的 pill，不重复说两遍。
  const updateBadge = compact && !selecting && newChapters > 0 ? (
    <span
      className="book-update-badge"
      data-testid={`book-update-${book.id}`}
      title={`有 ${newChapters} 章更新`}
    >
      <span aria-hidden="true">{updateBadgeText(newChapters)}</span>
      <span className="sr-only">{`有 ${newChapters} 章更新`}</span>
    </span>
  ) : null;

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
        {compact ? (
          <CardProgress bookId={book.id} chapterCount={book.chapterCount} progress={progress} />
        ) : (
          <p className="book-meta">
            <span className="num">{formatCount(book.chapterCount)} 章</span>
            <span className="num">{formatCount(book.wordCount)} 字</span>
            {book.volumeCount > 0 ? <span>{book.volumeCount} 卷</span> : null}
            {newChapters > 0 ? (
              <span className="pill is-live" data-testid={`book-new-chapters-${book.id}`}>
                新章 +{newChapters}
              </span>
            ) : null}
          </p>
        )}
        {compact ? null : <p className="book-intro">{stripTags(book.intro) || "这本书还没有简介。"}</p>}
      </div>
    </>
  );

  return (
    <article
      className={`book-card${selecting ? " is-selecting" : ""}${selected ? " is-selected" : ""}`}
      data-status={status}
      data-book-id={book.id}
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
      {updateBadge}
      {selecting ? (
        <span className="book-check" aria-hidden="true">
          {selected ? "✓" : ""}
        </span>
      ) : null}
    </article>
  );
}
