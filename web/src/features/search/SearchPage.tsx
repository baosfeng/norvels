import { useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { useLocation, useSearchParams } from "react-router-dom";
import { useAuthors, useCategories, useSearch, useTags } from "../../api/hooks";
import { ErrorPanel } from "../../components/StatePanel";
import { formatCount } from "../../lib/format";
import {
  SEARCH_HISTORY_MIN_LEN,
  clearSearchHistory,
  readSearchHistory,
  rememberSearch,
} from "../../lib/searchHistory";
import { useIsH5 } from "../../lib/tier";
import { showToast } from "../../lib/toastStore";
import { SceneRecommendBlock } from "../recommend/SceneRecommendBlock";
import { SearchEmpty } from "./SearchEmpty";
import { SearchFilters } from "./SearchFilterBar";
import { SearchResultItem } from "./SearchResultItem";
import { SearchResultSkeleton } from "./SearchResultSkeleton";
import {
  EMPTY_FILTERS,
  activeFilterCount,
  filtersToSearchParams,
  readFilters,
  writeFilters,
  type SearchFilterValues,
} from "./searchFilters";
import { useSearchScrollRestore } from "./useSearchScrollRestore";

const SEARCH_PAGE_SIZE = 8;

/**
 * 搜索结果：**只按书名搜**，两档同一口径、同一份结果。
 *
 * 检索范围收敛过一次：曾经按「书名 / 作者 / 章节标题」找，页面上还有 all/book/chapter 三个 tab，
 * 但请求恒传 type=all、切 tab 只改渲染——结果是查目录查出一堆章节命中，用户要的书反而被挤下去。
 * 现在章节命中整块删除（后端同批删除章节检索），页面不再传 type=chapter，也不再有多余的分栏。
 *
 * 分页为服务端能力：page / size 走地址，返回、刷新都还原同一屏。
 * 滚动位置不走地址，由 useSearchScrollRestore 记在本次会话里。
 */
export function SearchPage() {
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const isH5 = useIsH5();
  const q = params.get("q") ?? "";
  const page = Math.max(1, Number(params.get("page") ?? 1) || 1);
  const [draft, setDraft] = useState(q);
  const [history, setHistory] = useState<string[]>(readSearchHistory);
  const searchBoxRef = useRef<HTMLInputElement | null>(null);
  // 筛选只在有关键词时出现：没有关键词就不会发请求，摆一排点了没反应的 chips 是噪音
  const trimmed = q.trim();
  const showFilters = trimmed.length > 0;
  const filters = useMemo(() => readFilters(params), [params]);
  const activeCount = activeFilterCount(filters);
  // type 恒为 "book"：只搜书名。不传 "all" 是为了让「不搜目录」在请求上也成立，而不只是嘴上说。
  // 筛选参数与 /api/books 同名同口径：一变就重新取数（key 进 useResource 的依赖）。
  const { data, loading, error, reload } = useSearch(
    q,
    filtersToSearchParams(filters, { type: "book", page, size: SEARCH_PAGE_SIZE }),
  );
  // chips 的候选值：分类树 / 常用作者（按藏书数）/ 标签，全部按需取（无关键词时不发请求）
  const categories = useCategories({ enabled: showFilters });
  const tags = useTags({ enabled: showFilters });
  const authors = useAuthors({ sort: "books", size: 12 }, { enabled: showFilters });

  const tooShort = trimmed.length > 0 && trimmed.length < 2;
  const bookCount = data?.books.total ?? 0;
  const bookItems = data?.books.items ?? [];
  const pages = Math.max(1, Math.ceil(bookCount / SEARCH_PAGE_SIZE));
  const done = Boolean(trimmed) && !tooShort && !loading && !error;

  useSearchScrollRestore(location.pathname + location.search, done && bookCount > 0);

  /** 所有「换个关键词再搜」的入口都走这里：写历史、换地址、第 1 页（筛选保留）。 */
  function runSearch(keyword: string) {
    const value = keyword.trim();
    setDraft(value);
    if (value.length >= SEARCH_HISTORY_MIN_LEN) setHistory(rememberSearch(value));
    const next = new URLSearchParams(params);
    next.delete("page");
    if (value) next.set("q", value);
    else next.delete("q");
    setParams(next);
  }

  /**
   * 点一下 chip 就重查：筛选写进地址、页码回 1，不等「搜索」按钮。
   * 分类是前置维度：把分类点回「全部」= 收起其余组并清空它们的值，
   * 否则会留下「没有分类却带着评分」这种界面上看不见的筛选。
   */
  function changeFilters(patch: Partial<SearchFilterValues>) {
    const next = { ...filters, ...patch };
    setParams(writeFilters(params, next.categoryL1 ? next : EMPTY_FILTERS));
  }

  /** 清除筛选只清筛选：关键词与页码之外的地址栏保持干净。 */
  function clearFilters() {
    setParams(writeFilters(params, EMPTY_FILTERS));
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 测试环境没有 scrollTo */
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    runSearch(draft);
  }

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Escape") {
      // 键盘：清空关键词 + 结果区回到默认空态 + 地址栏不再留着旧 q，焦点留在输入框
      event.preventDefault();
      setDraft("");
      setParams({}, { replace: true });
      searchBoxRef.current?.focus();
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

  function retry() {
    reload();
    // 离开错误态后焦点交给搜索框
    searchBoxRef.current?.focus();
  }

  function clearHistory() {
    clearSearchHistory();
    setHistory([]);
    showToast("搜索历史已清空。");
  }

  function emptyKind(): "idle" | "short" | "none" {
    if (!trimmed) return "idle";
    if (tooShort) return "short";
    return "none";
  }

  return (
    <section data-testid="view:search">
      <div className="search-head">
        <p className="eyebrow">搜索书库</p>
        <h1>{q ? `“${q}”` : "搜索"}</h1>
        <form className="search-form" onSubmit={submit} role="search">
          <input
            ref={searchBoxRef}
            className="input"
            type="search"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onSearchKeyDown}
            aria-label="搜索关键词"
            placeholder="输入书名"
          />
          <button className="btn btn-primary search-submit" type="submit">
            搜索
          </button>
        </form>

        {/* 平铺筛选区：有关键词就展开，点一下立即重查（不要求再点「搜索」） */}
        {showFilters ? (
          <SearchFilters
            values={filters}
            onChange={changeFilters}
            activeCount={activeCount}
            categories={categories.data?.items ?? []}
            authors={authors.data?.items ?? []}
            tags={tags.data?.items ?? []}
            loading={categories.loading || tags.loading || authors.loading}
          />
        ) : null}

        {/* 计数摘要只在真的有结果时出现：0 本时下面已经是「没有匹配」的空态，
            再来一行「找到 0 本书」是把同一件事说两遍，还把空态标题挤下去一屏。 */}
        {done && bookCount > 0 ? (
          <div className="search-summary">
            <span className="meta">找到 {formatCount(bookCount)} 本书</span>
            {activeCount > 0 ? (
              <span className="pill search-filter-badge" data-testid="search-active-filters">
                已选 {activeCount} 项
              </span>
            ) : null}
            <span className="pill">按书名搜索</span>
            {activeCount > 0 ? (
              <button
                type="button"
                className="btn btn-sm btn-ghost"
                data-testid="search-filters-clear"
                onClick={clearFilters}
              >
                清除筛选
              </button>
            ) : null}
          </div>
        ) : null}
      </div>

      {loading && !tooShort ? (
        <div data-testid="search-loading">
          <SearchResultSkeleton count={isH5 ? 3 : 4} />
        </div>
      ) : error ? (
        <div>
          <ErrorPanel error={error} title="搜索没能完成" onRetry={retry} />
        </div>
      ) : !trimmed || tooShort || bookCount === 0 ? (
        <>
          <SearchEmpty
            kind={emptyKind()}
            history={history}
            onPick={runSearch}
            onClearHistory={clearHistory}
            filtersActive={activeCount > 0}
            onClearFilters={clearFilters}
          />
          {/* 搜索没有结果时用推荐补位：用户来这一趟是为了找书，别让他空手回去 */}
          <SceneRecommendBlock scene="search-empty" />
        </>
      ) : (
        <div className="search-layout" style={isH5 ? { gridTemplateColumns: "minmax(0,1fr)" } : undefined}>
          <aside aria-label="书名结果">
            <h2>
              书名 <span className="meta">{bookCount}</span>
            </h2>
            <div className="search-books">
              {bookItems.map((book) => (
                <SearchResultItem book={book} keyword={trimmed} isH5={isH5} key={book.id} />
              ))}
            </div>
          </aside>
        </div>
      )}

      {q && !loading && !error && pages > 1 ? (
        <nav className="pagination" aria-label="分页" data-testid="search-pagination">
          <button type="button" aria-label="上一页" disabled={page <= 1} onClick={() => changePage(page - 1)}>
            ←
          </button>
          <span className="is-current num">{page}</span>
          <span className="meta">/ {pages}</span>
          <button
            type="button"
            aria-label="下一页"
            disabled={page >= pages}
            onClick={() => changePage(page + 1)}
          >
            →
          </button>
        </nav>
      ) : null}

      {data?.notice ? <p className="meta">{data.notice}</p> : null}
    </section>
  );
}
