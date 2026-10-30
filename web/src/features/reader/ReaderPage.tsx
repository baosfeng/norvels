import { useCallback, useMemo, useState, type CSSProperties } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useBook, useBookmarks, useChapter, useRecordReadingSession, useSaveProgress } from "../../api/hooks";
import { useIsH5 } from "../../lib/tier";
import { formatWords } from "../../lib/format";
import { useReaderControls } from "../../lib/useReaderControls";
import { useSwipePaging } from "../../lib/useSwipePaging";
import { prefersReducedMotion } from "../../lib/theme";
import { splitParagraphs } from "../../lib/readerSearch";
import { ReaderBottomBar } from "./ReaderBottomBar";
import { ReaderOverlays } from "./ReaderOverlays";
import { ReaderAutoScrollBar } from "./ReaderAutoScrollBar";
import { ReaderTopChrome } from "./ReaderTopChrome";
import { ReaderBody } from "./ReaderBody";
import { useReaderSearchState } from "./useReaderSearchState";
import { useReaderPanels } from "./useReaderPanels";
import { useReaderBookmarkActions } from "./useReaderBookmarkActions";
import { useReaderProgressSync } from "./useReaderProgressSync";
import { useChapterRetry } from "./useChapterRetry";
import { useReaderAutoScroll } from "./useReaderAutoScroll";
import { useReaderKeyboard } from "./useReaderKeyboard";
import { useReaderVisiblePara } from "./useReaderVisiblePara";
import { useReaderScrollRestore } from "./useReaderScrollRestore";
import { useReaderChapterIndex } from "./useReaderChapterIndex";
import { useReaderFullscreen } from "./useReaderFullscreen";
import { useReaderZoneNav } from "./useReaderZoneNav";
import { useReaderNavActions } from "./useReaderNavActions";
import { useReaderToggles } from "./useReaderToggles";
import { readerStyleVars } from "./readerSettings";
import { useReaderSettings } from "./useReaderSettings";
import { tradToSimp } from "../../lib/trad2simp";
import { pickStoredParaIndex, useReaderProgress } from "./readerProgress";
import { useReadingSessionReporter } from "../../lib/readingTimer";

/**
 * 阅读器：产品核心，两端能力等价。
 * PC：设置面板浮在右上、目录从右侧抽屉滑出；H5：设置面板变宽、目录全屏抽屉。
 * 五档背景 / 五档字号 / 三区手势 / 进度拖动跳章 / 键盘翻页 / 进度恢复在两端都可用。
 */
export function ReaderPage() {
  const params = useParams();
  const navigate = useNavigate();
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

  const { chapters, volumes, index, prev, next, percent } = useReaderChapterIndex(book.data, chapterId);

  const content = chapter.data?.content ?? "";
  const paragraphs = useMemo(() => splitParagraphs(content), [content]);
  const { settings, changeSettings, theme, themes, changeTheme } = useReaderSettings(paragraphs);
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
    drawerTab,
    settingsOpen,
    searchOpen,
    helpOpen,
    setDrawerOpen,
    setDrawerTab,
    setSettingsOpen,
    setHelpOpen,
    openDrawer,
    closeDrawer,
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
    onOpenBookmarks: () => openDrawer("bookmarks"),
    onCloseBookmarks: closeDrawer,
    onPendingJump: setPendingJump,
  });

  const overlayOpen = settingsOpen || drawerOpen || searchOpen || helpOpen || Boolean(deleteTarget);

  const { goShelf, goBack, goChapter } = useReaderNavActions(bookId);

  const { step: stepChapter, jump: jumpChapter } = useReaderZoneNav({ prev, next, goChapter });

  /** 进度滑块松手：按章节下标跳章（滑块与手势共用同一句浮条口径）。 */
  const jumpToChapterAt = useCallback(
    (at: number) => {
      const target = chapters[at];
      if (!target || target.id === chapterId) return;
      jumpChapter(target, "已经是第一章了");
    },
    [chapters, chapterId, jumpChapter],
  );


  const controls = useReaderControls({
    isH5,
    paused: overlayOpen,
    onZoneTap: stepChapter,
  });
  const { fullscreen, canFullscreen, toggle: toggleFullscreen } = useReaderFullscreen({ onUnsupported: controls.hide });

  const { retryingChapter, chapterFailures, retryChapter } = useChapterRetry({ chapter, chapterId });

  const storedParaIndex = useMemo(
    () => pickStoredParaIndex(progressList.data?.items ?? [], bookId, chapterId),
    [progressList.data, bookId, chapterId],
  );


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

  const { autoRunning, autoPaused, windowActive, setAutoPaused, exitAutoScroll } = useReaderAutoScroll({
    settings,
    updateSettings: changeSettings,
    overlayOpen,
    next,
    index,
    goChapter,
    isH5,
  });

  const closeAllPanels = useCallback(() => {
    setDrawerOpen(false);
    setSettingsOpen(false);
    setHelpOpen(false);
  }, [setDrawerOpen, setSettingsOpen, setHelpOpen]);

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
    goChapterAnnounce: (target) => jumpChapter(target, "已经是最后一章了"),
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
    () => ({ enabled: isH5, onPrev: () => stepChapter("prev"), onNext: () => stepChapter("next") }),
    [isH5, stepChapter],
  );
  const swipeRef = useSwipePaging(swipeHandlers);

  const { toggleSearch, toggleSettings, toggleDirectory, toggleBookmarks } = useReaderToggles({
    searchOpen,
    settingsOpen,
    drawerOpen,
    onOpenSearch: openSearch,
    onCloseSearch: closeSearch,
    onOpenSettings: openSettings,
    onCloseSettings: () => setSettingsOpen(false),
    onOpenDrawer: openDrawer,
    onCloseDrawer: closeDrawer,
  });

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
      <ReaderTopChrome
        isH5={isH5}
        controlsVisible={controls.visible}
        bookTitle={bookTitle}
        progressText={progressText}
        fullscreen={fullscreen}
        canFullscreen={canFullscreen}
        searchEnabled={Boolean(chapter.data)}
        searchOpen={searchOpen}
        drawerOpen={drawerOpen}
        helpOpen={helpOpen}
        settingsOpen={settingsOpen}
        search={{
          query: searchQuery,
          hits: hitCount,
          total: search.total,
          truncated: search.truncated,
          current: currentHit,
          onQueryChange: setSearchQuery,
          onPrev: () => stepHit(-1),
          onNext: () => stepHit(1),
        }}
        onBack={goBack}
        onToggleFullscreen={() => void toggleFullscreen()}
        onShelf={goShelf}
        onOpenBookmarks={toggleBookmarks}
        onToggleSearch={toggleSearch}
        onOpenDirectory={toggleDirectory}
        onToggleHelp={toggleHelp}
        onToggleSettings={toggleSettings}
        onCloseHelp={() => setHelpOpen(false)}
        onCloseSearch={closeSearch}
      />

      <ReaderBody
        isH5={isH5}
        swipeRef={swipeRef}
        surfaceProps={controls.surfaceProps}
        zonesEnabled={!overlayOpen}
        loading={chapter.loading}
        error={chapter.error}
        chapter={chapter.data ?? null}
        retrying={retryingChapter}
        retryFailures={chapterFailures}
        next={next}
        onRetry={retryChapter}
        onPrev={() => stepChapter("prev")}
        onNext={() => stepChapter("next")}
        onJumpChapter={jumpToChapterAt}
        onShelf={goShelf}
        paragraphs={viewParagraphs}
        hitsByPara={hitsByPara}
        currentHit={currentHit}
        searching={searchOpen}
        bookId={bookId}
        chapters={chapters}
        index={index}
        prev={prev}
        percent={percent}
        enterKey={articleKey}
        reducedMotion={reducedMotion}
        hintVisible={restoreHintVisible}
        onDismissHint={dismissHint}
      />

      {isH5 ? (
        <ReaderBottomBar
          hidden={!controls.visible}
          chapters={chapters}
          index={index}
          prev={prev}
          next={next}
          searchEnabled={Boolean(chapter.data)}
          onPrev={() => stepChapter("prev")}
          onNext={() => stepChapter("next")}
          onJumpChapter={jumpToChapterAt}
          onOpenDrawer={openDrawer}
          onGoShelf={goShelf}
          onToggleSearch={toggleSearch}
          onToggleSettings={toggleSettings}
          settingsOpen={settingsOpen}
          drawerOpen={drawerOpen}
          drawerTab={drawerTab}
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

      <ReaderOverlays
        isH5={isH5}
        settingsOpen={settingsOpen}
        drawerOpen={drawerOpen}
        drawerTab={drawerTab}
        deleteTarget={deleteTarget}
        deleting={deletingBookmark}
        settings={settings}
        theme={theme}
        themes={themes}
        autoScrollAvailable={Boolean(chapter.data)}
        volumes={volumes}
        bookId={bookId}
        currentChapterId={chapterId}
        tocLoading={book.loading}
        tocError={book.error}
        bookmarks={bookmarks}
        addingBookmark={addingBookmark}
        retryingBookmarks={retryingBookmarks}
        canAddBookmark={Boolean(chapter.data)}
        onClosePanels={closeAllPanels}
        onCancelDelete={cancelDelete}
        onConfirmDelete={() => void confirmDelete()}
        onChangeSettings={changeSettings}
        onSetTheme={changeTheme}
        onTabChange={setDrawerTab}
        onTocRetry={book.reload}
        onAddBookmark={() => void addBookmark()}
        onJumpBookmark={jumpToBookmark}
        onRequestDeleteBookmark={requestDelete}
        onBookmarksRetry={retryBookmarks}
      />
    </div>
  );
}
