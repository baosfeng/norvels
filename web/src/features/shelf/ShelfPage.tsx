import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import { useAuthors, useTags } from "../../api/hooks";
import type { BookSummary, ReadStatus } from "../../api/types";
import { Cover } from "../../components/Cover";
import { CardSkeleton, EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { useShelfMode } from "../../layouts/shelfMode";
import { formatCount } from "../../lib/format";
import {
  READ_STATUS_FILTERS,
  READ_STATUS_LABELS,
  nextReadStatus,
  readStatusFilterLabel,
} from "../../lib/readStatus";
import { stripTags } from "../../lib/text";
import { showToast } from "../../lib/toastStore";
import { BatchAuthorDialog } from "./BatchAuthorDialog";
import { useIsH5 } from "../../lib/tier";
import {
  SHELF_REFRESH_MIN_AGE_MS,
  readShelfCache,
  writeShelfCache,
  type ShelfFromState,
} from "../../lib/shelfCache";
import { useBookFeed, type BookFeedRestore } from "../../lib/useBookFeed";

const SORTS = [
  { value: "updated", label: "最近更新" },
  { value: "imported", label: "导入时间" },
  { value: "title", label: "书名" },
  { value: "chapters", label: "章节数" },
  { value: "words", label: "字数" },
] as const;

type SortValue = (typeof SORTS)[number]["value"];

const PAGE_SIZE = 24;
/** H5 首屏 30 本，之后每次再取 30 本追加；PC 仍按 PAGE_SIZE 分页 */
const H5_PAGE_SIZE = 30;

/**
 * 书架：PC 是工作台（排序 / 作者筛选 / 标签筛选 / 视图切换 / 分页），
 * H5 是单列简表——不做筛选与分页，但保留排序（底部弹层）与无限滚动。
 * 管理类入口（导入 / 作者 / 驾驶舱）仍是 PC 专属，见 00-brief §0.6。
 */
export function ShelfPage() {
  const isH5 = useIsH5();
  const { mode } = useShelfMode();
  // /read 在 AppShell 之外：进阅读器时本页整体卸载，靠会话缓存把列表与滚动位置带回来
  const cached = useMemo(() => readShelfCache(), []);
  const [sort, setSort] = useState<SortValue>(() =>
    SORTS.some((option) => option.value === cached?.sort) ? (cached?.sort as SortValue) : "updated",
  );
  const [authorId, setAuthorId] = useState<number | null>(cached?.authorId ?? null);
  const [tag, setTag] = useState<string | null>(cached?.tag ?? null);
  const [readStatus, setReadStatus] = useState<Exclude<ReadStatus, ""> | null>(cached?.readStatus ?? null);
  // 角标点击后的乐观值：列表重新取数会拿回后端值，这里只兜住「刚点完还没刷新」的空窗
  const [statusOverrides, setStatusOverrides] = useState<Record<number, ReadStatus>>({});
  const [sheetOpen, setSheetOpen] = useState(false);
  // 多选模式：右上角「选择」进入；选中后批量设作者（循环 PATCH，纯前端）
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [batchOpen, setBatchOpen] = useState(false);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  // 最近打开的书（返回时滚进视口并高亮）与「滚动位置只恢复一次」的标记
  const lastBookRef = useRef<number | null>(cached?.lastBookId ?? null);
  const restoredRef = useRef(false);
  // 记住「最后一次看到的滚动位置」：卸载时 DOM 已换成更短的下一页，再读 window.scrollY 会被夹到 0
  const scrollYRef = useRef(cached?.scrollTop ?? 0);

  // 筛选项的候选：作者与标签都是 PC 专属，H5 不发这两个请求
  const authorOptions = useAuthors({ sort: "books", size: 50 }, { enabled: !isH5 });
  const tagOptions = useTags({ enabled: !isH5 });

  const pageSize = isH5 ? H5_PAGE_SIZE : PAGE_SIZE;
  // 缓存命中就先用它渲染；够新（60 秒内）时连后台刷新都不做，返回瞬间不再发第 1 页
  const restore = useMemo<BookFeedRestore | undefined>(() => {
    if (!cached || cached.size !== pageSize || cached.items.length === 0) return undefined;
    return {
      items: cached.items,
      total: cached.total,
      page: cached.page,
      refresh: Date.now() - cached.savedAt > SHELF_REFRESH_MIN_AGE_MS,
    };
  }, [cached, pageSize]);
  const feed = useBookFeed({
    sort,
    authorId: authorId ?? undefined,
    tag: tag ?? undefined,
    readStatus: readStatus ?? undefined,
    size: pageSize,
    mode: isH5 ? "infinite" : "paged",
    restore,
  });

  const { items, total, loading, error, reload, loadMore, hasMore } = feed;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const filtering = authorId !== null || tag !== null || readStatus !== null;
  const sortLabel = SORTS.find((s) => s.value === sort)?.label ?? "";
  // 老浏览器 / 测试环境没有 IntersectionObserver：改用底部「加载更多」按钮兜底
  const canObserve = typeof IntersectionObserver !== "undefined";

  /** 把当前列表状态写进会话缓存：滚动位置取实时值，最近打开的书取 ref（避免闭包里的旧 state）。 */
  const persistShelf = useCallback(() => {
    if (items.length === 0) return;
    writeShelfCache({
      sort,
      authorId,
      tag,
      readStatus,
      size: pageSize,
      mode: isH5 ? "infinite" : "paged",
      items,
      total,
      page: feed.page,
      scrollTop: Math.round(scrollYRef.current),
      lastBookId: lastBookRef.current,
    });
  }, [sort, authorId, tag, readStatus, pageSize, isH5, items, total, feed.page]);

  /** 进详情/阅读器前记一下「最近打开的书 + 当前滚动位置」，返回时据此定位。 */
  function rememberBook(id: number) {
    lastBookRef.current = id;
    scrollYRef.current = typeof window === "undefined" ? 0 : window.scrollY || 0;
    persistShelf();
    writeShelfCache({
      sort,
      authorId,
      tag,
      readStatus,
      size: pageSize,
      mode: isH5 ? "infinite" : "paged",
      items,
      total,
      page: feed.page,
      scrollTop: Math.round(scrollYRef.current),
      lastBookId: id,
    });
  }

  // 滚动停止 300ms 写一次；卸载（进阅读器）前再补一次，保证返回时位置准确
  useEffect(() => {
    let timer = 0;
    function onScroll() {
      scrollYRef.current = window.scrollY || 0;
      window.clearTimeout(timer);
      timer = window.setTimeout(() => persistShelf(), 300);
    }
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("scroll", onScroll);
      persistShelf();
    };
  }, [persistShelf]);

  // 加载完成 / 筛选排序变化：立刻落一次
  useEffect(() => {
    persistShelf();
  }, [persistShelf, items.length]);

  // 恢复滚动位置，再把最近打开的那本滚进视口并高亮 1.5 秒（已经在视口里就只高亮，不动位置）
  useEffect(() => {
    if (restoredRef.current || !cached || items.length === 0) return;
    restoredRef.current = true;
    scrollYRef.current = cached.scrollTop;
    window.scrollTo({ top: cached.scrollTop });
    const target = cached.lastBookId;
    if (target === null) return;
    const node = document.querySelector<HTMLElement>(`[data-book-id="${target}"]`);
    if (!node) return;
    // 位置优先：缓存里有滚动位置就按它恢复，不再把最近那本拽到中间（否则位置差会被放大）。
    // 只有没有可用位置（新会话 / 从详情页深链进来）时才把这本书滚进视口居中。
    if (cached.scrollTop <= 0) node.scrollIntoView({ block: "center" });
    node.classList.add("is-recent");
    window.setTimeout(() => node.classList.remove("is-recent"), 1500);
  }, [cached, items.length]);

  // H5：滚到底自动取下一页。换排序/筛选会换查询键，钩子自己丢弃在途响应并回到第 1 页。
  useEffect(() => {
    const node = sentinelRef.current;
    if (!isH5 || !node || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "240px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [isH5, loadMore, items.length, hasMore]);

  // 底部弹层打开时锁住页面滚动，避免背景跟着滑
  useEffect(() => {
    document.body.classList.toggle("h5-sheet-open", sheetOpen);
    return () => document.body.classList.remove("h5-sheet-open");
  }, [sheetOpen]);

  // 底部弹层：Esc 收起（点遮罩与「关闭」同理）
  useEffect(() => {
    if (!sheetOpen) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSheetOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [sheetOpen]);

  function chooseSort(next: SortValue) {
    setSheetOpen(false);
    if (next === sort) return;
    setSort(next);
    // 换了排序就是一份新列表：回到顶部，页码由钩子重置为 1
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 老浏览器与测试环境可能没有 scrollTo：不影响排序结果 */
    }
  }

  function resetFilters() {
    setAuthorId(null);
    setTag(null);
    setReadStatus(null);
  }

  function chooseReadStatus(next: Exclude<ReadStatus, ""> | null) {
    setSheetOpen(false);
    setReadStatus(next);
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 老浏览器与测试环境可能没有 scrollTo：不影响筛选结果 */
    }
  }

  /** 角标循环：想读 → 在读 → 已读 → 未标记；先乐观回显，失败再退回原值。 */
  async function cycleStatus(book: BookSummary) {
    const current = statusOverrides[book.id] ?? book.readStatus ?? "";
    const next = nextReadStatus(current);
    setStatusOverrides((prev) => ({ ...prev, [book.id]: next }));
    try {
      await api.updateBook(book.id, { readStatus: next });
    } catch {
      setStatusOverrides((prev) => ({ ...prev, [book.id]: current }));
      showToast("阅读状态没能保存，稍后再试。");
    }
  }

  function toggleSelecting() {
    setSelecting((prev) => !prev);
    setSelected(new Set());
    setBatchOpen(false);
  }

  function toggleSelected(id: number) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  const selectedBooks = items.filter((book) => selected.has(book.id));

  return (
    <section className="view-shelf" data-testid="view:shelf" data-od-id="shelf">
      <div className="page-head">
        <div>
          <p className="eyebrow">书架 · {loading ? "读取中" : `${formatCount(total)} 本`}</p>
          <h1>{isH5 ? "书架" : "最近入库"}</h1>
          {isH5 ? null : (
            <p className="sub">
              按{sortLabel}排序 · 共 {formatCount(total)} 本
              {filtering ? "（已筛选）" : ""}
            </p>
          )}
        </div>
        <div className="shelf-head-actions">
          <button
            className="btn"
            type="button"
            data-testid="shelf-select-toggle"
            aria-pressed={selecting}
            onClick={toggleSelecting}
          >
            {selecting ? "退出选择" : "选择"}
          </button>
          {selecting ? (
            <button
              className="btn btn-primary"
              type="button"
              data-testid="shelf-batch-author"
              disabled={selected.size === 0}
              onClick={() => setBatchOpen(true)}
            >
              批量设作者（{selected.size}）
            </button>
          ) : null}
          {isH5 ? (
            <button
              className="btn shelf-sort-trigger"
              type="button"
              data-testid="shelf-sort-trigger"
              aria-haspopup="dialog"
              aria-expanded={sheetOpen}
              onClick={() => setSheetOpen(true)}
            >
              {readStatus === null ? `排序：${sortLabel}` : `${readStatusFilterLabel(readStatus)} · ${sortLabel}`}
            </button>
          ) : null}
        </div>
      </div>

      {isH5 ? null : (
        <div className="toolbar" data-testid="shelf-toolbar">
          <label>
            排序
            <select className="select" value={sort} onChange={(e) => setSort(e.target.value as SortValue)}>
              {SORTS.map((s) => (
                <option key={s.value} value={s.value}>
                  {s.label}
                </option>
              ))}
            </select>
          </label>

          <label>
            阅读状态
            <select
              className="select"
              value={readStatus ?? ""}
              data-testid="filter-read-status"
              onChange={(e) => chooseReadStatus((e.target.value || null) as Exclude<ReadStatus, ""> | null)}
            >
              {READ_STATUS_FILTERS.map((option) => (
                <option key={option.label} value={option.value ?? ""}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>

          <label>
            作者
            <select
              className="select"
              value={authorId ?? ""}
              onChange={(e) => setAuthorId(e.target.value ? Number(e.target.value) : null)}
              data-testid="filter-author"
            >
              <option value="">全部作者</option>
              {(authorOptions.data?.items ?? []).map((author) => (
                <option key={author.id} value={author.id}>
                  {author.name || "佚名"}（{author.bookCount}）
                </option>
              ))}
            </select>
          </label>

          <div className="chips" role="group" aria-label="标签筛选" data-testid="filter-tags">
            <button type="button" className={`chip${tag === null ? " is-active" : ""}`} onClick={() => setTag(null)}>
              全部
            </button>
            {(tagOptions.data?.items ?? []).map((item) => (
              <button
                key={item.id}
                type="button"
                className={`chip${tag === item.name ? " is-active" : ""}`}
                onClick={() => setTag(item.name)}
              >
                {item.name}
              </button>
            ))}
          </div>

          {filtering ? (
            <button className="btn btn-ghost" type="button" onClick={resetFilters}>
              清除筛选
            </button>
          ) : null}
        </div>
      )}

      {isH5 ? null : <FilterHints authors={authorOptions} tags={tagOptions} />}

      {loading ? (
        <div data-testid="shelf-loading">
          <LoadingPanel
            title="正在读取书架"
            description={`正在整理本地书目，${formatCount(total || 0) || "全部"} 本大约需要 1–2 秒。`}
          />
          <CardSkeleton count={isH5 ? 3 : 4} />
        </div>
      ) : error ? (
        <ErrorPanel error={error} title="书架没能加载" onRetry={reload} testId="shelf-error" />
      ) : items.length === 0 ? (
        <EmptyPanel
          title={filtering ? "没有符合筛选条件的书" : "书架还是空的"}
          description={
            filtering
              ? "换一个作者或标签试试，或者清除筛选看全部书目。"
              : isH5
                ? "先在电脑上把存放 txt 的目录导入，书目会同步到这里。"
                : "把存放 txt 的目录填进导入页，完成后书目会自动出现在这里。"
          }
          testId="shelf-empty"
          actions={
            filtering ? (
              <button className="btn btn-primary" type="button" onClick={resetFilters}>
                清除筛选
              </button>
            ) : isH5 ? null : (
              <Link className="btn btn-primary" to="/import">
                去导入
              </Link>
            )
          }
        />
      ) : (
        <>
          <div className={`book-grid ${mode === "list" && !isH5 ? "is-list" : ""}`.trim()} data-testid="shelf-live">
            {items.map((book) => (
              <BookCard
                key={book.id}
                book={book}
                compact={isH5}
                status={statusOverrides[book.id] ?? book.readStatus ?? ""}
                onCycle={() => void cycleStatus(book)}
                selecting={selecting}
                selected={selected.has(book.id)}
                onSelect={() => toggleSelected(book.id)}
                onOpen={() => rememberBook(book.id)}
              />
            ))}
          </div>

          {isH5 ? (
            <div className="shelf-more" data-testid="shelf-more">
              {feed.loadingMore ? (
                <p className="meta" role="status" data-testid="shelf-more-loading">
                  正在加载更多…
                </p>
              ) : null}
              {feed.errorMore ? (
                <div className="shelf-more-error" data-testid="shelf-more-error">
                  <p className="meta" role="alert">
                    后面的书没取回来，检查网络后再试一次。
                  </p>
                  <button className="btn" type="button" onClick={feed.loadMore}>
                    重试
                  </button>
                </div>
              ) : null}
              {feed.hasMore ? (
                <>
                  <div ref={sentinelRef} data-testid="shelf-sentinel" aria-hidden="true" style={{ height: 1 }} />
                  {canObserve ? null : (
                    <button className="btn" type="button" onClick={feed.loadMore} data-testid="shelf-load-more">
                      加载更多
                    </button>
                  )}
                </>
              ) : (
                <p className="meta" data-testid="shelf-end">
                  没有更多了 · 共 {formatCount(total)} 本
                </p>
              )}
            </div>
          ) : pages <= 1 ? null : (
            <nav className="pagination" aria-label="分页">
              <button
                type="button"
                aria-label="上一页"
                disabled={feed.page <= 1}
                onClick={() => feed.setPage(feed.page - 1)}
              >
                ←
              </button>
              <span className="is-current num">{feed.page}</span>
              <span className="meta">/ {pages}</span>
              <button
                type="button"
                aria-label="下一页"
                disabled={feed.page >= pages}
                onClick={() => feed.setPage(feed.page + 1)}
              >
                →
              </button>
            </nav>
          )}
        </>
      )}

      {isH5 && sheetOpen ? (
        <div className="sheet-backdrop" data-testid="shelf-sort-sheet" onClick={() => setSheetOpen(false)}>
          <div
            className="sheet"
            role="dialog"
            aria-modal="true"
            aria-label="排序方式"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="sheet-head">
              <strong>排序方式</strong>
              <button className="btn btn-ghost" type="button" onClick={() => setSheetOpen(false)}>
                关闭
              </button>
            </div>
            <div className="sheet-section" role="group" aria-label="阅读状态筛选">
              <span className="sheet-section-title">阅读状态</span>
              {READ_STATUS_FILTERS.map((option) => (
                <button
                  key={option.label}
                  type="button"
                  className={`sheet-option${option.value === readStatus ? " is-active" : ""}`}
                  data-testid={`read-status-option-${option.value ?? "all"}`}
                  aria-pressed={option.value === readStatus}
                  onClick={() => chooseReadStatus(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
            <div className="sheet-list">
              {SORTS.map((s) => (
                <button
                  key={s.value}
                  type="button"
                  className={`sheet-option${s.value === sort ? " is-active" : ""}`}
                  data-testid={`sort-option-${s.value}`}
                  aria-pressed={s.value === sort}
                  onClick={() => chooseSort(s.value)}
                >
                  {s.label}
                </button>
              ))}
            </div>
          </div>
        </div>
      ) : null}
      {batchOpen ? (
        <BatchAuthorDialog
          books={selectedBooks}
          onClose={() => setBatchOpen(false)}
          onSaved={(count) => {
            setBatchOpen(false);
            setSelecting(false);
            setSelected(new Set());
            reload();
            showToast(`已批量更新 ${count} 本书的作者`);
          }}
        />
      ) : null}
    </section>
  );
}

/** 筛选项自身的加载/失败/空态：不占主内容区，但也不静默失败。 */
function FilterHints({
  authors,
  tags,
}: {
  authors: { loading: boolean; error: unknown; data: { items: unknown[] } | null };
  tags: { loading: boolean; error: unknown; data: { items: unknown[] } | null };
}) {
  const notes: string[] = [];
  if (tags.loading) notes.push("正在读取标签…");
  else if (tags.error) notes.push("标签暂时读不到，可以先用作者筛选。");
  else if ((tags.data?.items.length ?? 0) === 0) notes.push("暂无标签（导入的书还没有标签信息）。");

  if (authors.error) notes.push("作者列表暂时读不到，作者筛选可能不全。");

  if (notes.length === 0) return null;
  return (
    <p className="meta filter-hints" data-testid="filter-hints">
      {notes.join(" ")}
    </p>
  );
}

/**
 * 书卡：角标是独立按钮（不套在链接里，避免交互元素嵌套），点一下循环切换阅读状态。
 * 多选模式下整张卡片变成选择按钮（同样不嵌套链接），右上角显示勾选状态。
 */
function BookCard({
  book,
  compact,
  status,
  onCycle,
  selecting,
  selected,
  onSelect,
  onOpen,
}: {
  book: BookSummary;
  compact: boolean;
  status: ReadStatus;
  onCycle: () => void;
  selecting: boolean;
  selected: boolean;
  onSelect: () => void;
  onOpen: () => void;
}) {
  // 角标放在底部元信息行里（静态排布），不再绝对定位压书名
  const badge = (
    <button
      className="book-status"
      type="button"
      data-testid={`book-status-${book.id}`}
      data-status={status}
      aria-label={`阅读状态：${READ_STATUS_LABELS[status]}，点击切换`}
      onClick={onCycle}
    >
      {READ_STATUS_LABELS[status]}
    </button>
  );

  const content = (
    <>
      <Cover
        title={book.title}
        bookId={book.id}
        updatedAt={book.updatedAt}
        hasCover={book.hasCover}
        showLabel={!compact}
      />
      <div className="book-card-body">
        <h3 className="book-title">{book.title}</h3>
        <p className="book-author">{book.authorName || "佚名"}</p>
        <p className="book-meta">
          <span className="num">{formatCount(book.chapterCount)} 章</span>
          <span className="num">{formatCount(book.wordCount)} 字</span>
          {book.volumeCount > 0 ? <span>{book.volumeCount} 卷</span> : null}
          {selecting ? null : badge}
        </p>
        {compact ? null : <p className="book-intro">{stripTags(book.intro) || "这本书还没有简介。"}</p>}
      </div>
    </>
  );

  return (
    <article
      className={`book-card${selecting ? " is-selecting" : ""}${selected ? " is-selected" : ""}`}
      data-status={status}
      data-book-id={book.id}
    >
      {selecting ? (
        <button
          className="book-link"
          type="button"
          aria-pressed={selected}
          aria-label={`选择《${book.title}》`}
          data-testid={`book-select-${book.id}`}
          onClick={onSelect}
        >
          {content}
        </button>
      ) : (
        <Link
          className="book-link"
          to={`/book/${book.id}`}
          state={{ from: "shelf", scrollTop: Math.round(typeof window === "undefined" ? 0 : window.scrollY || 0) } satisfies ShelfFromState}
          onClick={onOpen}
        >
          {content}
        </Link>
      )}
      {selecting ? (
        <span className="book-check" aria-hidden="true">
          {selected ? "✓" : ""}
        </span>
      ) : null}
    </article>
  );
}
