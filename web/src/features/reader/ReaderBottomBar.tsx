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

/**
 * H5 底部翻章栏：与顶部 header 同属控制层，一起显隐。
 * 只做「上一章 / 目录 / 设置 / 下一章」，每个可点区域 ≥48px。
 */
export function ReaderBottomBar({
  hidden,
  prev,
  next,
  onPrev,
  onNext,
  onOpenDrawer,
  onToggleSettings,
  settingsOpen,
  drawerOpen,
}: {
  hidden: boolean;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  onPrev: () => void;
  onNext: () => void;
  onOpenDrawer: () => void;
  onToggleSettings: () => void;
  settingsOpen: boolean;
  drawerOpen: boolean;
}) {
  return (
    <nav className="h5-reader-bar" data-testid="reader-bar" aria-label="阅读控制" aria-hidden={hidden}>
      <button type="button" aria-label="上一章" disabled={!prev} onClick={onPrev}>
        <IconPrev />
        <span>上一章</span>
      </button>
      <button type="button" aria-label="章节目录" aria-expanded={drawerOpen} onClick={onOpenDrawer}>
        <IconMenu />
        <span>目录</span>
      </button>
      <button type="button" aria-label="阅读设置" aria-expanded={settingsOpen} onClick={onToggleSettings}>
        <IconSettings />
        <span>设置</span>
      </button>
      <button type="button" aria-label="下一章" disabled={!next} onClick={onNext}>
        <IconNext />
        <span>下一章</span>
      </button>
    </nav>
  );
}
