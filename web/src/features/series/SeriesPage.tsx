import { useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useSeries } from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { buildSeriesCards } from "../../lib/seriesGroup";
import { formatCount } from "../../lib/format";

const PAGE_SIZE = 24;

/** 系列卡的「最近更新」：契约示例是日期（10 月 24 日），这里固定用日期，不用相对时间。 */
function updatedDate(iso: string | null | undefined): string {
  if (!iso) return "还没有章节";
  const time = Date.parse(iso);
  if (Number.isNaN(time)) return "还没有章节";
  const d = new Date(time);
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日`;
}

/**
 * 系列列表（P3 发现域，PC 专属）：按系列聚合书目。
 * 数据全部来自 GET /api/series（后端已把书数 / 字数 / 最近更新 / 封面书算好），
 * 前端只做「字段 → 人话」与分页；筛选条件放 URL（q/page），刷新与回退都还原同一屏。
 */
export function SeriesPage() {
  const [params, setParams] = useSearchParams();
  const q = (params.get("q") ?? "").trim();
  const page = Math.max(1, Number(params.get("page") ?? 1) || 1);
  const [draft, setDraft] = useState(q);
  const searchRef = useRef<HTMLInputElement | null>(null);
  const feed = useSeries({ q: q || undefined, page, size: PAGE_SIZE });

  const model = useMemo(() => buildSeriesCards(feed.data?.items ?? []), [feed.data]);
  // 卡上的「最近更新」按契约示例显示日期（10 月 24 日），不跟随「几分钟前」的相对时间
  const updatedByName = useMemo(
    () => new Map((feed.data?.items ?? []).map((item) => [(item.name ?? "").trim(), item.lastUpdatedAt])),
    [feed.data],
  );
  const total = feed.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  function apply(next: { q?: string; page?: number }) {
    const merged = new URLSearchParams(params);
    const keyword = (next.q ?? q).trim();
    if (keyword) merged.set("q", keyword);
    else merged.delete("q");
    const nextPage = next.page ?? 1;
    if (nextPage > 1) merged.set("page", String(nextPage));
    else merged.delete("page");
    setParams(merged, { replace: false });
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    apply({ q: draft, page: 1 });
  }

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    // Esc 清空并回到全部系列，焦点留在输入框（C-56）
    if (event.key === "Escape") {
      event.preventDefault();
      setDraft("");
      apply({ q: "", page: 1 });
      return;
    }
    if (event.key === "Enter" && event.nativeEvent.isComposing) event.preventDefault();
  }

  function clearSearch() {
    setDraft("");
    apply({ q: "", page: 1 });
    searchRef.current?.focus();
  }

  function goPage(next: number) {
    apply({ page: next });
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 测试环境没有 scrollTo */
    }
  }

  function retry() {
    feed.reload();
    searchRef.current?.focus();
  }

  return (
    <section data-testid="view:series" data-od-id="series">
      <div className="page-head">
        <div>
          <p className="eyebrow">系列 · {feed.loading ? "读取中" : `${formatCount(total)} 个`}</p>
          <h1>系列</h1>
          <p className="sub">
            按系列聚合书目
            {q ? ` · 关键词「${q}」` : ""}
            {model.cards.length > 0 ? ` · 本页 ${formatCount(model.bookTotal)} 本` : ""}
          </p>
        </div>
      </div>

      <form className="searchbar" onSubmit={submit} role="search" data-testid="series-search" data-od-id="C-56">
        <input
          ref={searchRef}
          className="input"
          type="search"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={onSearchKeyDown}
          placeholder="搜系列名"
          aria-label="搜索系列名"
          data-testid="series-search-input"
        />
        <button className="btn btn-primary" type="submit">
          搜索
        </button>
        {q ? (
          <button
            className="btn btn-ghost"
            type="button"
            data-testid="series-search-clear"
            onClick={clearSearch}
          >
            清除
          </button>
        ) : null}
      </form>

      {feed.loading ? (
        <LoadingPanel title="正在整理系列" description="正在按系列归并书目，稍等一下。" />
      ) : feed.error ? (
        <div data-od-id="C-59">
          <ErrorPanel error={feed.error} title="系列没能加载" onRetry={retry} testId="series-error" />
        </div>
      ) : model.empty ? (
        <div data-od-id="C-58">
          <EmptyPanel
            title={q ? "没有匹配的系列" : "还没有系列"}
            description={
              q
                ? "换个关键词试试，或者清除搜索看全部系列。"
                : "书的元信息里带系列名（例如多部成套的作品）时，会聚合到这里。"
            }
            testId="series-empty"
            actions={
              q ? (
                <button className="btn btn-primary" type="button" onClick={clearSearch}>
                  清除搜索
                </button>
              ) : (
                <Link className="btn btn-primary" to="/shelf">
                  去书架
                </Link>
              )
            }
          />
        </div>
      ) : (
        <>
          <div className="book-grid" data-testid="series-live">
            {model.cards.map((card) => (
              <article className="book-card" key={card.name} data-testid={`series-card-${card.name}`} data-od-id="C-57">
                <Link className="book-link" to={`/series/${encodeURIComponent(card.name)}`}>
                  <Cover title={card.name} bookId={card.coverBookId} hasCover={card.hasCover} />
                  <div className="book-card-body">
                    <h3 className="book-title">{card.name}</h3>
                    <p className="book-author">
                      {card.bookCountLabel} · {card.wordCountLabel}
                    </p>
                    <p className="book-meta">
                      <span>最近更新 {updatedDate(updatedByName.get(card.name))}</span>
                    </p>
                  </div>
                </Link>
              </article>
            ))}
          </div>

          {pages <= 1 ? null : (
            <nav className="pagination" aria-label="分页" data-testid="series-pagination">
              <button type="button" aria-label="上一页" data-od-id="C-60" disabled={page <= 1} onClick={() => goPage(page - 1)}>
                ←
              </button>
              <span className="is-current num">{page}</span>
              <span className="meta">/ {pages}</span>
              <button type="button" aria-label="下一页" data-od-id="C-61" disabled={page >= pages} onClick={() => goPage(page + 1)}>
                →
              </button>
            </nav>
          )}
        </>
      )}
    </section>
  );
}
