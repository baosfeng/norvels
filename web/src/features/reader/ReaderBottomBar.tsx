import type { ChapterBrief } from "../../api/types";
import { ReaderChapterSlider } from "./ReaderChapterSlider";

const ICON = { width: 20, height: 20, fill: "none", stroke: "currentColor", strokeWidth: 1.7, "aria-hidden": true } as const;

function IconPrev() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <path d="M14.5 5.5 8 12l6.5 6.5" />
    </svg>
  );
}
function IconNext() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <path d="M9.5 5.5 16 12l-6.5 6.5" />
    </svg>
  );
}
function IconMenu() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <path d="M4 6h16M4 12h16M4 18h10" />
    </svg>
  );
}
function IconSettings() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <path d="M4 8h9M17 8h3M4 16h3M11 16h9" />
      <circle cx="15" cy="8" r="2" />
      <circle cx="9" cy="16" r="2" />
    </svg>
  );
}
function IconBookmark() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <path d="M7 4h10v16l-5-3.5L7 20z" />
    </svg>
  );
}
function IconShelf() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <path d="M4 5h16M6 5v14M18 5v14M4 19h16M9 9h6M9 13h6" />
    </svg>
  );
}
function IconSearch() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <circle cx="11" cy="11" r="5.6" />
      <path d="M15.4 15.4 20 20" />
    </svg>
  );
}

/**
 * H5 底部控制栏：与顶部 header 同属控制层，一起显隐。
 * 两行结构：第一行是全书进度滑块（右端「全书 % · 还剩约」），第二行是 7 个入口
 * （上一章 / 目录 / 书签 / 搜索 / 设置 / 书架 / 下一章），每格 ≥48px、整条 ≤120px。
 * 首末章与正文未载入时按钮置灰：格内只留 9px 单行短词（首章 / 未载入 / 末章），
 * 完整理由常驻在 aria-label / title——长句在 ~51px 的格里换行会把底栏撑高。
 */
export function ReaderBottomBar({
  hidden,
  chapters,
  index,
  prev,
  next,
  searchEnabled,
  progressText,
  onPrev,
  onNext,
  onJumpChapter,
  onOpenDrawer,
  onGoShelf,
  onToggleSearch,
  onToggleSettings,
  settingsOpen,
  drawerOpen,
  drawerTab,
  searchOpen,
}: {
  hidden: boolean;
  chapters: ChapterBrief[];
  index: number;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  searchEnabled: boolean;
  /** 滑块行右端：全书 % · 还剩约；空串（目录未就绪）时不渲染 */
  progressText: string;
  onPrev: () => void;
  onNext: () => void;
  onJumpChapter: (index: number) => void;
  onOpenDrawer: (tab: "toc" | "bookmarks") => void;
  onGoShelf: () => void;
  onToggleSearch: () => void;
  onToggleSettings: () => void;
  settingsOpen: boolean;
  drawerOpen: boolean;
  drawerTab: "toc" | "bookmarks";
  searchOpen: boolean;
}) {
  // 与顶栏同一套：收起态整条 bar 加 inert（键盘不可达），老浏览器靠子元素 tabIndex=-1 兜底
  const tabInert = hidden ? -1 : undefined;
  return (
    <nav
      className="h5-reader-bar"
      data-testid="reader-bar"
      aria-label="阅读控制"
      aria-hidden={hidden}
      inert={hidden}
    >
      <div className="h5-reader-bar-top">
        <div className="h5-reader-bar-slider">
          <ReaderChapterSlider
            chapters={chapters}
            index={index}
            disabled={chapters.length === 0}
            tabIndex={hidden ? -1 : undefined}
            testId="reader-bar-slider"
            onCommit={onJumpChapter}
          />
        </div>
        {progressText ? (
          <span className="h5-reader-bar-pct" data-testid="reader-bar-pct">
            {progressText}
          </span>
        ) : null}
      </div>
      <div className="h5-reader-bar-actions">
        <button
          type="button"
          aria-label={prev ? "上一章" : "上一章（已是首章）"}
          title={prev ? undefined : "已是首章"}
          tabIndex={tabInert}
          disabled={!prev}
          onClick={onPrev}
        >
          <IconPrev />
          <span>上一章</span>
          {!prev ? <span className="inline-note">首章</span> : null}
        </button>
        <button type="button" aria-label="章节目录" tabIndex={tabInert} aria-expanded={drawerOpen && drawerTab === "toc"} onClick={() => onOpenDrawer("toc")}>
          <IconMenu />
          <span>目录</span>
        </button>
        <button type="button" aria-label="打开书签" tabIndex={tabInert} aria-expanded={drawerOpen && drawerTab === "bookmarks"} onClick={() => onOpenDrawer("bookmarks")}>
          <IconBookmark />
          <span>书签</span>
        </button>
        <button
          type="button"
          aria-label={searchEnabled ? "搜索本章" : "搜索本章（正文还没载入，稍后可以搜索）"}
          title={searchEnabled ? undefined : "正文还没载入，稍后可以搜索"}
          tabIndex={tabInert}
          aria-expanded={searchOpen}
          disabled={!searchEnabled}
          onClick={onToggleSearch}
        >
          <IconSearch />
          <span>搜索</span>
          {!searchEnabled ? <span className="inline-note">未载入</span> : null}
        </button>
        <button
          type="button"
          aria-label="阅读设置"
          tabIndex={tabInert}
          aria-expanded={settingsOpen}
          data-testid="reader-bottombar-settings"
          onClick={onToggleSettings}
        >
          <IconSettings />
          <span>设置</span>
        </button>
        <button type="button" aria-label="返回书架" tabIndex={tabInert} onClick={onGoShelf}>
          <IconShelf />
          <span>书架</span>
        </button>
        <button
          type="button"
          aria-label={next ? "下一章" : "下一章（已是末章）"}
          title={next ? undefined : "已是末章"}
          tabIndex={tabInert}
          disabled={!next}
          onClick={onNext}
        >
          <IconNext />
          <span>下一章</span>
          {!next ? <span className="inline-note">末章</span> : null}
        </button>
      </div>
    </nav>
  );
}
