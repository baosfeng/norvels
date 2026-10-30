import { useRef, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { ApiError } from "../../api/client";
import type { Bookmark, Volume } from "../../api/types";
import { useFocusTrap } from "../../lib/focusTrap";
import type { ResourceState } from "../../lib/useResource";
import { BookmarksPanel } from "./BookmarksPanel";
import { DRAWER_TABS, type ReaderDrawerTab } from "./readerConstants";
import { TocPanel } from "./TocPanel";

/**
 * 阅读器的目录 / 书签合并抽屉：顶部两个页签，切换不关抽屉。
 * 外壳负责焦点陷阱与页签键盘（←/→ 切换）；两页内容在同一「本次打开」内各自保留浏览状态，
 * 关掉再开会重新挂载，因此每次都从干净状态开始（与合并前的重置口径一致）。
 */
export function ReaderDrawer({
  open,
  tab,
  onTabChange,
  onClose,
  tocLoading,
  tocError,
  volumes,
  bookId,
  currentChapterId,
  onTocRetry,
  bookmarks,
  addingBookmark,
  retryingBookmarks,
  canAddBookmark,
  onAddBookmark,
  onJumpBookmark,
  onRequestDeleteBookmark,
  onBookmarksRetry,
}: {
  open: boolean;
  tab: ReaderDrawerTab;
  onTabChange: (tab: ReaderDrawerTab) => void;
  onClose: () => void;
  tocLoading: boolean;
  tocError: ApiError | null;
  volumes: Volume[];
  bookId: number;
  currentChapterId: number;
  onTocRetry: () => void;
  bookmarks: ResourceState<{ bookmarks: Bookmark[] }>;
  addingBookmark: boolean;
  retryingBookmarks: boolean;
  canAddBookmark: boolean;
  onAddBookmark: () => void;
  onJumpBookmark: (bookmark: Bookmark) => void;
  onRequestDeleteBookmark: (bookmark: Bookmark) => void;
  onBookmarksRetry: () => void;
}) {
  const rootRef = useRef<HTMLElement | null>(null);
  useFocusTrap(open, rootRef, onClose);

  const bookmarkCount = bookmarks.data?.bookmarks.length ?? 0;
  const chapterCount = volumes.reduce((sum, volume) => sum + volume.chapters.length, 0);
  const at = DRAWER_TABS.findIndex((item) => item.id === tab);

  const onTabKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const delta = event.key === "ArrowRight" ? 1 : -1;
    const next = DRAWER_TABS[(at + delta + DRAWER_TABS.length) % DRAWER_TABS.length];
    onTabChange(next.id);
    rootRef.current?.querySelector<HTMLElement>(`[data-drawer-tab="${next.id}"]`)?.focus();
  };

  const meta =
    tab === "toc" ? `${chapterCount} 章 · ${volumes.length} 卷` : bookmarkCount > 0 ? `${bookmarkCount} 条` : "";

  return (
    <aside
      ref={rootRef}
      className={`drawer reader-drawer${open ? " is-open" : ""}`}
      aria-label="目录与书签"
      aria-hidden={!open}
      data-testid="reader-drawer"
      data-od-id="reader-drawer"
    >
      <div className="drawer-head">
        <div className="drawer-tabs" role="tablist" aria-label="目录与书签" onKeyDown={onTabKeyDown}>
          {DRAWER_TABS.map((item) => (
            <button
              key={item.id}
              type="button"
              role="tab"
              id={`reader-drawer-tab-${item.id}`}
              className={`drawer-tab${item.id === tab ? " is-active" : ""}`}
              aria-selected={item.id === tab}
              aria-controls={`reader-drawer-panel-${item.id}`}
              tabIndex={item.id === tab ? 0 : -1}
              data-testid={`drawer-tab-${item.id}`}
              data-drawer-tab={item.id}
              onClick={() => onTabChange(item.id)}
            >
              {item.label}
              {item.id === "bookmarks" && bookmarkCount > 0 ? (
                <span className="drawer-tab-count num">{bookmarkCount}</span>
              ) : null}
            </button>
          ))}
        </div>
        <span className="meta" data-testid="drawer-meta">
          {meta}
        </span>
        <button className="btn btn-ghost" type="button" data-od-id="C-30" onClick={onClose} aria-label="关闭目录与书签">
          关闭
        </button>
      </div>

      {open ? (
        <>
          <div
            className="drawer-body"
            id="reader-drawer-panel-toc"
            role="tabpanel"
            aria-labelledby="reader-drawer-tab-toc"
            hidden={tab !== "toc"}
          >
            <TocPanel
              active={tab === "toc"}
              loading={tocLoading}
              error={tocError}
              volumes={volumes}
              bookId={bookId}
              currentChapterId={currentChapterId}
              onNavigate={onClose}
              onRetry={onTocRetry}
            />
          </div>
          <div
            className="drawer-body"
            id="reader-drawer-panel-bookmarks"
            role="tabpanel"
            aria-labelledby="reader-drawer-tab-bookmarks"
            hidden={tab !== "bookmarks"}
          >
            <BookmarksPanel
              bookmarks={bookmarks}
              currentChapterId={currentChapterId}
              adding={addingBookmark}
              retrying={retryingBookmarks}
              canAdd={canAddBookmark}
              onAdd={onAddBookmark}
              onJump={onJumpBookmark}
              onRequestDelete={onRequestDeleteBookmark}
              onRetry={onBookmarksRetry}
            />
          </div>
        </>
      ) : null}
    </aside>
  );
}
