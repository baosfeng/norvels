import { useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useSearch } from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { hasSnippet, renderSnippet } from "../../lib/snippet";
import { formatCount } from "../../lib/format";
import { useIsH5 } from "../../lib/tier";
import { stripTags } from "../../lib/text";

type Tab = "all" | "book" | "chapter";
const SEARCH_PAGE_SIZE = 8;
const TABS: Tab[] = ["all", "book", "chapter"];

/**
 * 搜索结果：PC 两栏（书名和作者 + 章节标题命中），H5 单列（两栏收成一列）。
 *
 * 检索范围只有三样：书名、作者、章节标题。章节命中不再带正文片段（后端仍返回 snippet 字段，
 * 但恒为空串），所以片段整行按「没有就完全不渲染」处理，不留空的高亮块或占位行。
 * 分页为新增能力：服务端已支持 page / size，两栏一起翻页（C-54 / C-55）。
 */
export function SearchPage() {
  const [params, setParams] = useSearchParams();
  const isH5 = useIsH5();
  const q = params.get("q") ?? "";
  const page = Math.max(1, Number(params.get("page") ?? 1) || 1);
  const [draft, setDraft] = useState(q);
  const [tab, setTab] = useState<Tab>("all");
  const searchBoxRef = useRef<HTMLInputElement | null>(null);
  const { data, loading, error, reload } = useSearch(q, "all", page, SEARCH_PAGE_SIZE);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const keyword = draft.trim();
    // 提交即回第 1 页；空关键词回到「输入关键词开始查找」
    setParams(keyword ? { q: keyword } : {});
  }

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      setDraft("");
      return;
    }
    if (event.key === "Enter" && event.nativeEvent.isComposing) event.preventDefault();
  }

  function changePage(next: number) {
    const merged = new URLSearchParams(params);
    if (next > 1) merged.set("page", String(next));
    else merged.delete("page");
    setParams(merged);
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 测试环境没有 scrollTo */
    }
  }

  function onTabsKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = TABS.indexOf(tab);
    let next: number;
    if (event.key === "ArrowRight") next = (index + 1) % TABS.length;
    else if (event.key === "ArrowLeft") next = (index + TABS.length - 1) % TABS.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = TABS.length - 1;
    else return;
    event.preventDefault();
    setTab(TABS[next]);
    event.currentTarget.querySelector<HTMLElement>(`[data-tab="${TABS[next]}"]`)?.focus();
  }

  function retry() {
    reload();
    // 离开错误态后焦点交给搜索框（C-53）
    searchBoxRef.current?.focus();
  }

  const bookCount = data?.books.total ?? 0;
  const chapterCount = data?.chapters.total ?? 0;
  const tooShort = q.trim().length > 0 && q.trim().length < 2;
  const bookItems = data?.books.items ?? [];
  const chapterItems = data?.chapters.items ?? [];
  const pages = Math.max(1, Math.ceil(bookCount / SEARCH_PAGE_SIZE), Math.ceil(chapterCount / SEARCH_PAGE_SIZE));

  return (
    <section data-testid="view:search" data-od-id="search">
      <div className="search-head">
        <p className="eyebrow">搜索书库</p>
        <h1>{q ? `“${q}”` : "搜索"}</h1>
        <form className="search-form" onSubmit={submit} role="search" data-od-id="C-49">
          <input
            ref={searchBoxRef}
            className="input"
            type="search"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onSearchKeyDown}
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
          <div className="search-tabs" role="tablist" aria-label="结果分类" data-od-id="C-50" onKeyDown={onTabsKeyDown}>
            <button
              type="button"
              role="tab"
              data-tab="all"
              aria-selected={tab === "all"}
              className={tab === "all" ? "is-active" : ""}
              onClick={() => setTab("all")}
            >
              全部
            </button>
            <button
              type="button"
              role="tab"
              data-tab="book"
              aria-selected={tab === "book"}
              className={tab === "book" ? "is-active" : ""}
              onClick={() => setTab("book")}
            >
              书名和作者 {bookCount}
            </button>
            <button
              type="button"
              role="tab"
              data-tab="chapter"
              aria-selected={tab === "chapter"}
              className={tab === "chapter" ? "is-active" : ""}
              onClick={() => setTab("chapter")}
            >
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
        <div data-od-id="C-53">
          <ErrorPanel error={error} title="搜索没能完成" onRetry={retry} />
        </div>
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
                  <article className="search-book" key={book.id} data-od-id="C-51">
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
                  <article className="hit" key={`${hit.bookId}-${hit.chapterId}`} data-testid="search-hit" data-od-id="C-52">
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

      {q && !loading && !error && pages > 1 ? (
        <nav className="pagination" aria-label="分页" data-testid="search-pagination">
          <button type="button" aria-label="上一页" data-od-id="C-54" disabled={page <= 1} onClick={() => changePage(page - 1)}>
            ←
          </button>
          <span className="is-current num">{page}</span>
          <span className="meta">/ {pages}</span>
          <button
            type="button"
            aria-label="下一页"
            data-od-id="C-55"
            disabled={page >= pages}
            onClick={() => changePage(page + 1)}
          >
            →
          </button>
        </nav>
      ) : null}

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
