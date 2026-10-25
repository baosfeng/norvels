import { useState } from "react";
import { Link } from "react-router-dom";
import { useBooks, useTags } from "../../api/hooks";
import type { BookSummary, SerialStatus } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { Cover } from "../../components/Cover";
import { formatCount, formatDate } from "../../lib/format";
import { AdminBatchDialog, type BatchKind } from "./AdminBatchDialog";
import { TagManageDialog } from "./TagManageDialog";

const PAGE_SIZE = 20;

/**
 * 书籍管理列表（作者后台首页）：搜索 + 筛选 + 表格 + 多选批量条。
 *
 * 与书架的区别是「管理视角」：显示章节数/字数/连载状态/更新时间，可多选批量改；
 * 不提供阅读入口——点一行是进单书管理页，不是进详情页。
 */
export function AdminBooksPage() {
  const [draftQuery, setDraftQuery] = useState("");
  const [query, setQuery] = useState("");
  const [serial, setSerial] = useState<"" | Exclude<SerialStatus, ""> | "none">("");
  const [tag, setTag] = useState("");
  const [page, setPage] = useState(1);
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [batch, setBatch] = useState<BatchKind | null>(null);
  const [managingTags, setManagingTags] = useState(false);
  const [notice, setNotice] = useState("");

  const { data, loading, error, reload } = useBooks({
    q: query,
    serialStatus: serial === "" ? undefined : serial,
    tag: tag === "" ? undefined : tag,
    page,
    size: PAGE_SIZE,
  });
  const tags = useTags();

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const selected = items.filter((book) => picked.has(book.id));

  function togglePick(id: number) {
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function resetFilters() {
    setDraftQuery("");
    setQuery("");
    setSerial("");
    setTag("");
    setPage(1);
  }

  const filtering = query !== "" || serial !== "" || tag !== "";

  return (
    <section data-testid="view:admin-books" data-od-id="admin-list">
      <div className="page-head">
        <div>
          <p className="eyebrow">书籍管理 · {loading ? "读取中" : `${formatCount(total)} 本`}</p>
          <h1>全部书籍</h1>
          <p className="sub">改元信息、传封面、改章节都在这里；阅读页只负责读。</p>
        </div>
      </div>

      {notice ? (
        <p className="notice" role="status" data-testid="admin-list-notice">
          {notice}
        </p>
      ) : null}

      <form
        className="toolbar admin-filter"
        data-testid="admin-filters"
        onSubmit={(event) => {
          event.preventDefault();
          setQuery(draftQuery.trim());
          setPage(1);
        }}
      >
        <label className="search-wrap">
          <span className="sr-only">搜索书名或作者</span>
          <input
            className="input"
            type="search"
            value={draftQuery}
            placeholder="搜书名或作者"
            data-testid="admin-search"
            onChange={(event) => setDraftQuery(event.target.value)}
          />
        </label>
        <label>
          连载状态
          <select
            className="select"
            value={serial}
            data-testid="admin-filter-serial"
            onChange={(event) => {
              setSerial(event.target.value as "" | Exclude<SerialStatus, ""> | "none");
              setPage(1);
            }}
          >
            <option value="">全部</option>
            <option value="ongoing">连载中</option>
            <option value="completed">已完结</option>
            <option value="none">未知</option>
          </select>
        </label>
        <label>
          标签
          <select
            className="select"
            value={tag}
            data-testid="admin-filter-tag"
            onChange={(event) => {
              setTag(event.target.value);
              setPage(1);
            }}
          >
            <option value="">全部</option>
            {(tags.data?.items ?? []).map((item) => (
              <option key={item.id} value={item.name}>
                {item.name}（{item.bookCount}）
              </option>
            ))}
          </select>
        </label>
        <button className="btn btn-sm" type="button" data-testid="admin-tag-manage" onClick={() => setManagingTags(true)}>
          标签管理…
        </button>
        <button className="btn btn-sm" type="submit" data-testid="admin-search-submit">
          搜索
        </button>
        {filtering ? (
          <button className="btn btn-sm btn-ghost" type="button" data-testid="admin-filter-reset" onClick={resetFilters}>
            重置筛选
          </button>
        ) : null}
      </form>

      {picked.size > 0 ? (
        <div className="batch-bar" data-testid="admin-batch-bar">
          <span className="meta">
            已选 <strong className="num">{picked.size}</strong> 本
          </span>
          <div className="actions">
            <button className="btn btn-sm" type="button" data-testid="admin-batch-author" onClick={() => setBatch("author")}>
              设作者
            </button>
            <button className="btn btn-sm" type="button" data-testid="admin-batch-tag" onClick={() => setBatch("tag")}>
              设标签
            </button>
            <button className="btn btn-sm" type="button" data-testid="admin-batch-serial" onClick={() => setBatch("serial")}>
              设连载状态
            </button>
            <button className="btn btn-sm btn-danger" type="button" data-testid="admin-batch-delete" onClick={() => setBatch("delete")}>
              删除
            </button>
            <button className="btn btn-sm btn-ghost" type="button" data-testid="admin-batch-clear" onClick={() => setPicked(new Set())}>
              取消选择
            </button>
          </div>
        </div>
      ) : null}

      <div className="admin-books">
        {loading ? (
          <LoadingPanel title="正在读取书籍" description="正在取回书名、章节数与更新时间。" testId="admin-list-loading" />
        ) : error ? (
          <ErrorPanel error={error} title="书籍列表没能载入" onRetry={reload} testId="admin-list-error" />
        ) : items.length === 0 ? (
          filtering ? (
            <EmptyPanel
              title="没有符合条件的书"
              description="换个关键词，或者把筛选条件清掉看看。"
              actions={
                <button className="btn" type="button" onClick={resetFilters}>
                  清除筛选
                </button>
              }
              testId="admin-list-nomatch"
            />
          ) : (
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
          )
        ) : (
          <div className="table-wrap">
            <table className="table admin-table" data-testid="admin-table">
              <thead>
                <tr>
                  <th style={{ width: 44 }}>
                    <span className="sr-only">选择</span>
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
                  <AdminBookRow
                    key={book.id}
                    book={book}
                    picked={picked.has(book.id)}
                    onToggle={() => togglePick(book.id)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {pages > 1 ? (
        <nav className="pagination" aria-label="分页">
          <button type="button" aria-label="上一页" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
            ←
          </button>
          <span className="is-current num">{page}</span>
          <span className="meta">/ {pages}</span>
          <button type="button" aria-label="下一页" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
            →
          </button>
        </nav>
      ) : null}

      {managingTags ? (
        <TagManageDialog
          tags={tags.data?.items ?? []}
          loading={tags.loading}
          error={tags.error}
          onReload={tags.reload}
          onChanged={setNotice}
          onClose={() => {
            setManagingTags(false);
            // 刚删掉的正好是筛选用的那个标签：选项没了还继续筛会得到「没有符合条件的书」
            if (tag !== "" && !(tags.data?.items ?? []).some((item) => item.name === tag)) {
              setTag("");
              setPage(1);
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
            setNotice(message);
            reload();
          }}
        />
      ) : null}
    </section>
  );
}

/** 一行：勾选 / 封面 / 书名 / 章节 / 字数 / 连载状态 / 更新时间（整行标题可点进单书管理）。 */
function AdminBookRow({ book, picked, onToggle }: { book: BookSummary; picked: boolean; onToggle: () => void }) {
  return (
    <tr data-testid={`admin-book-row-${book.id}`}>
      <td className="c-check">
        <input
          type="checkbox"
          checked={picked}
          aria-label={`选择《${book.title}》`}
          data-testid={`admin-book-pick-${book.id}`}
          onChange={onToggle}
        />
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
