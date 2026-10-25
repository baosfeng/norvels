import { useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useSearch } from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { renderSnippet } from "../../lib/snippet";
import { formatCount } from "../../lib/format";
import { useIsH5 } from "../../lib/tier";
import { stripTags } from "../../lib/text";

type Tab = "all" | "book" | "chapter";

/** 搜索结果：PC 两栏（书名 + 章节命中），H5 单列（两栏收成一列）。 */
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

  return (
    <section data-testid="view:search" data-od-id="search">
      <div className="search-head">
        <p className="eyebrow">全文检索</p>
        <h1>{q ? `“${q}”` : "搜索"}</h1>
        <form className="search-form" onSubmit={submit} role="search">
          <input
            className="input"
            type="search"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            aria-label="搜索关键词"
            placeholder="输入书名、作者或正文里的词"
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
            <span className="pill">书名 · 作者 · 正文</span>
          </div>
        ) : null}

        {data && !isH5 ? (
          <div className="search-tabs">
            <button type="button" className={tab === "all" ? "is-active" : ""} onClick={() => setTab("all")}>
              全部
            </button>
            <button type="button" className={tab === "book" ? "is-active" : ""} onClick={() => setTab("book")}>
              书名 {bookCount}
            </button>
            <button type="button" className={tab === "chapter" ? "is-active" : ""} onClick={() => setTab("chapter")}>
              章节 {chapterCount}
            </button>
          </div>
        ) : null}
      </div>

      {!q ? (
        <EmptyPanel title="输入关键词开始查找" description="支持按书名、作者和正文查找，正文命中会给出上下文片段。" />
      ) : tooShort ? (
        <EmptyPanel title="关键词太短" description="至少输入 2 个字，才能找到有意义的匹配。" />
      ) : loading ? (
        <LoadingPanel title="正在查找" description="正在书名、作者和正文里一起找。" />
      ) : error ? (
        <ErrorPanel error={error} title="搜索没能完成" onRetry={reload} />
      ) : bookCount === 0 && chapterCount === 0 ? (
        <EmptyPanel title="没有找到匹配的内容" description="试试更短的关键词，或者换一个说法。" testId="search-empty" />
      ) : (
        <div className="search-layout" style={isH5 ? { gridTemplateColumns: "minmax(0,1fr)" } : undefined}>
          {tab !== "chapter" && (data?.books.items.length ?? 0) > 0 ? (
            <aside aria-label="书名结果" data-od-id="search-books-col">
              <h2>
                书名结果 <span className="meta">{bookCount}</span>
              </h2>
              <div className="search-books">
                {data?.books.items.map((book) => (
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

          {tab !== "book" && (data?.chapters.items.length ?? 0) > 0 ? (
            <div>
              <h2>
                章节结果 <span className="meta">{chapterCount}</span>
              </h2>
              <div className="hits">
                {data?.chapters.items.map((hit) => (
                  <article className="hit" key={`${hit.bookId}-${hit.chapterId}`}>
                    <p className="hit-meta">
                      <Link to={`/book/${hit.bookId}`}>{hit.bookTitle}</Link>
                      <span aria-hidden="true">·</span>
                      <Link to={`/read/${hit.bookId}/${hit.chapterId}`}>{hit.chapterTitle}</Link>
                    </p>
                    <p className="hit-snippet">{renderSnippet(hit.snippet)}</p>
                  </article>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      )}

      {data?.notice ? <p className="meta">{data.notice}</p> : null}
    </section>
  );
}