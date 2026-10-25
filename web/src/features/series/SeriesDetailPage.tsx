import { useMemo } from "react";
import { Link, useParams } from "react-router-dom";
import { useSeriesDetail } from "../../api/hooks";
import type { BookSummary } from "../../api/types";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount, formatDate } from "../../lib/format";
import { sortSeriesBooks } from "../../lib/seriesGroup";

/**
 * 系列详情（P3 发现域，PC 专属）：系列名由路由参数给（useParams 已解码，不再二次 decode）。
 * 书目卡片复用书架的 book-grid / book-card 与 Cover，阅读顺序按 seriesIndex 排（没标序号的排最后）。
 */
export function SeriesDetailPage() {
  const { name } = useParams();
  const series = (name ?? "").trim();
  const feed = useSeriesDetail(series || null);
  const books = useMemo(() => sortSeriesBooks(feed.data?.books ?? []), [feed.data]);
  const words = books.reduce((sum, book) => sum + (Number.isFinite(book.wordCount) ? book.wordCount : 0), 0);

  return (
    <section data-testid="view:series-detail" data-od-id="series-detail">
      <nav className="crumbs" aria-label="面包屑">
        <Link className="crumb-link" to="/series">
          系列
        </Link>
        <span aria-hidden="true">/</span>
        <span className="meta">{series || "系列"}</span>
      </nav>

      <div className="page-head">
        <div>
          <p className="eyebrow">系列 · {feed.loading ? "读取中" : `${formatCount(books.length)} 本`}</p>
          <h1>{series || "系列"}</h1>
          <p className="sub">
            {books.length > 0 ? `本系列合计 ${formatCount(words)} 字 · 按系列序号排列` : "同一系列的书会聚合到这里"}
          </p>
        </div>
      </div>

      {feed.loading ? (
        <LoadingPanel title="正在打开系列" description="正在取回这个系列的书目。" />
      ) : feed.error ? (
        feed.error.status === 404 ? (
          <EmptyPanel
            title="没有这个系列"
            description="系列名可能改了，或者这个系列下的书都换到了别的系列。"
            testId="series-detail-empty"
            actions={
              <Link className="btn btn-primary" to="/series">
                回系列列表
              </Link>
            }
          />
        ) : (
          <ErrorPanel error={feed.error} title="系列没能打开" onRetry={feed.reload} testId="series-detail-error" />
        )
      ) : books.length === 0 ? (
        <EmptyPanel title="这个系列还没有书" description="系列里的书被移走或改了系列名之后，这里会空下来。" testId="series-detail-empty" />
      ) : (
        <div className="book-grid" data-testid="series-detail-live">
          {books.map((book, index) => (
            <SeriesBookCard key={book.id} book={book} order={index + 1} />
          ))}
        </div>
      )}
    </section>
  );
}

/** 系列内的一本书：与书架卡片同一套结构与样式，只多一个「第 N 部」的位置提示。 */
function SeriesBookCard({ book, order }: { book: BookSummary; order: number }) {
  return (
    <article className="book-card" data-book-id={book.id} data-testid={`series-book-${book.id}`}>
      <Link className="book-link" to={`/book/${book.id}`}>
        <Cover
          title={book.title}
          bookId={book.id}
          updatedAt={book.updatedAt}
          hasCover={book.hasCover}
        />
        <div className="book-card-body">
          <h3 className="book-title">{book.title}</h3>
          <p className="book-author">
            {book.authorName || "佚名"}
            {book.seriesIndex && book.seriesIndex > 0 ? ` · 第 ${book.seriesIndex} 部` : ""}
          </p>
          <p className="book-meta">
            <span className="num">{formatCount(book.chapterCount)} 章</span>
            <span className="num">{formatCount(book.wordCount)} 字</span>
            <span className="meta">{formatDate(book.updatedAt)}</span>
          </p>
          <p className="book-intro" data-testid={`series-book-order-${book.id}`}>
            系列内第 {order} 本
          </p>
        </div>
      </Link>
    </article>
  );
}
