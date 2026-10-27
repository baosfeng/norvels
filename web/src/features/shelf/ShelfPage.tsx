import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../../api/client";
import { useAuthors, useTags } from "../../api/hooks";
import type { BookSummary, ReadStatus } from "../../api/types";
import { useShelfMode } from "../../layouts/shelfMode";
import { tierFallbackText } from "../../layouts/tierFallback";
import {
  READ_STATUS_LABELS,
  nextReadStatus,
} from "../../lib/readStatus";
import { showToast } from "../../lib/toastStore";
import { BatchAuthorDialog } from "./BatchAuthorDialog";
import type { CategoryValue } from "./CategoryFilter";
import { ResumeCard } from "./ResumeCard";
import { FilterHints } from "./FilterHints";
import { ShelfSortSheet } from "./ShelfSortSheet";
import { ShelfToolbar } from "./ShelfToolbar";
import { ShelfHead } from "./ShelfHead";
import { ShelfList } from "./ShelfList";
import { SORTS, type SortValue } from "./shelfSorts";
import { useIsH5 } from "../../lib/tier";
import {
  SHELF_REFRESH_MIN_AGE_MS,
  readShelfCache,
  writeShelfCache,
} from "../../lib/shelfCache";
import { useBookFeed, type BookFeedRestore } from "../../lib/useBookFeed";

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
  // /shelf?tag=X 是详情页标签 chip 的落地地址：链接要能复制、能直接打开，
  // 所以地址上的标签优先于会话缓存（缓存里的筛选是「上次看到哪」的上下文，换链接进来不该沿用）。
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  // 首帧捕获地址上的标签；之后地址跟随 tag 变化（下面的 effect），不回读
  const [urlTag] = useState<string | null>(() => (isH5 ? null : searchParams.get("tag")));
  // 驾驶舱「全部书架 →」（C-23）要落到全部书：带上这个 state 就忽略会话缓存里的筛选
  const resetRequested = (location.state as { resetFilters?: boolean } | null)?.resetFilters === true;
  // /read 在 AppShell 之外：进阅读器时本页整体卸载，靠会话缓存把列表与滚动位置带回来
  const cached = useMemo(() => {
    const snapshot = readShelfCache();
    return snapshot && (urlTag || resetRequested) ? null : snapshot;
  }, [urlTag, resetRequested]);
  const [sort, setSort] = useState<SortValue>(() =>
    SORTS.some((option) => option.value === cached?.sort) ? (cached?.sort as SortValue) : "updated",
  );
  const [authorId, setAuthorId] = useState<number | null>(cached?.authorId ?? null);
  const [tag, setTag] = useState<string | null>(urlTag ?? cached?.tag ?? null);
  const [readStatus, setReadStatus] = useState<Exclude<ReadStatus, ""> | null>(cached?.readStatus ?? null);
  /** 评分筛选 1–5；0 星 = 未评，后端不提供「只有未评」，所以这里没有「未评」档 */
  const [rating, setRating] = useState<number | null>(cached?.rating ?? null);
  /** 书单筛选（扁平收藏夹，一本书可属多个书单） */
  const [collection, setCollection] = useState<number | null>(cached?.collection ?? null);
  /** 分类筛选（0015）：大分类 + 联动的小分类 */
  const [category, setCategory] = useState<CategoryValue>({
    l1: cached?.categoryL1 ?? null,
    l2: cached?.categoryL2 ?? null,
  });
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
  // 清除筛选后焦点回到排序下拉（C-29）；404 回书架后焦点交给页标题（C-18）
  const sortRef = useRef<HTMLSelectElement | null>(null);
  const titleRef = useRef<HTMLHeadingElement | null>(null);
  const navigate = useNavigate();

  // 筛选项的候选：作者与标签都是 PC 专属，H5 不发这两个请求
  const authorOptions = useAuthors({ sort: "books", size: 50 }, { enabled: !isH5 });
  const tagOptions = useTags({ enabled: !isH5 });

  const pageSize = isH5 ? H5_PAGE_SIZE : PAGE_SIZE;
  // 缓存命中就先用它渲染；够新（60 秒内）时连后台刷新都不做，返回瞬间不再发第 1 页
  const [now] = useState(() => Date.now());
  const restore = useMemo<BookFeedRestore | undefined>(() => {
    if (!cached || cached.size !== pageSize || cached.items.length === 0) return undefined;
    return {
      items: cached.items,
      total: cached.total,
      page: cached.page,
      refresh: now - cached.savedAt > SHELF_REFRESH_MIN_AGE_MS,
    };
  }, [cached, pageSize, now]);
  const feed = useBookFeed({
    sort,
    authorId: authorId ?? undefined,
    tag: tag ?? undefined,
    readStatus: readStatus ?? undefined,
    rating: rating ?? undefined,
    collection: collection ?? undefined,
    categoryL1: category.l1 ?? undefined,
    categoryL2: category.l2 ?? undefined,
    size: pageSize,
    mode: isH5 ? "infinite" : "paged",
    restore,
  });

  const { items, total, loading, error, reload, loadMore, hasMore } = feed;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const filtering =
    authorId !== null ||
    tag !== null ||
    readStatus !== null ||
    rating !== null ||
    collection !== null ||
    category.l1 !== null;
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
      rating,
      collection,
      categoryL1: category.l1,
      categoryL2: category.l2,
      size: pageSize,
      mode: isH5 ? "infinite" : "paged",
      items,
      total,
      page: feed.page,
      scrollTop: Math.round(scrollYRef.current),
      lastBookId: lastBookRef.current,
    });
  }, [sort, authorId, tag, readStatus, rating, collection, category.l1, category.l2, pageSize, isH5, items, total, feed.page]);

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
      rating,
      collection,
      categoryL1: category.l1,
      categoryL2: category.l2,
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

  function scrollToTopOfList() {
    // 换了排序 / 筛选 / 页码都回到列表顶部；老浏览器与测试环境可能没有 scrollTo
    try {
      window.scrollTo({ top: 0 });
    } catch {
      /* 不影响结果 */
    }
  }

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

  // 标签变化写回 ?tag=：复制出去的书架链接能复现同一筛选；replace 不往后退栈里塞历史
  useEffect(() => {
    setSearchParams(
      (prev) => {
        if ((prev.get("tag") ?? null) === tag) return prev;
        const next = new URLSearchParams(prev);
        if (tag) next.set("tag", tag);
        else next.delete("tag");
        return next;
      },
      { replace: true },
    );
  }, [tag, setSearchParams]);

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

  /** 一次清空七项筛选（C-29），焦点回排序下拉；只清筛选不动排序。 */
  function resetFilters() {
    setAuthorId(null);
    setTag(null);
    setReadStatus(null);
    setRating(null);
    setCollection(null);
    setCategory({ l1: null, l2: null });
    sortRef.current?.focus();
    scrollToTopOfList();
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
        authorId={authorId}
        tag={tag}
        collection={collection}
        selecting={selecting}
        selectedCount={selected.size}
        sheetOpen={sheetOpen}
        readStatus={readStatus}
        onToggleSelecting={toggleSelecting}
        onOpenBatch={() => setBatchOpen(true)}
        onOpenSheet={() => setSheetOpen(true)}
      />

      {isH5 ? null : <ResumeCard />}

      {isH5 ? null : (
        <ShelfToolbar
          sortRef={sortRef}
          sort={sort}
          onChooseSort={chooseSortOnPc}
          readStatus={readStatus}
          onChooseReadStatus={chooseReadStatus}
          rating={rating}
          onRatingChange={setRating}
          authorId={authorId}
          onAuthorChange={setAuthorId}
          authors={authorOptions.data?.items ?? []}
          collection={collection}
          onCollectionChange={setCollection}
          category={category}
          onCategoryChange={setCategory}
          tag={tag}
          onTagChange={setTag}
          tags={tagOptions.data?.items ?? []}
          filtering={filtering}
          onResetFilters={resetFilters}
        />
      )}

      {isH5 ? null : <FilterHints authors={authorOptions} tags={tagOptions} />}

      <ShelfList
        isH5={isH5}
        loading={loading}
        error={error}
        items={items}
        total={total}
        filtering={filtering}
        mode={mode}
        statusOverrides={statusOverrides}
        selecting={selecting}
        selected={selected}
        feed={feed}
        pages={pages}
        canObserve={canObserve}
        sentinelRef={sentinelRef}
        onRetry={reload}
        onResetFilters={resetFilters}
        onCycle={(book) => void cycleStatus(book)}
        onSelect={toggleSelected}
        onOpen={rememberBook}
        onPage={changePage}
      />

      {isH5 && sheetOpen ? (
        <ShelfSortSheet
          sort={sort}
          readStatus={readStatus}
          onChooseSort={chooseSort}
          onChooseReadStatus={chooseReadStatus}
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
