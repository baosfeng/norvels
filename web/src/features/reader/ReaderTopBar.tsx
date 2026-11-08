import { ChevronLeftIcon } from "../../components/Icons";

/**
 * 阅读器顶栏：PC 右侧是书架 / 详情 / 书签 / 搜索 / 目录 / 帮助 / 设置；
 * H5 是「返回 + 可点的书名 + 全屏」——手机上没有横向空间再放一颗按钮，
 * 书名本来就是读者想点的地方，点它回书籍详情（aria-label 与 PC 的「详情」一致）。
 * 其余按钮是常驻操作入口，禁用态用常驻说明而不是悬停提示。
 */
export function ReaderTopBar({
  isH5,
  hidden,
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
  onBack,
  onToggleFullscreen,
  onShelf,
  onOpenBookDetail,
  onOpenBookmarks,
  onToggleSearch,
  onOpenDirectory,
  onToggleHelp,
  onToggleSettings,
  onOpenTts,
  onOpenAi,
}: {
  isH5: boolean;
  hidden: boolean;
  bookTitle: string;
  progressText: string;
  /** H5 顶栏第二行小字（全书 % · 还剩约）；不传就只显示主进度行 */
  progressSubtext?: string;
  fullscreen: boolean;
  canFullscreen: boolean;
  searchEnabled: boolean;
  searchOpen: boolean;
  drawerOpen: boolean;
  helpOpen: boolean;
  settingsOpen: boolean;
  onBack: () => void;
  onToggleFullscreen: () => void;
  onShelf: () => void;
  /** 回书籍详情（/book/:bookId）：PC 是顶栏「详情」按钮，H5 是可点的书名 */
  onOpenBookDetail: () => void;
  onOpenBookmarks: () => void;
  onToggleSearch: () => void;
  onOpenDirectory: () => void;
  onToggleHelp: () => void;
  onToggleSettings: () => void;
  onOpenTts?: () => void;
  onOpenAi?: () => void;
}) {
  // H5 收起态：aria-hidden 只对读屏生效，键盘仍能 Tab 进屏幕外的按钮。
  // 容器加 inert（把整棵子树移出焦点顺序），老浏览器不认时靠各按钮的 tabIndex=-1 兜底。
  return (
    <header
      className="reader-top"
      data-testid="reader-top"
      aria-hidden={hidden ? "true" : "false"}
      inert={hidden}
    >
      <button
        className="icon-btn"
        type="button"
        aria-label="返回书籍详情"
        tabIndex={hidden ? -1 : undefined}
        onClick={onBack}
      >
        <ChevronLeftIcon />
      </button>
      {isH5 ? (
        <button
          className="reader-top-title reader-top-title-btn"
          type="button"
          aria-label="查看书籍详情"
          tabIndex={hidden ? -1 : undefined}
          data-testid="reader-top-detail"
          onClick={onOpenBookDetail}
        >
          <TitleLines bookTitle={bookTitle} progressText={progressText} subtext={progressSubtext} />
        </button>
      ) : (
        <div className="reader-top-title">
          <TitleLines bookTitle={bookTitle} progressText={progressText} />
        </div>
      )}
      <div className="reader-top-actions">
        {isH5 ? (
          <>
            <button className="btn" type="button" aria-label="朗读本章" onClick={onOpenTts}>朗读</button>
            <button className="btn" type="button" aria-label="打开 AI 辅读" onClick={onOpenAi}>AI</button>
            <button
            className="icon-btn"
            type="button"
            aria-label={fullscreen ? "退出全屏" : "全屏阅读"}
            aria-pressed={fullscreen}
            aria-haspopup="true"
            tabIndex={hidden ? -1 : undefined}
            onClick={onToggleFullscreen}
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
              <path d={canFullscreen ? "M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" : "M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"} />
            </svg>
          </button>
          </>
        ) : (
          <>
            <button className="btn" type="button" aria-label="返回书架" onClick={onShelf}>
              书架
            </button>
            <button
              className="btn"
              type="button"
              aria-label="查看书籍详情"
              data-testid="reader-top-detail"
              onClick={onOpenBookDetail}
            >
              详情
            </button>
            <button
              className="btn"
              type="button"
              onClick={onOpenBookmarks}
              aria-expanded={drawerOpen}
              aria-label="打开书签"
            >
              书签
            </button>
            <button
              className="btn"
              type="button"
              aria-expanded={searchOpen}
              aria-label="搜索本章"
              disabled={!searchEnabled}
              onClick={onToggleSearch}
            >
              搜索
            </button>
            {!searchEnabled ? <span className="inline-note">正文还没载入，稍后可以搜索</span> : null}
            <button className="btn" type="button" aria-label="朗读本章" onClick={onOpenTts}>朗读</button>
            <button className="btn" type="button" aria-label="打开 AI 辅读" onClick={onOpenAi}>AI</button>
            <button className="btn" type="button" onClick={onOpenDirectory} aria-expanded={drawerOpen}>
              目录
            </button>
            <button
              className="btn"
              type="button"
              aria-label="快捷键帮助"
              aria-expanded={helpOpen}
              onClick={onToggleHelp}
            >
              ?
            </button>
            <button
              className="btn"
              type="button"
              onClick={onToggleSettings}
              aria-expanded={settingsOpen}
              data-testid="reader-top-settings"
            >
              设置
            </button>
          </>
        )}
      </div>
    </header>
  );
}

/**
 * 书名 + 进度（+ H5 的第二行小字）：PC 放进 div、H5 放进可点的 button，排版只写一次。
 * 第二行是「全书 % · 还剩约」，与 PC 顶栏同一份数据；H5 只是换行显示，不另算一遍。
 */
function TitleLines({
  bookTitle,
  progressText,
  subtext,
}: {
  bookTitle: string;
  progressText: string;
  subtext?: string;
}) {
  return (
    <>
      <strong>{bookTitle || "正在打开"}</strong>
      <span data-testid="reader-top-progress">{progressText}</span>
      {subtext ? (
        <span className="reader-top-sub" data-testid="reader-top-progress-sub">
          {subtext}
        </span>
      ) : null}
    </>
  );
}