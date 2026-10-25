import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useAuthors, useBooks, useTags } from "../../api/hooks";
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

/**
 * 书架：PC 是工作台（排序 / 作者筛选 / 标签筛选 / 视图切换 / 分页），
 * H5 是单列简表——工具栏整体不渲染（筛选属于管理能力，见 00-brief §0.6）。
 */
export function ShelfPage() {
  const isH5 = useIsH5();
  const { mode } = useShelfMode();
  const [sort, setSort] = useState<(typeof SORTS)[number]["value"]>("updated");
  const [authorId, setAuthorId] = useState<number | null>(null);
  const [tag, setTag] = useState<string | null>(null);
  const [page, setPage] = useState(1);

  // 筛选项的候选：作者与标签都是 PC 专属，H5 不发这两个请求
  const authorOptions = useAuthors({ sort: "books", size: 50 }, { enabled: !isH5 });
  const tagOptions = useTags({ enabled: !isH5 });

  const params = useMemo(
    () => ({
      sort,
      page,
      size: isH5 ? 30 : PAGE_SIZE,
      authorId: authorId ?? undefined,
      tag: tag ?? undefined,
    }),
    [sort, page, isH5, authorId, tag],
  );
  const { data, loading, error, reload } = useBooks(params);

  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / (isH5 ? 30 : PAGE_SIZE)));
  const items = data?.items ?? [];
  const filtering = authorId !== null || tag !== null;

  function resetFilters() {
    setAuthorId(null);
    setTag(null);
    setPage(1);
  }

  return (
    <section className="view-shelf" data-testid="view:shelf" data-od-id="shelf">
      <div className="page-head">
        <div>
          <p className="eyebrow">书架 · {loading ? "读取中" : `${formatCount(total)} 本`}</p>
          <h1>{isH5 ? "书架" : "最近入库"}</h1>
          {isH5 ? null : (
            <p className="sub">
              按{SORTS.find((s) => s.value === sort)?.label}排序 · 共 {formatCount(total)} 本
              {filtering ? "（已筛选）" : ""}
            </p>
          )}
        </div>
      </div>

      {isH5 ? null : (
        <div className="toolbar" data-testid="shelf-toolbar">
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

          <label>
            作者
            <select
              className="select"
              value={authorId ?? ""}
              onChange={(e) => {
                setAuthorId(e.target.value ? Number(e.target.value) : null);
                setPage(1);
              }}
              data-testid="filter-author"
            >
              <option value="">全部作者</option>
              {(authorOptions.data?.items ?? []).map((author) => (
                <option key={author.id} value={author.id}>
                  {author.name || "佚名"}（{author.bookCount}）
                </option>
              ))}
            </select>
          </label>

          <div className="chips" role="group" aria-label="标签筛选" data-testid="filter-tags">
            <button type="button" className={`chip${tag === null ? " is-active" : ""}`} onClick={() => { setTag(null); setPage(1); }}>
              全部
            </button>
            {(tagOptions.data?.items ?? []).map((item) => (
              <button
                key={item.id}
                type="button"
                className={`chip${tag === item.name ? " is-active" : ""}`}
                onClick={() => {
                  setTag(item.name);
                  setPage(1);
                }}
              >
                {item.name}
              </button>
            ))}
          </div>

          {filtering ? (
            <button className="btn btn-ghost" type="button" onClick={resetFilters}>
              清除筛选
            </button>
          ) : null}
        </div>
      )}

      {isH5 ? null : <FilterHints authors={authorOptions} tags={tagOptions} />}

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
              <button className="btn btn-primary" type="button" onClick={resetFilters}>
                清除筛选
              </button>
            ) : isH5 ? null : (
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

/** 筛选项自身的加载/失败/空态：不占主内容区，但也不静默失败。 */
function FilterHints({
  authors,
  tags,
}: {
  authors: { loading: boolean; error: unknown; data: { items: unknown[] } | null };
  tags: { loading: boolean; error: unknown; data: { items: unknown[] } | null };
}) {
  const notes: string[] = [];
  if (tags.loading) notes.push("正在读取标签…");
  else if (tags.error) notes.push("标签暂时读不到，可以先用作者筛选。");
  else if ((tags.data?.items.length ?? 0) === 0) notes.push("暂无标签（导入的书还没有标签信息）。");

  if (authors.error) notes.push("作者列表暂时读不到，作者筛选可能不全。");

  if (notes.length === 0) return null;
  return (
    <p className="meta filter-hints" data-testid="filter-hints">
      {notes.join(" ")}
    </p>
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