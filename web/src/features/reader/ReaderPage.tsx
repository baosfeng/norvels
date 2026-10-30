import { useCallback, useMemo, useState, type CSSProperties } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useBook, useBookmarks, useRecordReadingSession, useSaveProgress } from "../../api/hooks";
import { useIsH5 } from "../../lib/tier";
import { formatWords } from "../../lib/format";
import { useReaderControls } from "../../lib/useReaderControls";
import { prefersReducedMotion } from "../../lib/theme";
import { ReaderBottomBar } from "./ReaderBottomBar";
import { ReaderOverlays } from "./ReaderOverlays";
import { ReaderAutoScrollBar } from "./ReaderAutoScrollBar";
import { ReaderTopChrome } from "./ReaderTopChrome";
import { ReaderBody } from "./ReaderBody";
import { ReaderScrollProgress } from "./ReaderScrollProgress";
import { useReaderSearchState } from "./useReaderSearchState";
import { useReaderPanels } from "./useReaderPanels";
import { useReaderBookmarkActions } from "./useReaderBookmarkActions";
import { useReaderProgressSync } from "./useReaderProgressSync";
import { useChapterRetry } from "./useChapterRetry";
import { useReaderAutoScroll } from "./useReaderAutoScroll";
import { useReaderKeyboard } from "./useReaderKeyboard";
import { useReaderVisiblePara } from "./useReaderVisiblePara";
import { useReaderScrollRestore } from "./useReaderScrollRestore";
import { useReaderRepositionAfterReflow } from "./useReaderRepositionAfterReflow";
import { useReaderFullscreen } from "./useReaderFullscreen";
import { useReaderZoneNav } from "./useReaderZoneNav";
import { useReaderNavActions } from "./useReaderNavActions";
import { useReaderToggles } from "./useReaderToggles";
import { readerStyleVars } from "./readerSettings";
import { useReaderSettings } from "./useReaderSettings";
import { useReaderSwipe } from "./useReaderSwipe";
import { useChapterStream } from "./useChapterStream";
import { tradToSimp } from "../../lib/trad2simp";
import { pickStoredParaIndex, useReaderProgress } from "./readerProgress";
import { useReadingSessionReporter } from "../../lib/readingTimer";

/**
 * 阅读器：产品核心，两端能力等价。
 * PC：设置面板浮在右上、目录从右侧抽屉滑出；H5：设置面板变宽、目录全屏抽屉。
 * 五档背景 / 五档字号 / 三区手势 / 进度拖动跳章 / 键盘翻页 / 进度恢复在两端都可用。
 *
 * 正文是一条连续章节流（useChapterStream）：滚过章末继续下滑就是下一章。
 * params.chapterId 只表示「路由带来的入口章」，读到哪一章由流自己跟踪（stream.activeId），
 * 两者在滚动跨章时用 replace 对齐，所以这里统一用 chapterId = stream.activeId。
 */
export function ReaderPage() {
  const params = useParams();
  const navigate = useNavigate();
  const isH5 = useIsH5();
  const bookId = Number(params.bookId);
  const routeChapterId = Number(params.chapterId);

  const book = useBook(Number.isFinite(bookId) ? bookId : null);
  const stream = useChapterStream({ bookId, routeChapterId, book: book.data });
  const chapterId = stream.activeId;
  const saveProgress = useSaveProgress();
  const recordSession = useRecordReadingSession();
  useReadingSessionReporter({ bookId, chapterId, report: recordSession });
  const bookmarks = useBookmarks(Number.isFinite(bookId) ? bookId : null);
  const progressList = useReaderProgress(bookId);

  const [hintDismissedFor, setHintDismissedFor] = useState(0);
  const [anchor, setAnchor] = useState({ chapterId: 0, para: 0 });
  const [pendingJump, setPendingJump] = useState<{ chapterId: number; paraIndex: number } | null>(null);

  const { chapters, volumes, activeIndex: index, prev, next } = stream;
  const { settings, changeSettings, theme, themes, changeTheme } = useReaderSettings(stream.activeParagraphs);

  // 简繁映射作用到流里的每一章：滚到下一章时字形仍是同一个口径
  const segments = useMemo(
    () =>
      stream.segments.map((segment) =>
        settings.script ? { ...segment, paragraphs: segment.paragraphs.map(tradToSimp) } : segment,
      ),
    [stream.segments, settings.script],
  );
  const activeSegment = segments.find((segment) => segment.id === chapterId);
  const viewParagraphs = activeSegment?.paragraphs ?? [];

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
  } = useReaderPanels({ canSearch: Boolean(stream.chapterData), onResetSearch: resetSearch });

  const anchorPara = anchor.chapterId === chapterId ? anchor.para : 0;

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
    chapterTitle: activeSegment?.title ?? null,
    navigate,
    bookmarks,
    onOpenBookmarks: () => openDrawer("bookmarks"),
    onCloseBookmarks: closeDrawer,
    onPendingJump: setPendingJump,
  });

  const overlayOpen = settingsOpen || drawerOpen || searchOpen || helpOpen || Boolean(deleteTarget);

  const { goShelf, goBack, goChapter } = useReaderNavActions(bookId);

  // consumeFlip / goFlip 由翻章出口自己标记：只有翻章落章首，目录链接与浏览器后退按段恢复。
  const { step: stepChapter, jump: jumpChapter, goFlip, consumeFlip } = useReaderZoneNav({ prev, next, goChapter });

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

  const { retryingChapter, chapterFailures, retryChapter } = useChapterRetry({
    chapter: { data: stream.chapterData, error: stream.error, reload: stream.reload },
    chapterId,
  });

  const storedParaIndex = useMemo(
    () => pickStoredParaIndex(progressList.data?.items ?? [], bookId, chapterId),
    [progressList.data, bookId, chapterId],
  );

  useReaderProgressSync({
    bookId,
    chapterId,
    chapters,
    index,
    chapterDataId: stream.chapterData?.id,
    storedParaIndex,
    anchor,
    saveProgress,
  });

  const { autoRunning, autoPaused, windowActive, setAutoPaused, exitAutoScroll } = useReaderAutoScroll({
    settings,
    updateSettings: changeSettings,
    overlayOpen,
    next: stream.flipTarget,
    index: stream.flipIndex,
    goChapter: goFlip,
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

  useReaderVisiblePara({ segments: stream.segments, onAnchor: setAnchor });

  useReaderScrollRestore({
    consumeFlip,
    consumeAdvance: stream.consumeAdvance,
    searchOpen,
    currentHit,
    hitCount,
    chapterId,
    chapterDataId: stream.chapterData?.id,
    pendingJump,
    storedParaIndex,
    progressLoading: progressList.loading,
    onRestoreAnchor: setAnchor,
  });

  // 改字号 / 行距 / 段距 / 字体 / 页宽 / 对齐会让正文重排，像素位置随之失效：按段落锚点回到原处
  useReaderRepositionAfterReflow({ settings, chapterDataId: stream.chapterData?.id, anchor });

  const restoreHintVisible =
    Boolean(stream.chapterData) && storedParaIndex > 0 && !pendingJump && hintDismissedFor !== chapterId;

  const reducedMotion = prefersReducedMotion();
  const swipeRef = useReaderSwipe({ isH5, overlayOpen, reducedMotion, step: stepChapter });

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

  const bookTitle = book.data?.title ?? stream.chapterData?.bookTitle ?? "";
  const chapterWords = activeSegment ? formatWords(activeSegment.wordCount) : "";
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
        searchEnabled={Boolean(stream.chapterData)}
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
        loading={stream.loading}
        error={stream.error}
        retrying={retryingChapter}
        retryFailures={chapterFailures}
        // 流里的段落按当前设置映射过简繁（渲染与搜索 / 书签用同一份文本）
        stream={{ ...stream, segments }}
        onRetry={retryChapter}
        onPrev={() => stepChapter("prev")}
        onNext={() => stepChapter("next")}
        onJumpChapter={jumpToChapterAt}
        onShelf={goShelf}
        hitsByPara={hitsByPara}
        currentHit={currentHit}
        searching={searchOpen}
        bookId={bookId}
        reducedMotion={reducedMotion}
        hintVisible={restoreHintVisible}
        onDismissHint={dismissHint}
      />

      <ReaderScrollProgress resetKey={chapterId} />

      {isH5 ? (
        <ReaderBottomBar
          hidden={!controls.visible}
          chapters={chapters}
          index={index}
          prev={prev}
          next={next}
          searchEnabled={Boolean(stream.chapterData)}
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
        autoScrollAvailable={Boolean(stream.chapterData)}
        volumes={volumes}
        bookId={bookId}
        currentChapterId={chapterId}
        tocLoading={book.loading}
        tocError={book.error}
        bookmarks={bookmarks}
        addingBookmark={addingBookmark}
        retryingBookmarks={retryingBookmarks}
        canAddBookmark={Boolean(stream.chapterData)}
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