import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useAuthors } from "../../api/hooks";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";

/** 作者管理（PC 专属）：表格 + 排序 + 分页。H5 下不渲染。 */
export function AuthorsPage() {
  const [sort, setSort] = useState<"books" | "words" | "name">("books");
  const [page, setPage] = useState(1);
  const { data, loading, error, reload } = useAuthors({ sort, page, size: 30 });
  const tableRef = useRef<HTMLDivElement | null>(null);

  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / 30));
  const items = data?.items ?? [];

  /** 切页：页码更新后表格回到第一行（契约 C-20 / C-21 移除后）。 */
  function goPage(next: number) {
    setPage(next);
    tableRef.current?.scrollIntoView?.({ block: "start" });
  }

  return (
    <section data-testid="view:authors" data-od-id="authors">
      <div className="page-head">
        <div>
          <p className="eyebrow">作者 · {loading ? "读取中" : `${formatCount(total)} 位`}</p>
          <h1>作者</h1>
          <p className="sub">按{sortLabel(sort)}排序</p>
        </div>
      </div>

      <div className="toolbar">
        <label>
          排序
          <select
            className="select"
            value={sort}
            data-od-id="C-19"
            onChange={(e) => {
              setSort(e.target.value as typeof sort);
              setPage(1);
            }}
          >
            <option value="books">书籍数</option>
            <option value="words">总字数</option>
            <option value="name">名字</option>
          </select>
        </label>
      </div>

      <div className="authors-page" ref={tableRef}>
        {loading ? (
          <LoadingPanel title="正在读取作者" description="正在整理作者与其作品。" />
        ) : error ? (
          <ErrorPanel error={error} title="作者列表没能加载" onRetry={reload} />
        ) : items.length === 0 ? (
          <EmptyPanel title="还没有作者" description="导入书目后，作者会按作品数自动汇总到这里。" />
        ) : (
          <div className="authors-table">
            <table className="table">
              <thead>
                <tr>
                  <th>作者</th>
                  <th className="num">作品</th>
                  <th className="num">总字数</th>
                </tr>
              </thead>
              <tbody>
                {items.map((author) => (
                  <tr key={author.id}>
                    <td>
                      {/* C-22（新增）：作者名进该作者的作品页；超长单行省略，全名仍在文本节点里 */}
                      <Link
                        className="author-link"
                        to={`/authors/${author.id}`}
                        data-od-id="C-22"
                        data-testid={`author-link-${author.id}`}
                      >
                        {author.name || "佚名"}
                      </Link>
                    </td>
                    <td className="num">{author.bookCount}</td>
                    <td className="num">{formatCount(author.wordCount)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {pages > 1 ? (
        <nav className="pagination" aria-label="分页">
          <button
            type="button"
            aria-label="上一页"
            data-od-id="C-20"
            disabled={page <= 1}
            onClick={() => goPage(page - 1)}
          >
            ←
          </button>
          <span className="is-current num">{page}</span>
          <span className="meta">/ {pages}</span>
          <button
            type="button"
            aria-label="下一页"
            data-od-id="C-21"
            disabled={page >= pages}
            onClick={() => goPage(page + 1)}
          >
            →
          </button>
        </nav>
      ) : null}
    </section>
  );
}

function sortLabel(sort: "books" | "words" | "name"): string {
  if (sort === "words") return "总字数";
  if (sort === "name") return "名字";
  return "书籍数";
}
