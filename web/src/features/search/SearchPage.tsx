import { useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useSearch } from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { hasSnippet, renderSnippet } from "../../lib/snippet";
import { formatCount } from "../../lib/format";
import { useIsH5 } from "../../lib/tier";
import { stripTags } from "../../lib/text";

type Tab = "all" | "book" | "chapter";

/**
 * 搜索结果：PC 两栏（书名和作者 + 章节标题命中），H5 单列（两栏收成一列）。
 *
 * 检索范围只有三样：书名、作者、章节标题。章节命中不再带正文片段（后端仍返回 snippet 字段，
 * 但恒为空串），所以片段整行按「没有就完全不渲染」处理，不留空的高亮块或占位行。
 */
export function SearchPage() {
  const [params, setParams] = useSearchParams();
  const isH5 = useIsH5();
  const q = params.get("q") ?? "";
  const [draft, setDraft] = useState(q);
  const [tab, setTab] = useState<Tab>("all");
  const { data, loading, error, reload } = useSearch(q);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const keyword = draft.trim();
    setParams(keyword ? { q: keyword } : {});
  }

  const bookCount = data?.books.total ?? 0;
  const chapterCount = data?.chapters.total ?? 0;
  const tooShort = q.trim().length > 0 && q.trim().length < 2;
  const bookItems = data?.books.items ?? [];
  const chapterItems = data?.chapters.items ?? [];

  return (
    <section data-testid="view:search" data-od-id="search">
      <div className="search-head">
        <p className="eyebrow">搜索书库</p>
        <h1>{q ? `“${q}”` : "搜索"}</h1>
        <form className="search-form" onSubmit={submit} role="search">
          <input
            className="input"
            type="search"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            aria-label="搜索关键词"
            placeholder="输入书名、作者或章节标题"
          />
          <button className="btn btn-primary" type="submit">
            搜索
          </button>
        </form>

        {q && !loading && !error ? (
          <div className="search-summary">
            <span className="meta">
              找到 {formatCount(bookCount)} 本书 · {formatCount(chapterCount)} 个章节
            </span>
            <span className="pill">书名 · 作者 · 章节标题</span>
          </div>
        ) : null}

        {data && !isH5 ? (
          <div className="search-tabs">
            <button type="button" className={tab === "all" ? "is-active" : ""} onClick={() => setTab("all")}>
              全部
            </button>
            <button type="button" className={tab === "book" ? "is-active" : ""} onClick={() => setTab("book")}>
              书名和作者 {bookCount}
            </button>
            <button type="button" className={tab === "chapter" ? "is-active" : ""} onClick={() => setTab("chapter")}>
              章节标题 {chapterCount}
            </button>
          </div>
        ) : null}
      </div>

      {!q ? (
        <EmptyPanel title="输入关键词开始查找" description="可以按书名、作者或章节标题查找。" />
      ) : tooShort ? (
        <EmptyPanel title="关键词太短" description="至少输入 2 个字，才能找到有意义的匹配。" />
      ) : loading ? (
        <LoadingPanel title="正在查找" description="正在书名、作者和章节标题里一起找。" />
      ) : error ? (
        <ErrorPanel error={error} title="搜索没能完成" onRetry={reload} />
      ) : bookCount === 0 && chapterCount === 0 ? (
        <EmptyPanel
          title="没有匹配的书名、作者或章节标题"
          description="试试更短的关键词，或者换一个说法。"
          testId="search-empty"
        />
      ) : (
        <div className="search-layout" style={isH5 ? { gridTemplateColumns: "minmax(0,1fr)" } : undefined}>
          {tab !== "chapter" && bookItems.length > 0 ? (
            <aside aria-label="书名和作者结果" data-od-id="search-books-col">
              <h2>
                书名和作者 <span className="meta">{bookCount}</span>
              </h2>
              <div className="search-books">
                {bookItems.map((book) => (
                  <article className="search-book" key={book.id}>
                    <Link className="book-link" to={`/book/${book.id}`}>
                      <Cover title={book.title} showLabel={!isH5} />
                      <div className="book-card-body">
                        <h3>{book.title}</h3>
                        <p className="book-author">{book.authorName || "佚名"}</p>
                        <p className="book-meta">
                          <span className="num">{formatCount(book.chapterCount)} 章</span>
                          <span className="num">{formatCount(book.wordCount)} 字</span>
                        </p>
                        {isH5 ? null : <p className="book-intro">{stripTags(book.intro) || "这本书还没有简介。"}</p>}
                      </div>
                    </Link>
                  </article>
                ))}
              </div>
            </aside>
          ) : null}

          {tab !== "book" && chapterItems.length > 0 ? (
            <div data-od-id="search-chapters-col">
              <h2>
                章节标题 <span className="meta">{chapterCount}</span>
              </h2>
              <div className="hits">
                {chapterItems.map((hit) => (
                  <article className="hit" key={`${hit.bookId}-${hit.chapterId}`} data-testid="search-hit">
                    <p className="hit-meta">
                      <Link to={`/book/${hit.bookId}`}>{hit.bookTitle}</Link>
                      <span aria-hidden="true">·</span>
                      <Link to={`/read/${hit.bookId}/${hit.chapterId}`}>{hit.chapterTitle}</Link>
                    </p>
                    {/* 章节命中不再有正文片段：没有片段就整行不渲染，不留空段落 */}
                    {hasSnippet(hit.snippet) ? <p className="hit-snippet">{renderSnippet(hit.snippet)}</p> : null}
                  </article>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      )}

      {/* 切到某一档、而这一档恰好没有命中：说清这一档查的是什么，而不是留一块空白 */}
      {q && !loading && !error && tab === "book" && bookCount === 0 && chapterCount > 0 ? (
        <p className="meta" data-testid="search-books-empty">
          这个关键词没有匹配的书名或作者，可以切到「章节标题」看看。
        </p>
      ) : null}
      {q && !loading && !error && tab === "chapter" && chapterCount === 0 && bookCount > 0 ? (
        <p className="meta" data-testid="search-chapters-empty">
          这个关键词没有匹配的章节标题，可以切到「书名和作者」看看。
        </p>
      ) : null}

      {data?.notice ? <p className="meta">{data.notice}</p> : null}
    </section>
  );
}