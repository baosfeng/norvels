import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { api } from "../../api/client";
import { useBook, useBookmarks, useChapter, useRecordReadingSession, useSaveProgress } from "../../api/hooks";
import type { Bookmark, ChapterBrief } from "../../api/types";
import { ChevronLeftIcon } from "../../components/Icons";
import { ErrorPanel, LoadingPanel, EmptyPanel } from "../../components/StatePanel";
import { TocDrawer } from "./TocDrawer";
import { useTheme } from "../../lib/theme";
import { useIsH5 } from "../../lib/tier";
import { formatWords } from "../../lib/format";
import { useReaderControls } from "../../lib/useReaderControls";
import { useSwipePaging } from "../../lib/useSwipePaging";
import { prefersReducedMotion } from "../../lib/theme";
import { bookmarkLabel, findBookmarkAt, isValidPosition } from "../../lib/bookmark";
import { searchParagraphs, splitParagraphs, type ReaderHit } from "../../lib/readerSearch";
import { showToast } from "../../lib/toastStore";
import { BookmarksDrawer } from "./BookmarksDrawer";
import { ReaderBottomBar } from "./ReaderBottomBar";
import { ReaderSearchPanel } from "./ReaderSearchPanel";
import { ReaderSettingsPanel } from "./ReaderSettingsPanel";
import { ReaderAutoScrollBar } from "./ReaderAutoScrollBar";
import { ReaderHelpPanel } from "./ReaderHelpPanel";
import { readReaderSettings, readerStyleVars, writeReaderSettings, type ReaderSettings } from "./readerSettings";
import { tradToSimp } from "../../lib/trad2simp";
import { pickStoredParaIndex, useReaderProgress } from "./readerProgress";
import { useReadingSessionReporter } from "../../lib/readingTimer";

/** 章内位置的上报防抖：滚动停下 1 秒才发一次；切章 / 隐藏 / 卸载不等防抖。 */
const PROGRESS_DEBOUNCE_MS = 1000;
import { createAutoScroll, isAtBottom } from "../../lib/autoScroll";

/**
 * 阅读器：产品核心，两端能力等价。
 * PC：设置面板浮在右上、目录从右侧抽屉滑出；H5：设置面板变宽、目录全屏抽屉。
 * 三主题 / 字号行距 / 键盘翻页 / 进度恢复在两端都可用。
 */
export function ReaderPage() {
  const params = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const isH5 = useIsH5();
  const bookId = Number(params.bookId);
  const chapterId = Number(params.chapterId);

  const book = useBook(Number.isFinite(bookId) ? bookId : null);
  const chapter = useChapter(Number.isFinite(bookId) ? bookId : null, Number.isFinite(chapterId) ? chapterId : null);
  const saveProgress = useSaveProgress();
  // 阅读时长：可见时计时，切章/隐藏/卸载落一次会话（单次 ≥60s 才上报）。
  const recordSession = useRecordReadingSession();
  useReadingSessionReporter({ bookId, chapterId, report: recordSession });
  // 书签列表随阅读器一起取：抽屉打开即可见，同段已有书签时也不重复发请求
  const bookmarks = useBookmarks(Number.isFinite(bookId) ? bookId : null);
  // 上次读到哪一段：进章节后据此把正文滚回原处（与书签跳转共用段落 anchor）
  const progressList = useReaderProgress(bookId);

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [bookmarksOpen, setBookmarksOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  // 当前命中：query 与本次搜索一致时 position 才有效，换关键词自动回到第 1 处
  const [hitState, setHitState] = useState({ query: "", position: 0 });
  // 自动滚动：读者手动暂停 + 窗口是否在前台（失焦 / 页面隐藏都算不活跃）
  const [autoPaused, setAutoPaused] = useState(false);
  const [windowActive, setWindowActive] = useState(true);
  const [deletingBookmark, setDeletingBookmark] = useState<number | null>(null);
  const [addingBookmark, setAddingBookmark] = useState(false);
  // 段落锚点 = 视口里最靠上的段落（书签记在这里）；没有 IntersectionObserver 时恒为第 0 段
  const [anchor, setAnchor] = useState({ chapterId: 0, para: 0 });
  // 位置上报的去重与「立刻补发」时机（切章 / 隐藏 / 卸载）都要看最新锚点，所以镜像进 ref
  const lastSent = useRef({ chapterId: 0, para: -1 });
  const anchorRef = useRef(anchor);
  useEffect(() => {
    anchorRef.current = anchor;
  });
  const [pendingJump, setPendingJump] = useState<{ chapterId: number; paraIndex: number } | null>(null);
  // 排版设置集中在一个对象里：面板改一项只 patch 一项，落盘统一写 norvels.reader.*
  const [settings, setSettings] = useState<ReaderSettings>(() => readReaderSettings());
  const updateSettings = useCallback((patch: Partial<ReaderSettings>) => {
    setSettings((prev) => ({ ...prev, ...patch }));
  }, []);
  const { theme, setTheme, themes } = useTheme();

  // 控制层：H5 默认收起、单击正文唤出、3 秒无操作收起；设置/目录打开时暂停收起
  const controls = useReaderControls({
    isH5,
    paused: settingsOpen || drawerOpen || bookmarksOpen || searchOpen || helpOpen,
  });

  // 全屏：只有浏览器真的支持才给按钮（iOS Safari 没有 Fullscreen API，静默降级为沉浸式阅读）
  const canFullscreen = typeof document !== "undefined" && document.fullscreenEnabled === true;
  const [fullscreen, setFullscreen] = useState(false);
  useEffect(() => {
    const onChange = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, []);

  async function toggleFullscreen() {
    if (!canFullscreen) return;
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await document.documentElement.requestFullscreen();
    } catch {
      // 被拒（策略限制 / 不支持）：静默降级为沉浸式——收起控制层，正文照常可读
      if (controls.visible) controls.toggle();
    }
  }

  const chapters = useMemo<ChapterBrief[]>(
    () => (book.data?.volumes ?? []).flatMap((v) => v.chapters).sort((a, b) => a.idx - b.idx),
    [book.data],
  );
  const index = chapters.findIndex((c) => c.id === chapterId);
  const prev = index > 0 ? chapters[index - 1] : null;
  const next = index >= 0 && index < chapters.length - 1 ? chapters[index + 1] : null;

  // 正文分段：段落下标即后端 paraIndex，书签与跳转都按它对齐（空行保留占位，不挪下标）
  const content = chapter.data?.content ?? "";
  const paragraphs = useMemo(() => splitParagraphs(content), [content]);
  // 繁→简是逐字替换、长度不变，所以段落下标与书签 / 进度仍然对齐
  const viewParagraphs = useMemo(
    () => (settings.script ? paragraphs.map(tradToSimp) : paragraphs),
    [paragraphs, settings.script],
  );
  const search = useMemo(() => searchParagraphs(viewParagraphs, searchQuery), [viewParagraphs, searchQuery]);
  const hitCount = search.hits.length;
  const currentHit = hitState.query === search.query && hitCount > 0 ? Math.min(hitState.position, hitCount - 1) : 0;
  const hitsByPara = useMemo(() => {
    const map = new Map<number, ReaderHit[]>();
    for (const hit of search.hits) {
      const bucket = map.get(hit.paraIndex);
      if (bucket) bucket.push(hit);
      else map.set(hit.paraIndex, [hit]);
    }
    return map;
  }, [search]);
  const anchorPara = anchor.chapterId === (chapter.data?.id ?? 0) ? anchor.para : 0;
  // 上次读到的段落下标：只有「本书 + 本章」对得上才生效
  const storedParaIndex = useMemo(
    () => pickStoredParaIndex(progressList.data?.items ?? [], bookId, chapterId),
    [progressList.data, bookId, chapterId],
  );

  // 排版设置持久化（隐私模式下写入失败不影响当前阅读）
  useEffect(() => {
    writeReaderSettings(settings);
  }, [settings]);

  // 失焦 / 页面隐藏时暂停自动滚动：切到别的窗口不该继续偷跑进度
  useEffect(() => {
    const onVisibility = () => setWindowActive(document.visibilityState === "visible");
    const onBlur = () => setWindowActive(false);
    const onFocus = () => setWindowActive(true);
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("blur", onBlur);
    window.addEventListener("focus", onFocus);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("blur", onBlur);
      window.removeEventListener("focus", onFocus);
    };
  }, []);

  // 全书进度（章节维度）与章内位置（段落维度）分开记：percent 跟章节下标走，paraIndex 跟锚点走
  const percent = chapters.length > 0 && index >= 0 ? (index + 1) / chapters.length : 0;
  const sendProgress = useCallback(
    (targetChapter: number, paraIndex: number) => {
      if (!Number.isFinite(bookId)) return;
      const at = chapters.findIndex((chapter) => chapter.id === targetChapter);
      if (at < 0) return;
      lastSent.current = { chapterId: targetChapter, para: paraIndex };
      // 失败不打断阅读：本地位置还在，下一次滚动/切章会再试
      void saveProgress({ bookId, chapterId: targetChapter, percent: (at + 1) / chapters.length, paraIndex });
    },
    [bookId, chapters, saveProgress],
  );

  // 打开 / 切章：立刻记一次（含恢复出来的 paraIndex），首屏位置也落库
  useEffect(() => {
    if (!Number.isFinite(bookId) || !Number.isFinite(chapterId)) return;
    if (chapters.length === 0 || index < 0) return;
    if (chapter.data?.id !== chapterId) return;
    if (lastSent.current.chapterId === chapterId && lastSent.current.para === storedParaIndex) return;
    sendProgress(chapterId, storedParaIndex);
  }, [bookId, chapterId, chapter.data?.id, chapters.length, index, storedParaIndex, sendProgress]);

  // 滚动停下 1s 才上报 paraIndex：长章里每滚一下发一次，写放大到没法看
  useEffect(() => {
    if (anchor.chapterId !== chapterId || !Number.isFinite(bookId)) return;
    const para = anchor.para;
    const timer = window.setTimeout(() => {
      if (lastSent.current.chapterId === chapterId && lastSent.current.para === para) return;
      sendProgress(chapterId, para);
    }, PROGRESS_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [anchor, chapterId, bookId, sendProgress]);

  // 切章 / 卸载：不等防抖，立刻把最后一次位置补发
  useEffect(() => {
    const watching = chapterId;
    return () => {
      const latest = anchorRef.current;
      if (latest.chapterId !== watching) return;
      if (lastSent.current.chapterId === latest.chapterId && lastSent.current.para === latest.para) return;
      sendProgress(latest.chapterId, latest.para);
    };
  }, [chapterId, sendProgress]);

  // 页面隐藏（手机切后台 / 锁屏 / 浏览器关标签）：不指望卸载时机，立刻补发
  useEffect(() => {
    const onHide = () => {
      if (document.visibilityState !== "hidden") return;
      const latest = anchorRef.current;
      if (!latest.chapterId) return;
      if (lastSent.current.chapterId === latest.chapterId && lastSent.current.para === latest.para) return;
      sendProgress(latest.chapterId, latest.para);
    };
    document.addEventListener("visibilitychange", onHide);
    return () => document.removeEventListener("visibilitychange", onHide);
  }, [sendProgress]);

  /** 回书架：直接落到 /shelf（带 restore），书架按会话缓存恢复滚动位置与已加载列表 */
  const goShelf = useCallback(() => {
    navigate("/shelf", { state: { restore: true } });
  }, [navigate]);

  /** 返回：优先浏览器历史（从哪来回哪去），没有历史时才回书架 */
  const goBack = useCallback(() => {
    if (location.key !== "default") navigate(-1);
    else navigate("/shelf", { state: { restore: true } });
  }, [location.key, navigate]);

  const goChapter = useCallback(
    (target: ChapterBrief | null) => {
      if (!target) return;
      navigate(`/read/${bookId}/${target.id}`);
      window.scrollTo({ top: 0 });
    },
    [bookId, navigate],
  );

  const closeSearch = useCallback(() => {
    setSearchOpen(false);
    setSearchQuery("");
    setHitState({ query: "", position: 0 });
  }, []);

  /** 目录与书签是同级的两个抽屉，互斥打开（设置面板也一样） */
  const openDirectory = useCallback(() => {
    setBookmarksOpen(false);
    setSettingsOpen(false);
    setDrawerOpen(true);
  }, []);

  const openBookmarks = useCallback(() => {
    setDrawerOpen(false);
    setSettingsOpen(false);
    setBookmarksOpen(true);
  }, []);

  function stepHit(delta: number) {
    if (hitCount === 0) return;
    const base = hitState.query === search.query ? Math.min(hitState.position, hitCount - 1) : 0;
    setHitState({ query: search.query, position: (base + delta + hitCount) % hitCount });
  }

  /** 加书签：记在当前读到的段落；同段已有书签就不发请求（后端同样按位置幂等）。 */
  async function addBookmark() {
    if (!chapter.data) return;
    const paraIndex = anchorPara;
    if (!isValidPosition(chapterId, paraIndex)) {
      showToast("这个位置记不下来，换个位置再试。");
      return;
    }
    if (findBookmarkAt(bookmarks.data?.bookmarks ?? [], chapterId, paraIndex)) {
      showToast("这个位置已经有书签了。");
      return;
    }
    setAddingBookmark(true);
    try {
      await api.createBookmark(bookId, {
        chapterId,
        paraIndex,
        label: bookmarkLabel(viewParagraphs[paraIndex] ?? ""),
      });
      bookmarks.reload();
      showToast("已加入书签。");
    } catch {
      showToast("书签没能保存，稍后再试。");
    } finally {
      setAddingBookmark(false);
    }
  }

  async function removeBookmark(id: number) {
    if (!Number.isInteger(id) || id <= 0) return;
    setDeletingBookmark(id);
    try {
      await api.deleteBookmark(id);
      bookmarks.reload();
    } catch {
      showToast("书签没能删除，稍后再试。");
    } finally {
      setDeletingBookmark(null);
    }
  }

  function jumpToBookmark(bookmark: Bookmark) {
    setBookmarksOpen(false);
    setPendingJump({ chapterId: bookmark.chapterId, paraIndex: bookmark.paraIndex });
    if (bookmark.chapterId !== chapterId) navigate(`/read/${bookId}/${bookmark.chapterId}`);
  }

  /** 一屏 = 0.85 个视口高：翻页时保留上一屏的尾巴，读者不会跟丢行。 */
  const scrollByScreen = useCallback((direction: 1 | -1) => {
    window.scrollBy({ top: window.innerHeight * 0.85 * direction, behavior: "smooth" });
  }, []);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.key === "Escape") {
        setDrawerOpen(false);
        setSettingsOpen(false);
        setBookmarksOpen(false);
        setHelpOpen(false);
        closeSearch();
        return;
      }
      // 输入框里打字不接管（否则「?」这类字符打不进搜索框）
      const target = event.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) return;
      // 「?」= Shift+/ ：打开 / 关闭快捷键帮助
      if (event.key === "?" || (event.key === "/" && event.shiftKey)) {
        event.preventDefault();
        setHelpOpen((open) => !open);
        return;
      }
      // 帮助面板打开时只留 Esc：别在读者看说明时把正文翻走
      if (helpOpen) return;
      // 搜索面板打开时方向键/空格留给输入框，不翻章也不滚页
      if (searchOpen) return;
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        goChapter(prev);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        goChapter(next);
      } else if (event.key === " " || event.key === "PageDown") {
        event.preventDefault();
        scrollByScreen(event.shiftKey ? -1 : 1);
      } else if (event.key === "PageUp") {
        event.preventDefault();
        scrollByScreen(-1);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [goChapter, prev, next, searchOpen, helpOpen, closeSearch, scrollByScreen]);

  // 段落锚点：视口内最靠上的段落即「当前读到的地方」（没有 IntersectionObserver 时保持第 0 段）
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined" || paragraphs.length === 0) return;
    const nodes = Array.from(document.querySelectorAll<HTMLElement>("[data-para]"));
    if (nodes.length === 0) return;
    const visible = new Set<number>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const index2 = Number((entry.target as HTMLElement).dataset.para ?? "0");
          if (entry.isIntersecting) visible.add(index2);
          else visible.delete(index2);
        }
        if (visible.size > 0) setAnchor({ chapterId: chapter.data?.id ?? 0, para: Math.min(...visible) });
      },
      { rootMargin: "-10% 0px -60% 0px" },
    );
    nodes.forEach((node) => observer.observe(node));
    return () => observer.disconnect();
  }, [paragraphs, chapter.data?.id]);

  // 搜索翻页：把当前命中滚到视口中间（测试环境没有 scrollIntoView 就跳过）
  useEffect(() => {
    if (!searchOpen || hitCount === 0) return;
    const node = document.querySelector<HTMLElement>(`[data-hit="${currentHit}"]`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "center" });
  }, [searchOpen, currentHit, hitCount, chapter.data?.id]);

  // 书签跳转：章载入后滚到那一行（同章跳转靠 pendingJump 换引用触发）
  useEffect(() => {
    if (!pendingJump || chapter.data?.id !== pendingJump.chapterId) return;
    const node = document.getElementById(`para-${pendingJump.paraIndex}`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
  }, [pendingJump, chapter.data?.id]);

  // 章内位置恢复：进入章节时按进度里的 paraIndex 滚回原段。
  // 与书签跳转（pendingJump）不打架——书签跳转自己会滚到目标段，这里的恢复让位。
  const restoredChapter = useRef(0);
  useEffect(() => {
    if (!chapter.data || chapter.data.id !== chapterId) return;
    if (restoredChapter.current === chapterId) return;
    // 进度还在路上时先别定稿：否则会把「章首」当成要恢复的位置，等它回来又不再重试
    if (progressList.loading) return;
    restoredChapter.current = chapterId;
    if (pendingJump && pendingJump.chapterId === chapterId) return;
    if (storedParaIndex <= 0) return;
    // 先把锚点对齐：否则首帧会把「第 0 段」当成位置写回去
    setAnchor({ chapterId, para: storedParaIndex });
    const node = document.getElementById(`para-${storedParaIndex}`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
  }, [chapter.data, chapterId, pendingJump, storedParaIndex, progressList.loading]);

  // 自动滚动：开关开着、没被手动暂停、没有面板打开、窗口在前台时才真的滚
  const overlayOpen = settingsOpen || drawerOpen || bookmarksOpen || searchOpen || helpOpen;
  const autoRunning = settings.autoScroll && !autoPaused && !overlayOpen && windowActive;
  // 到底时的动作走 ref：滚动引擎只该在速度 / 运行状态变化时重建，不该因为换了一章就重来
  const autoBottomRef = useRef<() => void>(() => {});
  useEffect(() => {
    autoBottomRef.current = () => {
      // 到底自动下一章默认关：开着且确有下一章才翻，否则直接停住，别把读者带跑
      if (settings.autoScrollNext && next) goChapter(next);
      else updateSettings({ autoScroll: false });
    };
  });
  useEffect(() => {
    if (!autoRunning) return;
    const controller = createAutoScroll(
      {
        scrollBy: (deltaY) => window.scrollBy({ top: deltaY }),
        atBottom: () =>
          isAtBottom({
            scrollTop: window.scrollY || document.documentElement.scrollTop || 0,
            clientHeight: window.innerHeight || 0,
            scrollHeight: document.documentElement.scrollHeight || 0,
          }),
        onBottom: () => autoBottomRef.current(),
      },
      settings.autoScrollSpeed,
    );
    controller.start();
    return () => controller.stop();
  }, [autoRunning, settings.autoScrollSpeed]);

  // 章节切换淡入：用 key 让正文在换章时重新挂载，CSS 动画只播一次（不在 effect 里 setState）
  const reducedMotion = prefersReducedMotion();
  const articleKey = reducedMotion ? "static" : `chapter-${chapterId}`;

  // H5 左右滑动翻页：只在窄屏启用（PC 上鼠标拖拽不是翻页手势）
  const swipeHandlers = useMemo(
    () => ({ enabled: isH5, onPrev: () => goChapter(prev), onNext: () => goChapter(next) }),
    [isH5, goChapter, prev, next],
  );
  const swipeRef = useSwipePaging(swipeHandlers);

  const bookTitle = book.data?.title ?? chapter.data?.bookTitle ?? "";
  // 当前章字数：正文响应里就有，不需要额外请求
  const chapterWords = chapter.data ? formatWords(chapter.data.wordCount) : "";

  return (
    <div
      className={`reader-shell${isH5 ? " is-h5" : ""}${controls.visible ? " is-controls" : ""}`}
      data-testid="view:reader"
      data-od-id="reader"
      style={readerStyleVars(settings) as CSSProperties}
    >
      <header
        className="reader-top"
        data-testid="reader-top"
        data-od-id="reader-top"
        aria-hidden={isH5 && !controls.visible}
      >
        <button className="icon-btn" type="button" aria-label="返回目录" onClick={goBack}>
          <ChevronLeftIcon />
        </button>
        <div className="reader-top-title">
          <strong>{bookTitle || "正在打开"}</strong>
          <span data-testid="reader-top-progress">
            {chapters.length > 0 && index >= 0
              ? `第 ${index + 1} / ${chapters.length} 章${chapterWords ? ` · ${chapterWords}` : ""}`
              : "正在读取目录"}
          </span>
        </div>
        <div className="reader-top-actions">
          <button className="btn" type="button" aria-label="返回书架" onClick={goShelf}>
            书架
          </button>
          <button
            className="btn"
            type="button"
            onClick={openBookmarks}
            aria-expanded={bookmarksOpen}
            aria-label="打开书签"
          >
            书签
          </button>
          <button
            className="btn"
            type="button"
            aria-expanded={searchOpen}
            aria-label="搜索本章"
            onClick={() => (searchOpen ? closeSearch() : setSearchOpen(true))}
          >
            搜索
          </button>
          <button className="btn" type="button" onClick={openDirectory} aria-expanded={drawerOpen}>
            目录
          </button>
          <button
            className="btn"
            type="button"
            aria-label="快捷键帮助"
            aria-expanded={helpOpen}
            onClick={() => setHelpOpen((open) => !open)}
          >
            ?
          </button>
          <button className="btn" type="button" onClick={() => setSettingsOpen((v) => !v)} aria-expanded={settingsOpen}>
            设置
          </button>
          {isH5 && canFullscreen ? (
            <button
              className="icon-btn"
              type="button"
              aria-label={fullscreen ? "退出全屏" : "全屏阅读"}
              aria-pressed={fullscreen}
              onClick={toggleFullscreen}
            >
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
                <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
              </svg>
            </button>
          ) : null}
        </div>
      </header>

      <ReaderHelpPanel open={helpOpen} onClose={() => setHelpOpen(false)} />

      <ReaderSearchPanel
        open={searchOpen}
        query={searchQuery}
        hits={hitCount}
        total={search.total}
        truncated={search.truncated}
        current={currentHit}
        onQueryChange={setSearchQuery}
        onPrev={() => stepHit(-1)}
        onNext={() => stepHit(1)}
        onClose={closeSearch}
      />

      <div className="reader-main" ref={swipeRef} data-testid="reader-swipe-area" {...controls.surfaceProps}>
        {chapter.loading ? (
          <div className="reader-loading-layer">
            <LoadingPanel title="正在载入正文" description="单章 3000–6000 字，首次打开需要整理分段。" testId="reader-loading" />
          </div>
        ) : null}

        {chapter.error ? (
          <ErrorPanel error={chapter.error} title="正文没能打开" onRetry={chapter.reload} testId="reader-error" />
        ) : chapter.data ? (
          <article key={articleKey} className={`reader-content reader-article${reducedMotion ? "" : " is-entering"}`}>
            <h2>{chapter.data.title}</h2>
            <div>
              {viewParagraphs.map((para, i) =>
                para ? (
                  <p key={i} id={`para-${i}`} data-para={i}>
                    {paragraphNodes(para, hitsByPara.get(i) ?? [], currentHit, searchOpen)}
                  </p>
                ) : null,
              )}
            </div>
            <nav className="reader-pager" aria-label="章节切换">
              <button className="btn" type="button" disabled={!prev} onClick={() => goChapter(prev)}>
                ← 上一章
              </button>
              <Link className="btn" to={bookId ? `/book/${bookId}` : "/shelf"}>
                返回目录
              </Link>
              <button className="btn" type="button" disabled={!next} onClick={() => goChapter(next)}>
                下一章 →
              </button>
            </nav>
            <div className="reader-progress" data-od-id="reader-progress">
              <div className="reader-progress-row">
                <span>
                  第 <strong className="num">{index + 1}</strong> / {chapters.length} 章
                  <span className="word" data-testid="reader-word">
                    {" "}
                    · {formatWords(chapter.data.wordCount)}
                  </span>
                </span>
                <span>
                  全书进度 <strong className="num">{Math.round(percent * 100)}</strong>%
                </span>
              </div>
              <div className="progress">
                <span style={{ width: `${Math.round(percent * 100)}%` }} />
              </div>
            </div>
          </article>
        ) : (
          <EmptyPanel title="还没有正文" description="这一章的内容还没入库，稍后再来看看。" testId="reader-empty" />
        )}
      </div>

      {isH5 ? (
        <ReaderBottomBar
          hidden={!controls.visible}
          prev={prev}
          next={next}
          onPrev={() => goChapter(prev)}
          onNext={() => goChapter(next)}
          onOpenDrawer={openDirectory}
          onGoShelf={goShelf}
          onToggleSettings={() => setSettingsOpen((v) => !v)}
          onOpenBookmarks={openBookmarks}
          onToggleSearch={() => (searchOpen ? closeSearch() : setSearchOpen(true))}
          settingsOpen={settingsOpen}
          drawerOpen={drawerOpen}
          bookmarksOpen={bookmarksOpen}
          searchOpen={searchOpen}
        />
      ) : null}

      {settings.autoScroll ? (
        <ReaderAutoScrollBar
          running={autoRunning}
          paused={autoPaused || !windowActive || overlayOpen}
          speed={settings.autoScrollSpeed}
          onTogglePause={() => setAutoPaused((paused) => !paused)}
          onExit={() => {
            setAutoPaused(false);
            updateSettings({ autoScroll: false });
          }}
        />
      ) : null}

      <ReaderSettingsPanel
        open={settingsOpen}
        settings={settings}
        onChange={updateSettings}
        theme={theme}
        themes={themes}
        setTheme={setTheme}
        onClose={() => setSettingsOpen(false)}
      />

      <button
        className={`overlay${drawerOpen || bookmarksOpen ? " is-open" : ""}`}
        type="button"
        aria-label={bookmarksOpen ? "关闭书签" : "关闭目录"}
        tabIndex={drawerOpen || bookmarksOpen ? 0 : -1}
        onClick={() => {
          setDrawerOpen(false);
          setBookmarksOpen(false);
        }}
      />

      <BookmarksDrawer
        open={bookmarksOpen}
        bookmarks={bookmarks}
        currentChapterId={Number.isFinite(chapterId) ? chapterId : 0}
        deletingId={deletingBookmark}
        adding={addingBookmark}
        canAdd={Boolean(chapter.data)}
        onClose={() => setBookmarksOpen(false)}
        onAdd={() => void addBookmark()}
        onJump={jumpToBookmark}
        onDelete={(id) => void removeBookmark(id)}
      />

      <TocDrawer
        open={drawerOpen}
        loading={book.loading}
        volumeCount={book.data?.volumes.length ?? 0}
        bookId={bookId}
        chapters={chapters}
        currentChapterId={chapterId}
        onClose={() => setDrawerOpen(false)}
      />
    </div>
  );
}

/**
 * 段落渲染：搜索态把命中包成 <mark data-hit>（当前那一处加 is-current），
 * 非搜索态原样返回字符串——不加包裹节点，正文文本匹配保持原样。
 */
function paragraphNodes(para: string, hits: ReaderHit[], currentHit: number, searching: boolean): ReactNode {
  if (!searching || hits.length === 0) return para;
  const nodes: ReactNode[] = [];
  let from = 0;
  for (const hit of hits) {
    if (hit.start > from) nodes.push(<span key={`t${hit.index}`}>{para.slice(from, hit.start)}</span>);
    nodes.push(
      <mark key={`m${hit.index}`} data-hit={hit.index} className={hit.index === currentHit ? "is-current" : undefined}>
        {para.slice(hit.start, hit.end)}
      </mark>,
    );
    from = hit.end;
  }
  if (from < para.length) nodes.push(<span key="tail">{para.slice(from)}</span>);
  return nodes;
}

