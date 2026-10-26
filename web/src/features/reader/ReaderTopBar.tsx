import { ChevronLeftIcon } from "../../components/Icons";

/**
 * 阅读器顶栏：PC 右侧是书架 / 书签 / 搜索 / 目录 / 帮助 / 设置，H5 只保留全屏。
 * 每个按钮带契约锚点（C-01…C-08），禁用态用常驻说明而不是悬停提示。
 */
export function ReaderTopBar({
  isH5,
  hidden,
  bookTitle,
  progressText,
  fullscreen,
  canFullscreen,
  searchEnabled,
  bookmarksOpen,
  searchOpen,
  drawerOpen,
  helpOpen,
  settingsOpen,
  onBack,
  onToggleFullscreen,
  onShelf,
  onOpenBookmarks,
  onToggleSearch,
  onOpenDirectory,
  onToggleHelp,
  onToggleSettings,
}: {
  isH5: boolean;
  hidden: boolean;
  bookTitle: string;
  progressText: string;
  fullscreen: boolean;
  canFullscreen: boolean;
  searchEnabled: boolean;
  bookmarksOpen: boolean;
  searchOpen: boolean;
  drawerOpen: boolean;
  helpOpen: boolean;
  settingsOpen: boolean;
  onBack: () => void;
  onToggleFullscreen: () => void;
  onShelf: () => void;
  onOpenBookmarks: () => void;
  onToggleSearch: () => void;
  onOpenDirectory: () => void;
  onToggleHelp: () => void;
  onToggleSettings: () => void;
}) {
  return (
    <header
      className="reader-top"
      data-testid="reader-top"
      data-od-id="reader-top"
      aria-hidden={hidden || undefined}
    >
      <button className="icon-btn" type="button" aria-label="返回目录" data-od-id="C-01" onClick={onBack}>
        <ChevronLeftIcon />
      </button>
      <div className="reader-top-title">
        <strong>{bookTitle || "正在打开"}</strong>
        <span data-testid="reader-top-progress">{progressText}</span>
      </div>
      <div className="reader-top-actions">
        {isH5 ? (
          <button
            className="icon-btn"
            type="button"
            aria-label={fullscreen ? "退出全屏" : "全屏阅读"}
            aria-pressed={fullscreen}
            aria-haspopup="true"
            data-od-id="C-02"
            onClick={onToggleFullscreen}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
              <path d={canFullscreen ? "M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" : "M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"} />
            </svg>
          </button>
        ) : (
          <>
            <button className="btn" type="button" aria-label="返回书架" data-od-id="C-03" onClick={onShelf}>
              书架
            </button>
            <button
              className="btn"
              type="button"
              onClick={onOpenBookmarks}
              aria-expanded={bookmarksOpen}
              aria-label="打开书签"
              data-od-id="C-04"
            >
              书签
            </button>
            <button
              className="btn"
              type="button"
              aria-expanded={searchOpen}
              aria-label="搜索本章"
              disabled={!searchEnabled}
              data-od-id="C-05"
              onClick={onToggleSearch}
            >
              搜索
            </button>
            {!searchEnabled ? <span className="inline-note">正文还没载入，稍后可以搜索</span> : null}
            <button className="btn" type="button" onClick={onOpenDirectory} aria-expanded={drawerOpen} data-od-id="C-06">
              目录
            </button>
            <button
              className="btn"
              type="button"
              aria-label="快捷键帮助"
              aria-expanded={helpOpen}
              data-od-id="C-07"
              onClick={onToggleHelp}
            >
              ?
            </button>
            <button className="btn" type="button" onClick={onToggleSettings} aria-expanded={settingsOpen} data-od-id="C-08">
              设置
            </button>
          </>
        )}
      </div>
    </header>
  );
}