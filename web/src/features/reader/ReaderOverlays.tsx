import type { ApiError } from "../../api/client";
import type { Bookmark, Volume } from "../../api/types";
import type { Theme } from "../../lib/theme";
import type { ResourceState } from "../../lib/useResource";
import { BookmarkConfirmDialog } from "./BookmarkConfirmDialog";
import { ReaderDrawer } from "./ReaderDrawer";
import type { ReaderDrawerTab } from "./readerConstants";
import { ReaderSettingsPanel } from "./ReaderSettingsPanel";
import type { ReaderSettings } from "./readerSettings";

/**
 * 阅读器的全部浮层：遮罩、设置面板、目录/书签抽屉、删除确认框与页边提示。
 * 抽出来是因为这些层与正文互不依赖，放在 ReaderPage 里只会把「正文怎么读」淹没在面板参数里。
 * 遮罩的点击语义集中在这里：有确认框时先关确认框，否则关面板。
 */
export interface ReaderOverlaysProps {
  isH5: boolean;
  settingsOpen: boolean;
  drawerOpen: boolean;
  drawerTab: ReaderDrawerTab;
  deleteTarget: Bookmark | null;
  deleting: boolean;
  settings: ReaderSettings;
  theme: Theme;
  themes: Theme[];
  autoScrollAvailable: boolean;
  volumes: Volume[];
  bookId: number;
  currentChapterId: number;
  tocLoading: boolean;
  tocError: ApiError | null;
  bookmarks: ResourceState<{ bookmarks: Bookmark[] }>;
  addingBookmark: boolean;
  retryingBookmarks: boolean;
  canAddBookmark: boolean;
  onClosePanels: () => void;
  onCancelDelete: () => void;
  onConfirmDelete: () => void;
  onChangeSettings: (patch: Partial<ReaderSettings>) => void;
  onSetTheme: (theme: Theme) => void;
  onTabChange: (tab: ReaderDrawerTab) => void;
  onTocRetry: () => void;
  onAddBookmark: () => void;
  onJumpBookmark: (bookmark: Bookmark) => void;
  onRequestDeleteBookmark: (bookmark: Bookmark) => void;
  onBookmarksRetry: () => void;
}

export function ReaderOverlays(props: ReaderOverlaysProps) {
  const scrimOpen = props.drawerOpen || props.settingsOpen || Boolean(props.deleteTarget);
  return (
    <>
      <ReaderSettingsPanel
        open={props.settingsOpen}
        isH5={props.isH5}
        autoScrollAvailable={props.autoScrollAvailable}
        settings={props.settings}
        onChange={props.onChangeSettings}
        theme={props.theme}
        themes={props.themes}
        setTheme={props.onSetTheme}
        onClose={props.onClosePanels}
      />

      <button
        className={`overlay${scrimOpen ? " is-open" : ""}`}
        type="button"
        aria-label="关闭当前面板"
        tabIndex={scrimOpen ? 0 : -1}
        onClick={props.deleteTarget ? props.onCancelDelete : props.onClosePanels}
      />

      <ReaderDrawer
        open={props.drawerOpen}
        isH5={props.isH5}
        tab={props.drawerTab}
        onTabChange={props.onTabChange}
        onClose={props.onClosePanels}
        tocLoading={props.tocLoading}
        tocError={props.tocError}
        volumes={props.volumes}
        bookId={props.bookId}
        currentChapterId={props.currentChapterId}
        onTocRetry={props.onTocRetry}
        bookmarks={props.bookmarks}
        addingBookmark={props.addingBookmark}
        retryingBookmarks={props.retryingBookmarks}
        canAddBookmark={props.canAddBookmark}
        onAddBookmark={props.onAddBookmark}
        onJumpBookmark={props.onJumpBookmark}
        onRequestDeleteBookmark={props.onRequestDeleteBookmark}
        onBookmarksRetry={props.onBookmarksRetry}
      />

      <BookmarkConfirmDialog
        open={Boolean(props.deleteTarget)}
        bookmark={props.deleteTarget}
        deleting={props.deleting}
        onCancel={props.onCancelDelete}
        onConfirm={props.onConfirmDelete}
      />

    </>
  );
}
