import type { RefObject } from "react";
import { Link } from "react-router-dom";
import type { ApiError } from "../../api/client";
import type { BookSummary, ReadStatus } from "../../api/types";
import { CardSkeleton, EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
import { ShelfCard } from "./ShelfCard";

type Feed = {
  page: number;
  hasMore: boolean;
  loadingMore: boolean;
  errorMore: unknown;
  loadMore: () => void;
};

/** 书架内容区（C-37/C-38/C-39/C-40/C-41）：加载 / 错误 / 空态 / 列表 + H5 无限滚动或 PC 分页。 */
export function ShelfList({
  isH5,
  loading,
  error,
  items,
  total,
  filtering,
  mode,
  statusOverrides,
  selecting,
  selected,
  feed,
  pages,
  canObserve,
  sentinelRef,
  onRetry,
  onResetFilters,
  onCycle,
  onSelect,
  onOpen,
  onPage,
}: {
  isH5: boolean;
  loading: boolean;
  error: ApiError | null;
  items: BookSummary[];
  total: number;
  filtering: boolean;
  mode: "grid" | "list";
  statusOverrides: Record<number, ReadStatus>;
  selecting: boolean;
  selected: Set<number>;
  feed: Feed;
  pages: number;
  canObserve: boolean;
  sentinelRef: RefObject<HTMLDivElement | null>;
  onRetry: () => void;
  onResetFilters: () => void;
  onCycle: (book: BookSummary) => void;
  onSelect: (id: number) => void;
  onOpen: (id: number) => void;
  onPage: (next: number) => void;
}) {
  if (loading) {
    return (
      <div data-testid="shelf-loading">
        <LoadingPanel
          title="正在读取书架"
          description={`正在整理本地书目，${formatCount(total || 0) || "全部"} 本大约需要 1–2 秒。`}
        />
        <CardSkeleton count={isH5 ? 3 : 4} />
      </div>
    );
  }

  if (error) {
    return (
      <div data-od-id="C-41">
        <ErrorPanel error={error} title="书架没能加载" onRetry={onRetry} testId="shelf-error" />
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div>
        <EmptyPanel
          title={filtering ? "没有符合筛选条件的书" : "书架还是空的"}
          description={
            filtering
              ? "换一个作者或标签试试，或者清除筛选看全部书目。"
              : isH5
                ? "先在电脑上把存放 txt 的目录导入，书目会同步到这里。"
                : "把存放 txt 的目录填进导入页，完成后书目会自动出现在这里。"
          }
          testId="shelf-empty"
          actions={
            filtering ? (
              <button className="btn btn-primary" type="button" data-od-id="C-40" onClick={onResetFilters}>
                清除筛选
              </button>
            ) : isH5 ? null : (
              <Link className="btn btn-primary" to="/import" data-od-id="C-40">
                去导入
              </Link>
            )
          }
        />
      </div>
    );
  }

  return (
    <>
      <div className={`book-grid ${mode === "list" && !isH5 ? "is-list" : ""}`.trim()} data-testid="shelf-live">
        {items.map((book) => (
          <ShelfCard
            key={book.id}
            book={book}
            compact={isH5}
            status={statusOverrides[book.id] ?? book.readStatus ?? ""}
            onCycle={() => onCycle(book)}
            selecting={selecting}
            selected={selected.has(book.id)}
            onSelect={() => onSelect(book.id)}
            onOpen={() => onOpen(book.id)}
          />
        ))}
      </div>

      {isH5 ? (
        <div className="shelf-more" data-testid="shelf-more">
          {feed.loadingMore ? (
            <p className="meta" role="status" data-testid="shelf-more-loading">
              正在加载更多…
            </p>
          ) : null}
          {feed.errorMore ? (
            <div className="shelf-more-error" data-testid="shelf-more-error">
              <p className="meta" role="alert">
                后面的书没取回来，检查网络后再试一次。
              </p>
              <button className="btn" type="button" onClick={feed.loadMore}>
                重试
              </button>
            </div>
          ) : null}
          {feed.hasMore ? (
            <>
              <div ref={sentinelRef} data-testid="shelf-sentinel" aria-hidden="true" style={{ height: 1 }} />
              {canObserve ? null : (
                <div data-od-id="C-39">
                  <button className="btn" type="button" onClick={feed.loadMore} data-testid="shelf-load-more">
                    加载更多
                  </button>
                </div>
              )}
            </>
          ) : (
            <p className="meta" data-testid="shelf-end">
              没有更多了 · 共 {formatCount(total)} 本
            </p>
          )}
        </div>
      ) : pages <= 1 ? null : (
        <nav className="pagination" aria-label="分页">
          <button
            type="button"
            aria-label="上一页"
            data-od-id="C-37"
            disabled={feed.page <= 1}
            onClick={() => onPage(feed.page - 1)}
          >
            ←
          </button>
          <span className="is-current num">{feed.page}</span>
          <span className="meta">/ {pages}</span>
          <button
            type="button"
            aria-label="下一页"
            data-od-id="C-38"
            disabled={feed.page >= pages}
            onClick={() => onPage(feed.page + 1)}
          >
            →
          </button>
        </nav>
      )}
    </>
  );
}
