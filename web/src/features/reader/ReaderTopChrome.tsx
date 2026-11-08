import { ReaderHelpPanel } from "./ReaderHelpPanel";
import { ReaderSearchPanel } from "./ReaderSearchPanel";
import { ReaderTopBar } from "./ReaderTopBar";

/**
 * 阅读器顶部的一切：控制栏 + 快捷键帮助 + 章节内搜索。
 * 这三者都不碰正文数据，只读显隐状态与回调，单独成组免得页面正文被它们淹掉。
 */
export function ReaderTopChrome({
  isH5,
  controlsVisible,
  bookTitle,
  progressText,
  progressSubtext,
  fullscreen,
  canFullscreen,
  searchEnabled,
  searchOpen,
  drawerOpen,
  helpOpen,
  settingsOpen,
  search,
  onBack,
  onToggleFullscreen,
  onShelf,
  onOpenBookDetail,
  onOpenBookmarks,
  onToggleSearch,
  onOpenDirectory,
  onToggleHelp,
  onToggleSettings,
  ttsSpeaking,
  onToggleTTS,
  onCloseHelp,
  onCloseSearch,
}: {
  isH5: boolean;
  controlsVisible: boolean;
  bookTitle: string;
  progressText: string;
  /** H5 顶栏第二行小字（全书 % · 还剩约）；PC 不传，进度拼成一行 */
  progressSubtext?: string;
  fullscreen: boolean;
  canFullscreen: boolean;
  searchEnabled: boolean;
  searchOpen: boolean;
  drawerOpen: boolean;
  helpOpen: boolean;
  settingsOpen: boolean;
  search: {
    query: string;
    hits: number;
    total: number;
    truncated: boolean;
    current: number;
    onQueryChange: (query: string) => void;
    onPrev: () => void;
    onNext: () => void;
  };
  onBack: () => void;
  onToggleFullscreen: () => void;
  onShelf: () => void;
  onOpenBookDetail: () => void;
  onOpenBookmarks: () => void;
  onToggleSearch: () => void;
  onOpenDirectory: () => void;
  onToggleHelp: () => void;
  onToggleSettings: () => void;
  ttsSpeaking?: boolean;
  onToggleTTS?: () => void;
  onCloseHelp: () => void;
  onCloseSearch: () => void;
}) {
  return (
    <>
      <ReaderTopBar
        isH5={isH5}
        hidden={isH5 && !controlsVisible}
        bookTitle={bookTitle}
        progressText={progressText}
        progressSubtext={progressSubtext}
        fullscreen={fullscreen}
        canFullscreen={canFullscreen}
        searchEnabled={searchEnabled}
        searchOpen={searchOpen}
        drawerOpen={drawerOpen}
        helpOpen={helpOpen}
        settingsOpen={settingsOpen}
        onBack={onBack}
        onToggleFullscreen={onToggleFullscreen}
        onShelf={onShelf}
        onOpenBookDetail={onOpenBookDetail}
        onOpenBookmarks={onOpenBookmarks}
        onToggleSearch={onToggleSearch}
        onOpenDirectory={onOpenDirectory}
        onToggleHelp={onToggleHelp}
        onToggleSettings={onToggleSettings}
        ttsSpeaking={ttsSpeaking}
        onToggleTTS={onToggleTTS}
      />

      <ReaderHelpPanel open={helpOpen} onClose={onCloseHelp} />

      <ReaderSearchPanel
        open={searchOpen}
        query={search.query}
        hits={search.hits}
        total={search.total}
        truncated={search.truncated}
        current={search.current}
        onQueryChange={search.onQueryChange}
        onPrev={search.onPrev}
        onNext={search.onNext}
        onClose={onCloseSearch}
      />
    </>
  );
}
