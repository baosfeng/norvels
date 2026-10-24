import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useBooks } from "../../api/hooks";
import type { BookSummary } from "../../api/types";
import { Cover } from "../../components/Cover";
import { CardSkeleton, EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { useShelfMode } from "../../layouts/shelfMode";
import { formatCount } from "../../lib/format";
import { stripTags } from "../../lib/text";
import { useIsH5 } from "../../lib/tier";

const SORTS = [
  { value: "updated", label: "最近更新" },
  { value: "imported", label: "导入时间" },
  { value: "title", label: "书名" },
  { value: "chapters", label: "章节数" },
  { value: "words", label: "字数" },
] as const;

const PAGE_SIZE = 24;

/** 书架：PC 是工作台（排序 / 视图切换 / 分页），H5 是单列简表（无工具栏、无分页）。 */
export function ShelfPage() {
  const isH5 = useIsH5();
  const { mode } = useShelfMode();
  const [sort, setSort] = useState<(typeof SORTS)[number]["value"]>("updated");
  const [page, setPage] = useState(1);

  const params = useMemo(
    () => ({ sort, page, size: isH5 ? 30 : PAGE_SIZE }),
    [sort, page, isH5],
  );
  const { data, loading, error, reload } = useBooks(params);

  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / (isH5 ? 30 : PAGE_SIZE)));
  const items = data?.items ?? [];

  return (
    <section className="view-shelf" data-testid="view:shelf">
      <div className="page-head">
        <div>
          <p className="eyebrow">书架 · {loading ? "读取中" : `${formatCount(total)} 本`}</p>
          <h1>{isH5 ? "书架" : "最近入库"}</h1>
          {isH5 ? null : (
            <p className="sub">
              按{SORTS.find((s) => s.value === sort)?.label}排序 · 共 {formatCount(total)} 本
            </p>
          )}
        </div>
      </div>

      {isH5 ? null : (
        <div className="toolbar">
          <label>
            排序
            <select
              className="select"
              value={sort}
              onChange={(e) => {
                setSort(e.target.value as typeof sort);
                setPage(1);
              }}
            >
              {SORTS.map((s) => (
                <option key={s.value} value={s.value}>
                  {s.label}
                </option>
              ))}
            </select>
          </label>
        </div>
      )}

      {loading ? (
        <div data-testid="shelf-loading">
          <LoadingPanel
            title="正在读取书架"
            description={`正在整理本地书目，${formatCount(total || 0) || "全部"} 本大约需要 1–2 秒。`}
          />
          <CardSkeleton count={isH5 ? 3 : 4} />
        </div>
      ) : error ? (
        <ErrorPanel error={error} title="书架没能加载" onRetry={reload} testId="shelf-error" />
      ) : items.length === 0 ? (
        <EmptyPanel
          title="书架还是空的"
          description={
            isH5
              ? "先在电脑上把存放 txt 的目录导入，书目会同步到这里。"
              : "把存放 txt 的目录填进导入页，完成后书目会自动出现在这里。"
          }
          testId="shelf-empty"
          actions={
            isH5 ? null : (
              <Link className="btn btn-primary" to="/import">
                去导入
              </Link>
            )
          }
        />
      ) : (
        <>
          <div className={`book-grid ${mode === "list" && !isH5 ? "is-list" : ""}`.trim()} data-testid="shelf-live">
            {items.map((book) => (
              <BookCard key={book.id} book={book} compact={isH5} />
            ))}
          </div>
          {isH5 || pages <= 1 ? null : (
            <nav className="pagination" aria-label="分页">
              <button
                type="button"
                aria-label="上一页"
                disabled={page <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                ←
              </button>
              <span className="is-current num">{page}</span>
              <span className="meta">/ {pages}</span>
              <button
                type="button"
                aria-label="下一页"
                disabled={page >= pages}
                onClick={() => setPage((p) => Math.min(pages, p + 1))}
              >
                →
              </button>
            </nav>
          )}
        </>
      )}
    </section>
  );
}

function BookCard({ book, compact }: { book: BookSummary; compact: boolean }) {
  return (
    <article className="book-card">
      <Link className="book-link" to={`/book/${book.id}`}>
        <Cover title={book.title} showLabel={!compact} />
        <div className="book-card-body">
          <h3>{book.title}</h3>
          <p className="book-author">{book.authorName || "佚名"}</p>
          <p className="book-meta">
            <span className="num">{formatCount(book.chapterCount)} 章</span>
            <span className="num">{formatCount(book.wordCount)} 字</span>
            {book.volumeCount > 0 ? <span>{book.volumeCount} 卷</span> : null}
          </p>
          {compact ? null : <p className="book-intro">{stripTags(book.intro) || "这本书还没有简介。"}</p>}
        </div>
      </Link>
    </article>
  );
}