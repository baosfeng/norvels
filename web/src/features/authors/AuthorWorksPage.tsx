import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../../api/client";
import type { AuthorDetail } from "../../api/types";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
import { useResource } from "../../lib/useResource";

/**
 * 某位作者的作品列表（契约 C-22 的落点）。
 *
 * 作者页的表格只列「作者 / 作品数 / 总字数」，点进去看这个作者到底写了哪几本；
 * 数据来自作者详情（含作品列表），不额外拼查询条件，避免两处口径漂移。
 */
export function AuthorWorksPage() {
  const { authorId } = useParams();
  const id = Number(authorId);
  const enabled = Number.isFinite(id);
  const navigate = useNavigate();
  const { data, loading, error, reload } = useResource<AuthorDetail>(
    (signal) => api.author(id, { signal }),
    [id],
    { enabled },
  );

  const books = data?.books ?? [];
  const name = data?.name || "佚名";

  return (
    <section data-testid="view:author-works">
      <nav className="crumbs" aria-label="面包屑">
        <Link to="/authors">作者</Link>
        <span aria-hidden="true">/</span>
        <span className="meta">{loading ? "读取中" : name}</span>
      </nav>

      <div className="page-head">
        <div>
          <p className="eyebrow">作者作品 · {loading ? "读取中" : `${formatCount(books.length)} 本`}</p>
          <h1>{loading ? "作者" : name}</h1>
          <p className="sub">
            {loading
              ? "正在整理这位作者的作品。"
              : `共 ${formatCount(data?.bookCount ?? books.length)} 本 · ${formatCount(data?.wordCount ?? 0)} 字`}
          </p>
        </div>
      </div>

      <div className="authors-page">
        {loading ? (
          <LoadingPanel title="正在读取作品" description="正在整理这位作者的书。" />
        ) : error ? (
          <ErrorPanel error={error} title="作品列表没能加载" onRetry={reload} />
        ) : books.length === 0 ? (
          <EmptyPanel
            title="这位作者还没有作品"
            description="书在导入时会按作者名归档；补上作者名后就会出现在这里。"
            actions={
              <Link className="btn" to="/authors">
                回到作者列表
              </Link>
            }
          />
        ) : (
          <div className="book-grid">
            {books.map((book) => (
              <Link
                className="book-card"
                key={book.id}
                to={`/book/${book.id}`}
                data-testid={`author-book-${book.id}`}
              >
                <Cover title={book.title} bookId={book.id} updatedAt={book.updatedAt} hasCover={book.hasCover} showLabel={false} />
                <div className="book-card-body">
                  <h3>{book.title}</h3>
                  <p className="book-meta">
                    <span className="num">{formatCount(book.chapterCount)} 章</span>
                    <span className="num">{formatCount(book.wordCount)} 字</span>
                  </p>
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>

      <div className="confirm-actions">
        <button className="btn btn-ghost" type="button" onClick={() => navigate("/authors")}>
          回到作者列表
        </button>
      </div>
    </section>
  );
}
