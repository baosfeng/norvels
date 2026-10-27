import { useCallback, useEffect, useMemo, useState, type CSSProperties } from "react";
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
import { useReaderAutoScroll } from "./useReaderAutoScroll";
import { useReaderKeyboard } from "./useReaderKeyboard";
import { useReaderVisiblePara } from "./useReaderVisiblePara";
import { useReaderScrollRestore } from "./useReaderScrollRestore";
import { ReaderArticle } from "./ReaderArticle";
import { readReaderSettings, readerStyleVars, writeReaderSettings, type ReaderSettings } from "./readerSettings";
import { tradToSimp } from "../../lib/trad2simp";
import { pickStoredParaIndex, useReaderProgress } from "./readerProgress";
import { useReadingSessionReporter } from "../../lib/readingTimer";

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


  const overlayOpen = settingsOpen || drawerOpen || bookmarksOpen || searchOpen || helpOpen || Boolean(deleteTarget);

  const { autoRunning, autoPaused, windowActive, setAutoPaused, exitAutoScroll } = useReaderAutoScroll({
    settings,
    updateSettings,
    overlayOpen,
    next,
    index,
    goChapter,
    isH5,
  });

  const closeAllPanels = useCallback(() => {
    setDrawerOpen(false);
    setSettingsOpen(false);
    setBookmarksOpen(false);
    setHelpOpen(false);
  }, [setDrawerOpen, setSettingsOpen, setBookmarksOpen, setHelpOpen]);

  useReaderKeyboard({
    deleteTarget,
    onCancelDelete: cancelDelete,
    onClosePanels: closeAllPanels,
    closeSearch,
    toggleHelp,
    helpOpen,
    searchOpen,
    prev,
    next,
    goChapterAnnounce,
  });

  useReaderVisiblePara({ paragraphs, chapterDataId: chapter.data?.id, onAnchor: setAnchor });


  useReaderScrollRestore({
    searchOpen,
    currentHit,
    hitCount,
    chapterId,
    chapterDataId: chapter.data?.id,
    pendingJump,
    storedParaIndex,
    progressLoading: progressList.loading,
    onRestoreAnchor: setAnchor,
  });

  const restoreHintVisible =
    Boolean(chapter.data) && storedParaIndex > 0 && !pendingJump && hintDismissedFor !== chapterId;

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