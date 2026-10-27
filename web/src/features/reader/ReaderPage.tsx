import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { useBook, useBookmarks, useChapter, useRecordReadingSession, useSaveProgress } from "../../api/hooks";
import type { ChapterBrief } from "../../api/types";
import { useTheme } from "../../lib/theme";
import { useIsH5 } from "../../lib/tier";
import { formatWords } from "../../lib/format";
import { useReaderControls } from "../../lib/useReaderControls";
import { useSwipePaging } from "../../lib/useSwipePaging";
import { prefersReducedMotion } from "../../lib/theme";
import { splitParagraphs } from "../../lib/readerSearch";
import { showToast } from "../../lib/toastStore";
import { BookmarksDrawer } from "./BookmarksDrawer";
import { BookmarkConfirmDialog } from "./BookmarkConfirmDialog";
import { ReaderBottomBar } from "./ReaderBottomBar";
import { ReaderSearchPanel } from "./ReaderSearchPanel";
import { ReaderSettingsPanel } from "./ReaderSettingsPanel";
import { TocDrawer } from "./TocDrawer";
import { ReaderAutoScrollBar } from "./ReaderAutoScrollBar";
import { ReaderHelpPanel } from "./ReaderHelpPanel";
import { ReaderTopBar } from "./ReaderTopBar";
import { ReaderStateArea } from "./ReaderStateArea";
import { useReaderSearchState } from "./useReaderSearchState";
import { useReaderPanels } from "./useReaderPanels";
import { useReaderBookmarkActions } from "./useReaderBookmarkActions";
import { useReaderProgressSync } from "./useReaderProgressSync";
import { useChapterRetry } from "./useChapterRetry";
import { ReaderArticle } from "./ReaderArticle";
import { readReaderSettings, readerStyleVars, writeReaderSettings, type ReaderSettings } from "./readerSettings";
import { tradToSimp } from "../../lib/trad2simp";
import { pickStoredParaIndex, useReaderProgress } from "./readerProgress";
import { useReadingSessionReporter } from "../../lib/readingTimer";
import { createAutoScroll, isAtBottom } from "../../lib/autoScroll";

/** 章内位置的上报防抖：滚动停下 1 秒才发一次；切章 / 隐藏 / 卸载不等防抖。 */
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
  const recordSession = useRecordReadingSession();
  useReadingSessionReporter({ bookId, chapterId, report: recordSession });
  const bookmarks = useBookmarks(Number.isFinite(bookId) ? bookId : null);
  const progressList = useReaderProgress(bookId);

  const [autoPaused, setAutoPaused] = useState(false);
  const [windowActive, setWindowActive] = useState(true);
  const [hintDismissedFor, setHintDismissedFor] = useState(0);
  const [anchor, setAnchor] = useState({ chapterId: 0, para: 0 });
  const [pendingJump, setPendingJump] = useState<{ chapterId: number; paraIndex: number } | null>(null);
  const [settings, setSettings] = useState<ReaderSettings>(() => readReaderSettings());
  const { theme, setTheme, themes } = useTheme();

  const updateSettings = useCallback((patch: Partial<ReaderSettings>) => {
    setSettings((prev) => ({ ...prev, ...patch }));
  }, []);

  const canFullscreen = typeof document !== "undefined" && document.fullscreenEnabled === true;
  const [fullscreen, setFullscreen] = useState(false);
  useEffect(() => {
    const onChange = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, []);

  const chapters = useMemo<ChapterBrief[]>(
    () => (book.data?.volumes ?? []).flatMap((volume) => volume.chapters).sort((a, b) => a.idx - b.idx),
    [book.data],
  );
  const volumes = book.data?.volumes ?? [];
  const index = chapters.findIndex((c) => c.id === chapterId);
  const prev = index > 0 ? chapters[index - 1] : null;
  const next = index >= 0 && index < chapters.length - 1 ? chapters[index + 1] : null;

  const content = chapter.data?.content ?? "";
  const paragraphs = useMemo(() => splitParagraphs(content), [content]);
  const viewParagraphs = useMemo(
    () => (settings.script ? paragraphs.map(tradToSimp) : paragraphs),
    [paragraphs, settings.script],
  );
  const {
    query: searchQuery,
    setQuery: setSearchQuery,
    search,
    hitCount,
    currentHit,
    hitsByPara,
    stepHit,
    reset: resetSearch,
  } = useReaderSearchState(viewParagraphs);

  const {
    drawerOpen,
    settingsOpen,
    bookmarksOpen,
    searchOpen,
    helpOpen,
    setDrawerOpen,
    setSettingsOpen,
    setBookmarksOpen,
    setHelpOpen,
    openDirectory,
    openBookmarks,
    openSettings,
    openSearch,
    closeSearch,
    toggleHelp,
  } = useReaderPanels({ canSearch: Boolean(chapter.data), onResetSearch: resetSearch });

  const anchorPara = anchor.chapterId === (chapter.data?.id ?? 0) ? anchor.para : 0;

  const {
    addingBookmark,
    retryingBookmarks,
    deleteTarget,
    deletingBookmark,
    addBookmark,
    retryBookmarks,
    jumpToBookmark,
    requestDelete,
    cancelDelete,
    confirmDelete,
  } = useReaderBookmarkActions({
    bookId,
    chapterId,
    anchorPara,
    viewParagraphs,
    chapterTitle: chapter.data?.title ?? null,
    navigate,
    bookmarks,
    onOpenBookmarks: () => setBookmarksOpen(true),
    onCloseBookmarks: () => setBookmarksOpen(false),
    onPendingJump: setPendingJump,
  });

  const controls = useReaderControls({
    isH5,
    paused: settingsOpen || drawerOpen || bookmarksOpen || searchOpen || helpOpen || Boolean(deleteTarget),
  });
  const { retryingChapter, chapterFailures, retryChapter } = useChapterRetry({ chapter, chapterId });

  const storedParaIndex = useMemo(
    () => pickStoredParaIndex(progressList.data?.items ?? [], bookId, chapterId),
    [progressList.data, bookId, chapterId],
  );

  useEffect(() => {
    writeReaderSettings(settings);
  }, [settings]);

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

  const percent = chapters.length > 0 && index >= 0 ? (index + 1) / chapters.length : 0;
  useReaderProgressSync({
    bookId,
    chapterId,
    chapters,
    index,
    chapterDataId: chapter.data?.id,
    storedParaIndex,
    anchor,
    saveProgress,
  });

  const goShelf = useCallback(() => {
    navigate("/shelf", { state: { restore: true } });
  }, [navigate]);

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

  /** 用户主动翻章：契约 C-11/C-12 要求浮条提示「已翻到第 n 章」。
   *  自动滚动到底另有自己的提示，避免叠两条。 */
  const goChapterAnnounce = useCallback(
    (target: ChapterBrief | null) => {
      if (!target) return;
      showToast(`已翻到第 ${target.idx + 1} 章`);
      goChapter(target);
    },
    [goChapter],
  );

  /** 设置项联动：繁转简 / 自动滚动开启时给浮条提示（契约 C-50 / C-51）。 */
  const changeSettings = useCallback(
    (patch: Partial<ReaderSettings>) => {
      if (patch.script === true && !settings.script && paragraphs.some((para) => tradToSimp(para) !== para)) {
        showToast("已转换为简体，位置不变");
      }
      if (patch.autoScroll === true && !settings.autoScroll) showToast("已开始自动滚动");
      setSettings((prevSettings) => ({ ...prevSettings, ...patch }));
    },
    [settings.script, settings.autoScroll, paragraphs],
  );

  async function toggleFullscreen() {
    if (!canFullscreen) {
      controls.hide();
      showToast("当前浏览器不支持全屏，已收起控制栏");
      return;
    }
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else {
        await document.documentElement.requestFullscreen();
        showToast("已进入全屏阅读");
      }
    } catch {
      controls.hide();
      showToast("当前浏览器不支持全屏，已收起控制栏");
    }
  }


  const scrollByScreen = useCallback((direction: 1 | -1) => {
    window.scrollBy({ top: window.innerHeight * 0.85 * direction, behavior: "smooth" });
  }, []);

  const overlayOpen = settingsOpen || drawerOpen || bookmarksOpen || searchOpen || helpOpen || Boolean(deleteTarget);
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.key === "Escape") {
        if (deleteTarget) {
          cancelDelete();
          return;
        }
        setDrawerOpen(false);
        setSettingsOpen(false);
        setBookmarksOpen(false);
        setHelpOpen(false);
        closeSearch();
        return;
      }
      const target = event.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) return;
      if (event.key === "?" || (event.key === "/" && event.shiftKey)) {
        event.preventDefault();
        toggleHelp();
        return;
      }
      if (helpOpen) return;
      if (searchOpen) return;
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        goChapterAnnounce(prev);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        goChapterAnnounce(next);
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
  }, [goChapterAnnounce, prev, next, searchOpen, helpOpen, closeSearch, scrollByScreen, deleteTarget, toggleHelp, cancelDelete]);

  useEffect(() => {
    if (typeof IntersectionObserver === "undefined" || paragraphs.length === 0) return;
    const nodes = Array.from(document.querySelectorAll<HTMLElement>("[data-para]"));
    if (nodes.length === 0) return;
    const visible = new Set<number>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const paraIndex = Number((entry.target as HTMLElement).dataset.para ?? "0");
          if (entry.isIntersecting) visible.add(paraIndex);
          else visible.delete(paraIndex);
        }
        if (visible.size > 0) setAnchor({ chapterId: chapter.data?.id ?? 0, para: Math.min(...visible) });
      },
      { rootMargin: "-10% 0px -60% 0px" },
    );
    nodes.forEach((node) => observer.observe(node));
    return () => observer.disconnect();
  }, [paragraphs, chapter.data?.id]);

  useEffect(() => {
    if (!searchOpen || hitCount === 0) return;
    const node = document.querySelector<HTMLElement>(`[data-hit="${currentHit}"]`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "center" });
  }, [searchOpen, currentHit, hitCount, chapter.data?.id]);

  useEffect(() => {
    if (!pendingJump || chapter.data?.id !== pendingJump.chapterId) return;
    const node = document.getElementById(`para-${pendingJump.paraIndex}`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
    else {
      window.scrollTo({ top: 0 });
      showToast("这本书签的位置变了，先带你到这一章开头");
    }
  }, [pendingJump, chapter.data?.id]);

  const restoredChapter = useRef(0);
  useEffect(() => {
    if (!chapter.data || chapter.data.id !== chapterId) return;
    if (restoredChapter.current === chapterId) return;
    if (progressList.loading) return;
    restoredChapter.current = chapterId;
    if (pendingJump && pendingJump.chapterId === chapterId) return;
    if (storedParaIndex <= 0) return;
    window.setTimeout(() => {
      setAnchor({ chapterId, para: storedParaIndex });
      const node = document.getElementById(`para-${storedParaIndex}`);
      if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
    }, 0);
  }, [chapter.data, chapterId, pendingJump, storedParaIndex, progressList.loading]);

  const restoreHintVisible =
    Boolean(chapter.data) && storedParaIndex > 0 && !pendingJump && hintDismissedFor !== chapterId;

  const autoRunning = settings.autoScroll && !autoPaused && !overlayOpen && windowActive;
  const autoBottomRef = useRef<() => void>(() => {});
  useEffect(() => {
    autoBottomRef.current = () => {
      if (settings.autoScrollNext && next) {
        showToast(`已自动翻到第 ${index + 2} 章`);
        goChapter(next);
      } else {
        updateSettings({ autoScroll: false });
        showToast("已滚到章末，自动滚动暂停");
      }
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

  const reducedMotion = prefersReducedMotion();
  const articleKey = reducedMotion ? "static" : `chapter-${chapterId}`;

  const swipeHandlers = useMemo(
    () => ({ enabled: isH5, onPrev: () => goChapterAnnounce(prev), onNext: () => goChapterAnnounce(next) }),
    [isH5, goChapterAnnounce, prev, next],
  );
  const swipeRef = useSwipePaging(swipeHandlers);

  const bookTitle = book.data?.title ?? chapter.data?.bookTitle ?? "";
  const chapterWords = chapter.data ? formatWords(chapter.data.wordCount) : "";
  const progressText =
    chapters.length > 0 && index >= 0
      ? `第 ${index + 1} / ${chapters.length} 章${chapterWords ? ` · ${chapterWords}` : ""}`
      : "正在读取目录";

  function dismissHint() {
    setHintDismissedFor(chapterId);
    const surface = document.querySelector<HTMLElement>('[data-od-id="C-09"]');
    surface?.focus();
  }

  function exitAutoScroll() {
    setAutoPaused(false);
    updateSettings({ autoScroll: false });
    showToast("已关闭自动滚动");
    window.setTimeout(() => {
      document.querySelector<HTMLElement>(`[data-od-id="${isH5 ? "C-22" : "C-08"}"]`)?.focus();
    }, 0);
  }

  return (
    <div
      className={`reader-shell${isH5 ? " is-h5" : ""}${controls.visible ? " is-controls" : ""}`}
      data-testid="view:reader"
      data-od-id="reader"
      style={readerStyleVars(settings) as CSSProperties}
    >
      <ReaderTopBar
        isH5={isH5}
        hidden={isH5 && !controls.visible}
        bookTitle={bookTitle}
        progressText={progressText}
        fullscreen={fullscreen}
        canFullscreen={canFullscreen}
        searchEnabled={Boolean(chapter.data)}
        bookmarksOpen={bookmarksOpen}
        searchOpen={searchOpen}
        drawerOpen={drawerOpen}
        helpOpen={helpOpen}
        settingsOpen={settingsOpen}
        onBack={goBack}
        onToggleFullscreen={() => void toggleFullscreen()}
        onShelf={goShelf}
        onOpenBookmarks={openBookmarks}
        onToggleSearch={() => (searchOpen ? closeSearch() : openSearch())}
        onOpenDirectory={openDirectory}
        onToggleHelp={toggleHelp}
        onToggleSettings={() => (settingsOpen ? setSettingsOpen(false) : openSettings())}
      />

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

      <div
        className="reader-main"
        ref={swipeRef}
        data-testid="reader-swipe-area"
        data-od-id="C-09"
        tabIndex={0}
        role="region"
        aria-label="正文"
        aria-describedby="reader-gesture-hint"
        {...controls.surfaceProps}
      >
        <p id="reader-gesture-hint" className="sr-only">
          手机上左右滑动可翻章；也可以用底栏的上一章 / 下一章按钮，或键盘左右方向键
        </p>
        <div className="reader-swipe-layer" data-od-id="C-10">
          {chapter.loading || chapter.error || !chapter.data ? (
            <ReaderStateArea
              loading={chapter.loading}
              error={chapter.error}
              retrying={retryingChapter}
              retryFailures={chapterFailures}
              next={next}
              onRetry={retryChapter}
              onNext={() => goChapterAnnounce(next)}
            />
          ) : (
            <ReaderArticle
              title={chapter.data.title}
              paragraphs={viewParagraphs}
              hitsByPara={hitsByPara}
              currentHit={currentHit}
              searching={searchOpen}
              bookId={bookId}
              prev={prev}
              next={next}
              onPrev={() => goChapterAnnounce(prev)}
              onNext={() => goChapterAnnounce(next)}
              chapterNumber={index + 1}
              chapterCount={chapters.length}
              wordCount={chapter.data.wordCount}
              percent={percent}
              enterKey={articleKey}
              reducedMotion={reducedMotion}
              hintVisible={restoreHintVisible}
              onDismissHint={dismissHint}
            />
          )}
        </div>
      </div>

      {isH5 ? (
        <ReaderBottomBar
          hidden={!controls.visible}
          prev={prev}
          next={next}
          searchEnabled={Boolean(chapter.data)}
          onPrev={() => goChapterAnnounce(prev)}
          onNext={() => goChapterAnnounce(next)}
          onOpenDrawer={openDirectory}
          onGoShelf={goShelf}
          onOpenBookmarks={openBookmarks}
          onToggleSearch={() => (searchOpen ? closeSearch() : openSearch())}
          onToggleSettings={() => (settingsOpen ? setSettingsOpen(false) : openSettings())}
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
          onExit={exitAutoScroll}
        />
      ) : null}

      <ReaderSettingsPanel
        open={settingsOpen}
        isH5={isH5}
        autoScrollAvailable={Boolean(chapter.data)}
        settings={settings}
        onChange={changeSettings}
        theme={theme}
        themes={themes}
        setTheme={(value) => {
          setTheme(value);
          if (value === "system") showToast("已跟随系统外观");
        }}
        onClose={() => setSettingsOpen(false)}
      />

      <button
        className={`overlay${drawerOpen || bookmarksOpen || settingsOpen || Boolean(deleteTarget) ? " is-open" : ""}`}
        type="button"
        aria-label={bookmarksOpen ? "关闭书签" : "关闭当前面板"}
        data-od-id="C-24"
        tabIndex={drawerOpen || bookmarksOpen || settingsOpen || Boolean(deleteTarget) ? 0 : -1}
        onClick={() => {
          if (deleteTarget) {
            cancelDelete();
            return;
          }
          setDrawerOpen(false);
          setBookmarksOpen(false);
          setSettingsOpen(false);
        }}
      />

      <BookmarksDrawer
        open={bookmarksOpen}
        bookmarks={bookmarks}
        currentChapterId={Number.isFinite(chapterId) ? chapterId : 0}
        adding={addingBookmark}
        retrying={retryingBookmarks}
        canAdd={Boolean(chapter.data)}
        onClose={() => setBookmarksOpen(false)}
        onAdd={() => void addBookmark()}
        onJump={jumpToBookmark}
        onRequestDelete={requestDelete}
        onRetry={retryBookmarks}
      />

      <TocDrawer
        open={drawerOpen}
        loading={book.loading}
        error={book.error}
        volumes={volumes}
        bookId={bookId}
        currentChapterId={chapterId}
        onClose={() => setDrawerOpen(false)}
        onRetry={book.reload}
      />

      <BookmarkConfirmDialog
        open={Boolean(deleteTarget)}
        bookmark={deleteTarget}
        deleting={deletingBookmark}
        onCancel={cancelDelete}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  );
}