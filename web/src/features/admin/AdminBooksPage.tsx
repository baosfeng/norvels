import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { useBooks, useCategories, useTags, useAuthors } from "../../api/hooks";
import type { BookSummary, SerialStatus } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { Cover } from "../../components/Cover";
import { formatCount, formatDate } from "../../lib/format";
import { useTimedNote } from "../import/useTimedNote";
import { AdminBatchBar } from "./AdminBatchBar";
import { AdminFilterBar, type FilterValues } from "./AdminFilterBar";
import { AdminBatchDialog, type BatchKind } from "./AdminBatchDialog";
import { MAX_BATCH } from "./batchLimit";
import { TagManageDialog } from "./TagManageDialog";

const PAGE_SIZE = 20;

/** 空筛选值：重置与「没有条件」判断共用一份，避免两处各写一遍。 */
const EMPTY_FILTERS: FilterValues = { query: "", categoryL1: "", categoryL2: "", authorId: "", tag: "", serial: "" };

/**
 * 书籍管理列表（作者后台首页）：搜索 + 筛选 + 表格 + 多选批量条。
 *
 * 与书架的区别是「管理视角」：显示章节数/字数/连载状态/更新时间，可多选批量改；
 * 不提供阅读入口——点一行是进单书管理页，不是进详情页。
 */
export function AdminBooksPage() {
  const location = useLocation();
  const [draftQuery, setDraftQuery] = useState("");
  const [filters, setFilters] = useState<FilterValues>(EMPTY_FILTERS);
  const [page, setPage] = useState(1);
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [batch, setBatch] = useState<BatchKind | null>(null);
  const [managingTags, setManagingTags] = useState(false);
  const notice = useTimedNote();
  const headingRef = useRef<HTMLHeadingElement>(null);

  const { data, loading, error, reload } = useBooks({
    q: filters.query,
    serialStatus: filters.serial === "" ? undefined : (filters.serial as Exclude<SerialStatus, ""> | "none"),
    tag: filters.tag === "" ? undefined : filters.tag,
    categoryL1: filters.categoryL1 === "" ? undefined : filters.categoryL1,
    categoryL2: filters.categoryL2 === "" ? undefined : filters.categoryL2,
    authorId: filters.authorId === "" ? undefined : Number(filters.authorId),
    page,
    size: PAGE_SIZE,
  });
  const tags = useTags();
  const categories = useCategories();
  const authors = useAuthors({ size: 50 });

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const selected = items.filter((book) => picked.has(book.id));
  const allPicked = items.length > 0 && selected.length === items.length;

  // 删书后回到列表：焦点落在标题、滚动回顶部（契约 C-52 的移除后五问）
  useEffect(() => {
    const state = location.state as { deletedBook?: number } | null;
    if (!state?.deletedBook) return;
    window.scrollTo(0, 0);
    headingRef.current?.focus();
  }, [location.state]);

  /** 下拉改动立即生效并回到第 1 页：停在一张空页上最容易让人以为「没有书」。 */
  function changeFilters(patch: Partial<FilterValues>) {
    setFilters((prev) => ({ ...prev, ...patch }));
    setPage(1);
  }

  function submitSearch() {
    const next = draftQuery.trim();
    if (next === filters.query) {
      notice.show("筛选条件没变，已回到第 1 页");
      setPage(1);
      return;
    }
    setFilters((prev) => ({ ...prev, query: next }));
    setPage(1);
  }

  function resetFilters() {
    setDraftQuery("");
    setFilters(EMPTY_FILTERS);
    setPage(1);
    setPicked(new Set());
    keepFocusOnSearch();
  }

  /** 重置后焦点回搜索框（契约 C-33）。 */
  function keepFocusOnSearch() {
    window.setTimeout(() => document.querySelector<HTMLInputElement>('[data-testid="admin-search"]')?.focus(), 0);
  }

  function togglePick(id: number) {
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else if (next.size < MAX_BATCH) next.add(id);
      return next;
    });
  }

  function togglePage() {
    setPicked(allPicked ? new Set() : new Set(items.slice(0, MAX_BATCH).map((book) => book.id)));
  }

  return (
    <section className="ix-ops" data-testid="view:admin-books" data-od-id="admin-list">
      <div className="page-head">
        <div>
          <p className="eyebrow">书籍管理 · {loading ? "读取中" : `${formatCount(total)} 本`}</p>
          <h1 ref={headingRef} tabIndex={-1} data-testid="admin-list-title">
            全部书籍
          </h1>
          <p className="sub">改元信息、传封面、改章节都在这里；阅读页只负责读。</p>
        </div>
      </div>

      {notice.note ? (
        <p className="notice" role="status" data-testid="admin-list-notice">
          {notice.note}
        </p>
      ) : null}

      <AdminFilterBar
        draftQuery={draftQuery}
        onDraftQuery={setDraftQuery}
        values={filters}
        onChange={changeFilters}
        onSubmit={submitSearch}
        onReset={resetFilters}
        onTagManage={() => setManagingTags(true)}
        categories={categories.data?.items ?? []}
        authors={authors.data?.items ?? []}
        tags={tags.data?.items ?? []}
      />

      <AdminBatchBar
        count={picked.size}
        onAuthor={() => setBatch("author")}
        onTag={() => setBatch("tag")}
        onSerial={() => setBatch("serial")}
        onDelete={() => setBatch("delete")}
        onClear={() => setPicked(new Set())}
      />

      <div className="admin-books">
        {loading ? (
          <LoadingPanel title="正在读取书籍" description="正在取回书名、章节数与更新时间。" testId="admin-list-loading" />
        ) : error ? (
          <ErrorPanel error={error} title="书籍列表没能载入" onRetry={reload} testId="admin-list-error" />
        ) : items.length === 0 ? (
          <ListEmpty filtering={anyFilter(filters)} onReset={resetFilters} />
        ) : (
          <div className="table-wrap">
            <table className="table admin-table" data-testid="admin-table">
              <thead>
                <tr>
                  <th style={{ width: 44 }}>
                    <input
                      type="checkbox"
                      aria-label="全选当前页"
                      data-testid="admin-select-page"
                      data-od-id="C-35"
                      disabled={items.length === 0}
                      checked={allPicked}
                      ref={(node) => {
                        if (node) node.indeterminate = selected.length > 0 && !allPicked;
                      }}
                      onChange={togglePage}
                    />
                  </th>
                  <th style={{ width: 52 }}>
                    <span className="sr-only">封面</span>
                  </th>
                  <th>书名</th>
                  <th className="is-num" style={{ width: 90 }}>
                    章节
                  </th>
                  <th className="is-num" style={{ width: 100 }}>
                    字数
                  </th>
                  <th style={{ width: 110 }}>连载状态</th>
                  <th className="is-num" style={{ width: 110 }}>
                    更新时间
                  </th>
                </tr>
              </thead>
              <tbody>
                {items.map((book) => (
                  <AdminBookRow key={book.id} book={book} picked={picked.has(book.id)} onToggle={() => togglePick(book.id)} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <Pagination page={page} pages={pages} onPage={setPage} />

      {managingTags ? (
        <TagManageDialog
          tags={tags.data?.items ?? []}
          loading={tags.loading}
          error={tags.error}
          onReload={tags.reload}
          onChanged={(message) => {
            notice.show(message);
            // 标签增删后筛选下拉与列表同步刷新（契约 C-34 / C-78）
            tags.reload();
            reload();
          }}
          onClose={() => {
            setManagingTags(false);
            // 刚删掉的正好是筛选用的那个标签：重置为全部并说明原因
            if (filters.tag !== "" && !(tags.data?.items ?? []).some((item) => item.name === filters.tag)) {
              setFilters((prev) => ({ ...prev, tag: "" }));
              setPage(1);
              notice.show("刚才筛的标签已被删除");
            }
          }}
        />
      ) : null}

      {batch ? (
        <AdminBatchDialog
          kind={batch}
          books={selected}
          onClose={() => setBatch(null)}
          onDone={(message) => {
            setBatch(null);
            setPicked(new Set());
            notice.show(message);
            reload();
          }}
        />
      ) : null}
    </section>
  );
}

/** 有没有任何筛选条件（重置后与初始值逐字段相等）。 */
function anyFilter(values: FilterValues): boolean {
  return Object.values(values).some((value) => value !== "");
}

/** 空态分两种：筛选没命中给「清除筛选」，书库真空给「去导入」（契约 C-44）。 */
function ListEmpty({ filtering, onReset }: { filtering: boolean; onReset: () => void }) {
  if (filtering) {
    return (
      <EmptyPanel
        title="没有符合条件的书"
        description="换个关键词，或者把筛选条件清掉看看。"
        actions={
          <button className="btn" type="button" onClick={onReset}>
            清除筛选
          </button>
        }
        testId="admin-list-nomatch"
      />
    );
  }
  return (
    <EmptyPanel
      title="书库还没有书"
      description="先在导入管理里导入一本，之后就能在这里改信息与章节。"
      actions={
        <Link className="btn btn-primary" to="/import" data-od-id="C-44">
          去导入
        </Link>
      }
      testId="admin-list-empty"
    />
  );
}

/** 分页：只有一页时整条不出现；到头的那一侧禁用并把原因常驻在旁边（契约 C-43）。 */
function Pagination({ page, pages, onPage }: { page: number; pages: number; onPage: (next: number) => void }) {
  if (pages <= 1) return null;
  const atStart = page <= 1;
  const atEnd = page >= pages;
  return (
    <nav className="pagination" aria-label="分页" data-testid="admin-pagination">
      <button
        type="button"
        aria-label="上一页"
        disabled={atStart}
        data-od-id="C-43"
        data-testid="admin-page-prev"
        onClick={() => onPage(Math.max(1, page - 1))}
      >
        ←
      </button>
      <span className="is-current num">{page}</span>
      <span className="meta">/ {pages}</span>
      <button
        type="button"
        aria-label="下一页"
        disabled={atEnd}
        data-od-id="C-43"
        data-testid="admin-page-next"
        onClick={() => onPage(Math.min(pages, page + 1))}
      >
        →
      </button>
      {atStart || atEnd ? <span className="inline-note">已经到头了</span> : null}
    </nav>
  );
}

/** 一行：勾选 / 封面 / 书名 / 章节 / 字数 / 连载状态 / 更新时间（书名可点进单书管理）。 */
function AdminBookRow({ book, picked, onToggle }: { book: BookSummary; picked: boolean; onToggle: () => void }) {
  return (
    <tr data-testid={`admin-book-row-${book.id}`}>
      <td className="c-check">
        <input
          type="checkbox"
          checked={picked}
          aria-label={`选择《${book.title}》`}
          data-testid={`admin-book-pick-${book.id}`}
          data-od-id="C-36"
          onChange={onToggle}
        />
      </td>
      <td className="c-cover">
        <Cover title={book.title} bookId={book.id} updatedAt={book.updatedAt} hasCover={book.hasCover} className="cover-xs" showLabel={false} />
      </td>
      <td>
        <Link className="book-link" to={`/admin/books/${book.id}`} data-testid={`admin-book-open-${book.id}`} data-od-id="C-37">
          {book.title}
        </Link>
        <span className="meta admin-author">{book.authorName || "佚名"}</span>
      </td>
      <td className="is-num">{formatCount(book.chapterCount)}</td>
      <td className="is-num">{formatCount(book.wordCount)}</td>
      <td>{serialText(book.serialStatus)}</td>
      <td className="is-num meta">{formatDate(book.updatedAt)}</td>
    </tr>
  );
}

function serialText(status: SerialStatus | undefined): string {
  if (status === "ongoing") return "连载中";
  if (status === "completed") return "已完结";
  return "未知";
}
