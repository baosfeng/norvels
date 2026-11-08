import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useSearchParams } from "react-router-dom";
import { useBooks, useCategories, useTags, useAuthors } from "../../api/hooks";
import type { BookSummary, SerialStatus } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { Cover } from "../../components/Cover";
import { formatCount, formatDate } from "../../lib/format";
import { useTimedNote } from "../import/useTimedNote";
import { AdminBatchBar } from "./AdminBatchBar";
import { AdminFilterBar, type FilterValues } from "./AdminFilterBar";
import { Pagination } from "./Pagination";
import { buildAdminQuery, parseAdminQuery } from "./adminQuery";
import { AdminBatchDialog, type BatchKind } from "./AdminBatchDialog";
import { MAX_BATCH } from "./batchLimit";
import { TagManageDialog } from "./TagManageDialog";

const PAGE_SIZE = 20;

/**
 * 书籍管理列表（作者后台首页）：搜索 + 筛选 + 表格 + 多选批量条。
 *
 * 与书架的区别是「管理视角」：显示章节数/字数/连载状态/更新时间，可多选批量改；
 * 不提供阅读入口——点一行是进单书管理页，不是进详情页。
 */
export function AdminBooksPage() {
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  // 筛选与页码的唯一真源是地址栏：刷新、后退、把链接发给别人都能还原同一个列表。
  const { filters, page } = useMemo(() => parseAdminQuery(searchParams), [searchParams]);
  const [draftQuery, setDraftQuery] = useState(() => parseAdminQuery(searchParams).filters.query);
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [batch, setBatch] = useState<BatchKind | null>(null);
  const [managingTags, setManagingTags] = useState(false);
  const notice = useTimedNote();
  const headingRef = useRef<HTMLHeadingElement>(null);
  /** 表头全选：取消选择 / 清空已选后焦点回到这里。 */
  const selectAllRef = useRef<HTMLInputElement>(null);

  const { data, loading, error, reload } = useBooks({
    q: filters.query,
    serialStatus: filters.serial === "" ? undefined : (filters.serial as Exclude<SerialStatus, ""> | "none"),
    tag: filters.tag === "" ? undefined : filters.tag,
    categoryL1: filters.categoryL1 === "" ? undefined : filters.categoryL1,
    categoryL2: filters.categoryL2 === "" ? undefined : filters.categoryL2,
    authorId: filters.authorId === "" ? undefined : Number(filters.authorId),
    page,
    size: PAGE_SIZE,
    limit: PAGE_SIZE,
    offset: (page - 1) * PAGE_SIZE,
  });
  const tags = useTags();
  const categories = useCategories();
  // 作者「更多」里的搜索词：作者总数可能上百，本地只拿得到前 200 位（接口 size 上限），
  // 所以带 q 去服务端查，保证 323 位里任意一位都找得到。
  const [authorQuery, setAuthorQuery] = useState("");
  const authors = useAuthors({ q: authorQuery.trim() || undefined, size: 200 });

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  /** 有没有任何筛选生效：页头右侧的单位跟着切换（本在库 / 本符合） */
  const filtering =
    filters.query !== "" ||
    filters.categoryL1 !== "" ||
    filters.categoryL2 !== "" ||
    filters.authorId !== "" ||
    filters.tag !== "" ||
    filters.serial !== "";
  const selected = items.filter((book) => picked.has(book.id));
  const allPicked = items.length > 0 && selected.length === items.length;

  // 删书后回到列表：焦点落在标题、滚动回顶部（「移除后五问」）
  useEffect(() => {
    const state = location.state as { deletedBook?: number } | null;
    if (!state?.deletedBook) return;
    window.scrollTo(0, 0);
    headingRef.current?.focus();
  }, [location.state]);

  /** 地址栏是状态载体：筛选或页码一变就写回 URL（默认值不写，空地址 = 全部书第 1 页）。 */
  function apply(nextFilters: FilterValues, nextPage: number) {
    setSearchParams(buildAdminQuery(nextFilters, nextPage));
    setPicked(new Set());
  }

  /** 下拉改动立即生效并回到第 1 页：停在一张空页上最容易让人以为「没有书」。 */
  function changeFilters(patch: Partial<FilterValues>) {
    apply({ ...filters, ...patch }, 1);
  }

  function submitSearch() {
    const next = draftQuery.trim();
    if (next === filters.query) {
      notice.show("筛选条件没变，已回到第 1 页");
      apply(filters, 1);
      return;
    }
    apply({ ...filters, query: next }, 1);
  }

  function resetFilters() {
    setDraftQuery("");
    setSearchParams({});
    setPicked(new Set());
    keepFocusOnSearch();
  }

  // 输入框只是草稿；地址上的 q 变了（提交 / 后退 / 外部链接）才把草稿对齐过去。
  // 渲染期派生（与 useChapterStream 同一写法）：放 effect 里会多一帧，草稿在旧值上闪一下。
  const [syncedQuery, setSyncedQuery] = useState(filters.query);
  if (syncedQuery !== filters.query) {
    setSyncedQuery(filters.query);
    setDraftQuery(filters.query);
  }

  /** 重置后焦点回搜索框。 */
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

  /** 取消选择后批量条会卸载，焦点必须先在表头全选上再接住。 */
  function clearPicked() {
    setPicked(new Set());
    window.setTimeout(() => selectAllRef.current?.focus(), 0);
  }

  /** 关闭标签弹层后把焦点还给入口按钮；弹层里的焦点陷阱在卸载时也会还一次。 */
  function closeTagManage() {
    setManagingTags(false);
    window.setTimeout(() => document.querySelector<HTMLElement>('[data-testid="admin-tag-manage"]')?.focus(), 0);
  }

  return (
    <section className="ix-ops" data-testid="view:admin-books">
      <div className="page-head">
        <div>
          <p className="eyebrow">书籍管理</p>
          <h1 ref={headingRef} tabIndex={-1} data-testid="admin-list-title">
            全部书籍
          </h1>
          <p className="sub">改元信息、传封面、改章节都在这里；阅读页只负责读。</p>
        </div>
        {/* 右侧不再空着：库存量是这个页面唯一的「关键数字」，筛选后跟着变 */}
        <div className="page-head-stat" data-testid="admin-head-stat">
          <strong className="num">{loading ? "—" : formatCount(total)}</strong>
          <span>{filtering ? "本符合" : "本在库"}</span>
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
        authorQuery={authorQuery}
        onAuthorQuery={setAuthorQuery}
      />

      <AdminBatchBar
        count={picked.size}
        onAuthor={() => setBatch("author")}
        onTag={() => setBatch("tag")}
        onSerial={() => setBatch("serial")}
        onDelete={() => setBatch("delete")}
        onClear={clearPicked}
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
                    <label className="check-hit">
                      <input
                        type="checkbox"
                        aria-label="全选当前页"
                        data-testid="admin-select-page"
                        disabled={items.length === 0}
                        checked={allPicked}
                        ref={(node) => {
                          selectAllRef.current = node;
                          if (node) node.indeterminate = selected.length > 0 && !allPicked;
                        }}
                        onChange={togglePage}
                      />
                    </label>
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

      <Pagination page={page} pages={pages} onPage={(next) => apply(filters, next)} />

      {managingTags ? (
        <TagManageDialog
          tags={tags.data?.items ?? []}
          loading={tags.loading}
          error={tags.error}
          onReload={tags.reload}
          onChanged={(message) => {
            notice.show(message);
            // 标签增删后筛选下拉与列表同步刷新
            tags.reload();
            reload();
          }}
          onClose={() => {
            closeTagManage();
            // 刚删掉的正好是筛选用的那个标签：重置为全部并说明原因
            if (filters.tag !== "" && !(tags.data?.items ?? []).some((item) => item.name === filters.tag)) {
              apply({ ...filters, tag: "" }, 1);
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

/** 空态分两种：筛选没命中给「清除筛选」，书库真空给「去导入」。 */
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
        <Link className="btn btn-primary" to="/import">
          去导入
        </Link>
      }
      testId="admin-list-empty"
    />
  );
}

/** 一行：勾选 / 封面 / 书名 / 章节 / 字数 / 连载状态 / 更新时间（书名可点进单书管理）。 */
function AdminBookRow({ book, picked, onToggle }: { book: BookSummary; picked: boolean; onToggle: () => void }) {
  return (
    <tr className={picked ? "is-picked" : undefined} data-testid={`admin-book-row-${book.id}`}>
      <td className="c-check">
        <label className="check-hit">
          <input
            type="checkbox"
            checked={picked}
            aria-label={`选择《${book.title}》`}
            data-testid={`admin-book-pick-${book.id}`}
            onChange={onToggle}
          />
        </label>
      </td>
      <td className="c-cover">
        <Cover title={book.title} bookId={book.id} updatedAt={book.updatedAt} hasCover={book.hasCover} className="cover-xs" showLabel={false} />
      </td>
      <td>
        <Link className="book-link" to={`/admin/books/${book.id}`} data-testid={`admin-book-open-${book.id}`}>
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
