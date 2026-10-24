import { useState } from "react";
import { useAuthors } from "../../api/hooks";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";

/** 作者管理（PC 专属）：表格 + 排序。H5 下不渲染。 */
export function AuthorsPage() {
  const [sort, setSort] = useState<"books" | "words" | "name">("books");
  const [page, setPage] = useState(1);
  const { data, loading, error, reload } = useAuthors({ sort, page, size: 30 });

  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / 30));
  const items = data?.items ?? [];

  return (
    <section data-testid="view:authors">
      <div className="page-head">
        <div>
          <p className="eyebrow">作者 · {loading ? "读取中" : `${formatCount(total)} 位`}</p>
          <h1>作者</h1>
          <p className="sub">按{sort === "books" ? "书籍数" : sort === "words" ? "总字数" : "名字"}排序</p>
        </div>
      </div>

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
            <option value="books">书籍数</option>
            <option value="words">总字数</option>
            <option value="name">名字</option>
          </select>
        </label>
      </div>

      <div className="authors-page">
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
                    <td>{author.name || "佚名"}</td>
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
          <button type="button" aria-label="上一页" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            ←
          </button>
          <span className="is-current num">{page}</span>
          <span className="meta">/ {pages}</span>
          <button type="button" aria-label="下一页" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
            →
          </button>
        </nav>
      ) : null}
    </section>
  );
}
