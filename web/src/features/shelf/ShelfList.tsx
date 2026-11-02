import type { ReactNode, RefObject } from "react";
import { Link } from "react-router-dom";
import type { ApiError } from "../../api/client";
import type { BookSummary, ReadStatus, ReadingProgress } from "../../api/types";
import { EmptyPanel, ErrorPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
import { ShelfCard } from "./ShelfCard";
import { ShelfCardSkeleton } from "./ShelfCardSkeleton";

type Feed = {
  page: number;
  hasMore: boolean;
  loadingMore: boolean;
  errorMore: unknown;
  loadMore: () => void;
};

/**
 * 书架内容区：加载 / 错误 / 空态 / 列表 + H5 无限滚动或 PC 分页。
 *
 * 加载态用与真实书卡同尺寸的骨架（H5 两行三张 = 6 张、PC 四张），不用居中转圈——
 * 骨架先占住版面，数据回来时不跳一下。H5 追加下一页同样给骨架，只铺一行。
 */
export function ShelfList({
  isH5,
  loading,
  error,
  items,
  total,
  filtering,
  mode,
  statusOverrides,
  progress,
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
  emptySlot,
}: {
  isH5: boolean;
  loading: boolean;
  error: ApiError | null;
  items: BookSummary[];
  total: number;
  filtering: boolean;
  mode: "grid" | "list";
  statusOverrides: Record<number, ReadStatus>;
  /** H5 的阅读进度索引（bookId → 进度）；PC 传空 Map，卡片不显示进度行 */
  progress: Map<number, ReadingProgress>;
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
  /** 空态替身：给了就用它代替默认的「书架还是空的」（我的书架有自己的一套出口） */
  emptySlot?: ReactNode;
}) {
  if (loading) {
    return (
      <div data-testid="shelf-loading">
        <p className="meta shelf-loading-note" role="status">
          正在读取书目，{formatCount(total || 0) ? `${formatCount(total || 0)} 本` : "全部"}大约需要 1–2 秒。
        </p>
        <ShelfCardSkeleton count={isH5 ? 6 : 4} />
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <ErrorPanel error={error} title="书架没能加载" onRetry={onRetry} testId="shelf-error" />
      </div>
    );
  }

  if (items.length === 0) {
    if (emptySlot) return <>{emptySlot}</>;
    return (
      <div>
        <EmptyPanel
          title={filtering ? "没有符合筛选条件的书" : "书架还是空的"}
          description={
            filtering
              ? "换一个筛选条件试试，或者清除筛选看全部书目。"
              : "书库还没有内容：在电脑上打开导入页，把存放 txt 的目录加进来，书目就会出现在这里。"
          }
          testId="shelf-empty"
          actions={
            filtering ? (
              <button className="btn btn-primary" type="button" onClick={onResetFilters}>
                清除筛选
              </button>
            ) : isH5 ? null : (
              <Link className="btn btn-primary" to="/import">
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
      <div className={`book-grid${mode === "list" && !isH5 ? " is-list" : ""}`.trim()} data-testid="shelf-live">
        {items.map((book) => (
          <ShelfCard
            key={book.id}
            book={book}
            compact={isH5}
            status={statusOverrides[book.id] ?? book.readStatus ?? ""}
            progress={progress.get(book.id)}
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
            <div className="shelf-more-loading" role="status" data-testid="shelf-more-loading">
              <ShelfCardSkeleton count={3} label="正在加载更多" />
            </div>
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
                <div>
                  <button className="btn" type="button" onClick={feed.loadMore} data-testid="shelf-load-more">
                    加载更多
                  </button>
                </div>
              )}
            </>
          ) : (
            <p className="meta shelf-end" data-testid="shelf-end">
              已经到底啦 · 共 {formatCount(total)} 本
            </p>
          )}
        </div>
      ) : pages <= 1 ? null : (
        <nav className="pagination" aria-label="分页">
          <button
            type="button"
            aria-label="上一页"
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
