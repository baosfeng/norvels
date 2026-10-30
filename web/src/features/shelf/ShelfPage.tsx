import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../../api/client";
import { useAuthors, useCollections, useTags } from "../../api/hooks";
import type { BookSummary, ReadStatus } from "../../api/types";
import { useShelfMode } from "../../layouts/shelfMode";
import { tierFallbackText } from "../../layouts/tierFallback";
import { READ_STATUS_LABELS, nextReadStatus } from "../../lib/readStatus";
import { showToast } from "../../lib/toastStore";
import { useIsH5 } from "../../lib/tier";
import { hasShelfQuery } from "./shelfQuery";
import { useShelfView } from "./useShelfView";
import { useShelfIds } from "./shelfIds";
import { ShelfMineEmpty, ShelfMineLoginPrompt } from "./ShelfMinePanel";
import { ShelfViewSwitch } from "./ShelfViewSwitch";
import { BatchAuthorDialog } from "./BatchAuthorDialog";
import { FilterHints } from "./FilterHints";
import { ResumeCard } from "./ResumeCard";
import { ShelfFilterSheet } from "./ShelfFilterSheet";
import { ShelfHead } from "./ShelfHead";
import { ShelfList } from "./ShelfList";
import { ShelfToolbar } from "./ShelfToolbar";
import { SORTS, type SortValue } from "./shelfSorts";
import { useShelfFilters } from "./useShelfFilters";
import { useShelfCache, useShelfScrollRestore, shouldRefresh } from "./useShelfScrollRestore";
import { useBookFeed, type BookFeedRestore } from "../../lib/useBookFeed";

/** 我的书架不支持多选：给列表一个稳定空集合，避免每次渲染新建引用 */
const EMPTY_SELECTION: Set<number> = new Set();

const PAGE_SIZE = 24;
/** H5 首屏 30 本，之后每次再取 30 本追加；PC 仍按 PAGE_SIZE 分页 */
const H5_PAGE_SIZE = 30;

/**
 * 书架：两档**同一套筛选**，只换承载形态。
 * - PC：页头下方常驻工具栏（排序 + 分类/作者/书单/评分/状态 + 标签 chips），页码分页。
 * - H5：单列书卡 + 无限滚动，全部筛选收进底部弹层（排序 / 状态 / 评分 / 分类 / 作者 / 书单 / 标签）。
 *
 * 筛选值同时写进地址（`shelfQuery.ts`）与会话缓存：地址负责刷新 / 后退 / 分享，
 * 缓存负责返回时的列表、页码与滚动位置。管理类入口（导入 / 作者 / 驾驶舱）仍是 PC 专属。
 *
 * 状态与副作用拆在三个地方：筛选 → `useShelfFilters`；缓存与滚动 → `useShelfScrollRestore`；
 * 本文件只管取数与渲染。
 */
export function ShelfPage() {
  const isH5 = useIsH5();
  const { mode } = useShelfMode();
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  // 驾驶舱「全部书架 →」（C-23）要落到全部书：带上这个 state 就忽略会话缓存里的筛选
  const resetRequested = (location.state as { resetFilters?: boolean } | null)?.resetFilters === true;
  // 地址上有筛选参数 = 以地址为准（刷新 / 后退 / 分享都能复现同一屏）
  const cached = useShelfCache(hasShelfQuery(searchParams) || resetRequested);
  const filters = useShelfFilters(cached);
  const { sort, setSort, category, filtering, filterCount, resetFilters } = filters;
  // 视图（我的书架 / 全部书）进地址：?view=mine|all，默认按登录态选（B2）
  const shelfView = useShelfView();
  const shelfIds = useShelfIds();
  const mine = shelfView.view === "mine";
  // 我的书架按加入时间倒序、不参与筛选：那些筛选器只属于共享书库
  const feedFiltering = !mine && filtering;

  const [statusOverrides, setStatusOverrides] = useState<Record<number, ReadStatus>>({});
  const [sheetOpen, setSheetOpen] = useState(false);
  // 多选模式：右上角「选择」进入；选中后批量设作者（循环 PATCH，纯前端）
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [batchOpen, setBatchOpen] = useState(false);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  // 清除筛选后焦点回到排序下拉（C-29）；404 回书架后焦点交给页标题（C-18）
  const sortRef = useRef<HTMLSelectElement | null>(null);
  const titleRef = useRef<HTMLHeadingElement | null>(null);

  // 筛选项的候选：PC 常驻在工具栏里，H5 只在弹层打开时才去取（首屏不多发三个请求）
  const optionsEnabled = !isH5 || sheetOpen;
  const authorOptions = useAuthors({ sort: "books", size: 50 }, { enabled: optionsEnabled });
  const tagOptions = useTags({ enabled: optionsEnabled });
  const collectionOptions = useCollections({ enabled: optionsEnabled });

  const pageSize = isH5 ? H5_PAGE_SIZE : PAGE_SIZE;
  const [now] = useState(() => Date.now());
  // 会话缓存只属于「全部书」（它记的是 /api/books 的列表、筛选与页码）：
  // 我的书架是另一份内容，拿它恢复会出现"书架里显示着书库里没加入的书"。
  const restore = useMemo<BookFeedRestore | undefined>(() => {
    if (mine || !cached || cached.size !== pageSize || cached.items.length === 0) return undefined;
    return { items: cached.items, total: cached.total, page: cached.page, refresh: shouldRefresh(cached, now) };
  }, [mine, cached, pageSize, now]);
  // 未登录时「我的书架」不发请求：/api/shelf 会 401，先渲染成错误面板只会误导
  const feedEnabled = !mine || shelfView.loggedIn;
  const feed = useBookFeed({
    source: mine ? "shelf" : "books",
    enabled: feedEnabled,
    sort,
    authorId: filters.authorId ?? undefined,
    tag: filters.tag ?? undefined,
    readStatus: filters.readStatus ?? undefined,
    rating: filters.rating ?? undefined,
    collection: filters.collection ?? undefined,
    categoryL1: category.l1 ?? undefined,
    categoryL2: category.l2 ?? undefined,
    size: pageSize,
    mode: isH5 ? "infinite" : "paged",
    restore,
  });

  const { items, total, loading, error, reload, loadMore, hasMore } = feed;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const sortLabel = SORTS.find((s) => s.value === sort)?.label ?? "";
  // 老浏览器 / 测试环境没有 IntersectionObserver：改用底部「加载更多」按钮兜底
  const canObserve = typeof IntersectionObserver !== "undefined";

  // 滚动/缓存恢复同样只在「全部书」生效（我的书架不写会话缓存，避免覆盖书库那份）
  const scroll = useShelfScrollRestore({
    snapshot: {
      sort,
      authorId: filters.authorId,
      tag: filters.tag,
      readStatus: filters.readStatus,
      rating: filters.rating,
      collection: filters.collection,
      category,
      size: pageSize,
      mode: isH5 ? "infinite" : "paged",
      items,
      total,
      page: feed.page,
    },
    cached: mine ? null : cached,
    enabled: !mine && items.length > 0,
  });

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

  // 404 的「回书架」把焦点交给页标题（C-18 离开空态）
  useEffect(() => {
    if ((location.state as { focusTitle?: boolean } | null)?.focusTitle) titleRef.current?.focus();
  }, [location.state]);

  // 宽屏访问首页会落到书架（C-04 边界）：把「首页只在窄屏提供」说清楚，一次性 state 用完即清
  useEffect(() => {
    const text = tierFallbackText((location.state as { from?: string } | null)?.from);
    if (!text) return;
    showToast(text);
    navigate(`${location.pathname}${location.search}`, { replace: true, state: null });
  }, [location.state, location.pathname, location.search, navigate]);

  function scrollToTopOfList() {
    // 换了排序 / 筛选 / 页码都回到列表顶部；老浏览器与测试环境可能没有 scrollTo
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 不影响结果 */
    }
  }

  function chooseSort(next: SortValue) {
    setSheetOpen(false);
    if (next === sort) return;
    setSort(next);
    scrollToTopOfList();
  }

  /** PC 行内排序（C-19）：选中即生效，页码由取数钩子按查询键重置为 1。 */
  function chooseSortOnPc(next: SortValue) {
    if (next === sort) return;
    setSort(next);
    scrollToTopOfList();
  }

  function changePage(next: number) {
    scrollToTopOfList();
    feed.setPage(next);
  }

  /** 清空筛选（C-29）：焦点回排序下拉。 */
  function clearFilters() {
    resetFilters();
    sortRef.current?.focus();
    scrollToTopOfList();
  }

  function chooseReadStatus(next: Exclude<ReadStatus, ""> | null) {
    setSheetOpen(false);
    filters.setReadStatus(next);
    scrollToTopOfList();
  }

  /** 弹层里的其余筛选（评分 / 作者 / 书单 / 分类 / 标签）：选中即生效并收起弹层。 */
  function chooseInSheet(apply: () => void) {
    setSheetOpen(false);
    apply();
    scrollToTopOfList();
  }

  /** 角标循环：想读 → 在读 → 已读 → 未标记；先乐观回显，失败再退回原值。 */
  async function cycleStatus(book: BookSummary) {
    const current = statusOverrides[book.id] ?? book.readStatus ?? "";
    const next = nextReadStatus(current);
    setStatusOverrides((prev) => ({ ...prev, [book.id]: next }));
    try {
      await api.updateBook(book.id, { readStatus: next });
      // 结果原文要能让用户对上刚才点的是哪本
      showToast(`《${book.title}》标记为「${READ_STATUS_LABELS[next]}」。`);
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
      <ShelfHead
        isH5={isH5}
        loading={loading}
        total={total}
        titleRef={titleRef}
        sortLabel={sortLabel}
        filtering={filtering}
        authorId={filters.authorId}
        tag={filters.tag}
        collection={filters.collection}
        selecting={selecting}
        selectedCount={selected.size}
        sheetOpen={sheetOpen}
        filterCount={filterCount}
        onToggleSelecting={toggleSelecting}
        onOpenBatch={() => setBatchOpen(true)}
        onOpenSheet={() => setSheetOpen(true)}
        mine={mine}
      />

      <ShelfViewSwitch
        view={shelfView.view}
        onChange={shelfView.setView}
        mineCount={shelfIds.ready ? shelfIds.ids.size : null}
      />

      {/* 我的书架按加入时间倒序，排序与筛选对它不成立：mine 视图不渲染整排筛选器 */}
      {isH5 || mine ? null : <ResumeCard />}

      {isH5 || mine ? null : (
        <ShelfToolbar
          sortRef={sortRef}
          sort={sort}
          onChooseSort={chooseSortOnPc}
          readStatus={filters.readStatus}
          onChooseReadStatus={chooseReadStatus}
          rating={filters.rating}
          onRatingChange={filters.setRating}
          authorId={filters.authorId}
          onAuthorChange={filters.setAuthorId}
          authors={authorOptions.data?.items ?? []}
          collection={filters.collection}
          onCollectionChange={filters.setCollection}
          category={category}
          onCategoryChange={filters.setCategory}
          tag={filters.tag}
          onTagChange={filters.setTag}
          tags={tagOptions.data?.items ?? []}
          filtering={filtering}
          onResetFilters={clearFilters}
        />
      )}

      {isH5 || mine ? null : <FilterHints authors={authorOptions} tags={tagOptions} />}

      {mine && !shelfView.loggedIn ? (
        <ShelfMineLoginPrompt next={location.pathname + location.search} />
      ) : (
        <ShelfList
          isH5={isH5}
          loading={loading}
          error={error}
          items={items}
          total={total}
          filtering={feedFiltering}
          mode={mode}
          statusOverrides={statusOverrides}
          selecting={mine ? false : selecting}
          selected={mine ? EMPTY_SELECTION : selected}
          feed={feed}
          pages={pages}
          canObserve={canObserve}
          sentinelRef={sentinelRef}
          onRetry={reload}
          onResetFilters={clearFilters}
          onCycle={(book) => void cycleStatus(book)}
          onSelect={toggleSelected}
          onOpen={scroll.rememberBook}
          onPage={changePage}
          emptySlot={mine ? <ShelfMineEmpty filtering={false} /> : undefined}
        />
      )}

      {isH5 && sheetOpen ? (
        <ShelfFilterSheet
          sort={sort}
          readStatus={filters.readStatus}
          rating={filters.rating}
          authorId={filters.authorId}
          authors={authorOptions.data?.items ?? []}
          collection={filters.collection}
          collections={collectionOptions.data?.items ?? []}
          category={category}
          tag={filters.tag}
          tags={tagOptions.data?.items ?? []}
          filtering={filtering}
          onChooseSort={chooseSort}
          onChooseReadStatus={chooseReadStatus}
          onChooseRating={(next) => chooseInSheet(() => filters.setRating(next))}
          onChooseAuthor={(next) => chooseInSheet(() => filters.setAuthorId(next))}
          onChooseCollection={(next) => chooseInSheet(() => filters.setCollection(next))}
          onChooseCategory={(next) => chooseInSheet(() => filters.setCategory(next))}
          onChooseTag={(next) => chooseInSheet(() => filters.setTag(next))}
          onResetFilters={() => chooseInSheet(clearFilters)}
          onClose={() => setSheetOpen(false)}
        />
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
