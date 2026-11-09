import { ChevronLeftIcon } from "../../components/Icons";

/** H5 顶栏三颗图标：朗读 / AI 辅读 / 全屏（路径取自定稿原型 index.html 的 ttsBtn / aiBtn / fsBtn）。 */
const ICON = {
  viewBox: "0 0 24 24",
  width: 20,
  height: 20,
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.8,
  strokeLinecap: "round",
  strokeLinejoin: "round",
  "aria-hidden": true,
} as const;

/**
 * 阅读器顶栏：PC 右侧是书架 / 详情 / 书签 / 搜索 / 目录 / 帮助 / 设置；
 * H5 是「返回 + 可点的书名（两行：书名 / 章号）+ 朗读 / AI / 全屏三颗 44×44 图标」——
 * 手机上没有横向空间再放文字按钮，三颗图标与返回同一套尺寸。
 * 其余按钮是常驻操作入口，禁用态用常驻说明而不是悬停提示。
 */
export function ReaderTopBar({
  isH5,
  hidden,
  bookTitle,
  progressText,
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
  /** H5 只放「第 N 章 / 共 M 章」；PC 拼成「章号 · 字数 · 全书 % · 还剩约」一行 */
  progressText: string;
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
          <TitleLines bookTitle={bookTitle} progressText={progressText} />
        </button>
      ) : (
        <div className="reader-top-title">
          <TitleLines bookTitle={bookTitle} progressText={progressText} />
        </div>
      )}
      <div className="reader-top-actions">
        {isH5 ? (
          <>
            <button
              className="icon-btn"
              type="button"
              aria-label="朗读本章"
              title="朗读本章"
              tabIndex={hidden ? -1 : undefined}
              onClick={onOpenTts}
            >
              <svg {...ICON}>
                <path d="M4 9.5h3.2L11.5 6v12L7.2 14.5H4z" />
                <path d="M15 9.4a3.7 3.7 0 0 1 0 5.2" />
                <path d="M17.7 6.8a7.4 7.4 0 0 1 0 10.4" />
              </svg>
            </button>
            <button
              className="icon-btn"
              type="button"
              aria-label="打开 AI 辅读"
              title="AI 辅读"
              tabIndex={hidden ? -1 : undefined}
              onClick={onOpenAi}
            >
              <svg {...ICON}>
                <path d="M11 3.6l1.7 4.7 4.7 1.7-4.7 1.7L11 16.4l-1.7-4.7L4.6 10l4.7-1.7z" />
                <path d="M18 15.4l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8z" />
              </svg>
            </button>
            <button
              className="icon-btn"
              type="button"
              aria-label={fullscreen ? "退出全屏" : "全屏阅读"}
              title="全屏阅读"
              aria-pressed={fullscreen}
              tabIndex={hidden ? -1 : undefined}
              onClick={onToggleFullscreen}
            >
              <svg {...ICON}>
                <path d={canFullscreen ? "M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" : "M9 4H4v5M15 4h5v5M4 15v5h5M15 20h5v-5"} />
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
 * 书名 + 进度：PC 放进 div（书名 + 一行进度），H5 放进可点的 button（书名 + 章号两行）。
 * 排版只写一次；H5 的字数 / 全书 % / 剩余时长不在顶栏（字数在窄屏会挤掉书名，百分比移到下方滑块行）。
 */
function TitleLines({ bookTitle, progressText }: { bookTitle: string; progressText: string }) {
  return (
    <>
      <strong>{bookTitle || "正在打开"}</strong>
      <span data-testid="reader-top-progress">{progressText}</span>
    </>
  );
}
