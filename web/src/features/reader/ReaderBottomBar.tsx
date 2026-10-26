import type { ChapterBrief } from "../../api/types";

const ICON = { width: 20, height: 20, fill: "none", stroke: "currentColor", strokeWidth: 1.7, "aria-hidden": true } as const;

function IconPrev() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <path d="M15 18l-6-6 6-6" />
    </svg>
  );
}
function IconNext() {
  return (
    <svg viewBox="0 0 24 24" {...ICON}>
      <path d="M9 6l6 6-6 6" />
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
      <path d="M4 8h10M18 8h2M4 16h2M10 16h10" />
      <circle cx="16" cy="8" r="2" />
      <circle cx="8" cy="16" r="2" />
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
      <circle cx="11" cy="11" r="6" />
      <path d="M16 16l4 4" />
    </svg>
  );
}

/**
 * H5 底部控制栏：与顶部 header 同属控制层，一起显隐。
 * 上一章 / 目录 / 书签 / 搜索 / 书架 / 设置 / 下一章，每个可点区域 ≥48px；
 * 首末章与正文未载入时按钮置灰，原因文案常驻在按钮内第二行。
 */
export function ReaderBottomBar({
  hidden,
  prev,
  next,
  searchEnabled,
  onPrev,
  onNext,
  onOpenDrawer,
  onGoShelf,
  onOpenBookmarks,
  onToggleSearch,
  onToggleSettings,
  settingsOpen,
  drawerOpen,
  bookmarksOpen,
  searchOpen,
}: {
  hidden: boolean;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  searchEnabled: boolean;
  onPrev: () => void;
  onNext: () => void;
  onOpenDrawer: () => void;
  onGoShelf: () => void;
  onOpenBookmarks: () => void;
  onToggleSearch: () => void;
  onToggleSettings: () => void;
  settingsOpen: boolean;
  drawerOpen: boolean;
  bookmarksOpen: boolean;
  searchOpen: boolean;
}) {
  return (
    <nav className="h5-reader-bar" data-testid="reader-bar" aria-label="阅读控制" aria-hidden={hidden}>
      <button type="button" aria-label="上一章" disabled={!prev} data-od-id="C-17" onClick={onPrev}>
        <IconPrev />
        <span>上一章</span>
        {!prev ? <span className="inline-note">已经是第一章了</span> : null}
      </button>
      <button type="button" aria-label="章节目录" aria-expanded={drawerOpen} data-od-id="C-18" onClick={onOpenDrawer}>
        <IconMenu />
        <span>目录</span>
      </button>
      <button type="button" aria-label="打开书签" aria-expanded={bookmarksOpen} data-od-id="C-19" onClick={onOpenBookmarks}>
        <IconBookmark />
        <span>书签</span>
      </button>
      <button
        type="button"
        aria-label="搜索本章"
        aria-expanded={searchOpen}
        disabled={!searchEnabled}
        data-od-id="C-20"
        onClick={onToggleSearch}
      >
        <IconSearch />
        <span>搜索</span>
        {!searchEnabled ? <span className="inline-note">正文还没载入，稍后可以搜索</span> : null}
      </button>
      <button type="button" aria-label="返回书架" data-od-id="C-21" onClick={onGoShelf}>
        <IconShelf />
        <span>书架</span>
      </button>
      <button type="button" aria-label="阅读设置" aria-expanded={settingsOpen} data-od-id="C-22" onClick={onToggleSettings}>
        <IconSettings />
        <span>设置</span>
      </button>
      <button type="button" aria-label="下一章" disabled={!next} data-od-id="C-23" onClick={onNext}>
        <IconNext />
        <span>下一章</span>
        {!next ? <span className="inline-note">已经是最后一章了</span> : null}
      </button>
    </nav>
  );
}
