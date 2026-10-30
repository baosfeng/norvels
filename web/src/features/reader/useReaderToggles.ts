import { useCallback } from "react";
import type { ReaderDrawerTab } from "./readerConstants";

/**
 * 顶栏 / 底栏 / 抽屉共用的「开或关」动作：同一个按钮再点一次就关。
 * 抽出来是为了让三处调用的语义一致——搜索、设置、目录、书签都按「点开再点关」处理，
 * 避免某处写成「只能开不能关」这种走查才会发现的差异。
 */
export function useReaderToggles({
  searchOpen,
  settingsOpen,
  drawerOpen,
  onOpenSearch,
  onCloseSearch,
  onOpenSettings,
  onCloseSettings,
  onOpenDrawer,
  onCloseDrawer,
}: {
  searchOpen: boolean;
  settingsOpen: boolean;
  drawerOpen: boolean;
  onOpenSearch: () => void;
  onCloseSearch: () => void;
  onOpenSettings: () => void;
  onCloseSettings: () => void;
  onOpenDrawer: (tab: ReaderDrawerTab) => void;
  onCloseDrawer: () => void;
}) {
  const toggleSearch = useCallback(() => {
    if (searchOpen) onCloseSearch();
    else onOpenSearch();
  }, [searchOpen, onOpenSearch, onCloseSearch]);

  const toggleSettings = useCallback(() => {
    if (settingsOpen) onCloseSettings();
    else onOpenSettings();
  }, [settingsOpen, onOpenSettings, onCloseSettings]);

  /** 目录 / 书签共用同一个抽屉：已经开在该页签就收起，否则切到该页签。 */
  const openDrawerOrClose = useCallback(
    (tab: ReaderDrawerTab) => {
      if (drawerOpen) onCloseDrawer();
      else onOpenDrawer(tab);
    },
    [drawerOpen, onOpenDrawer, onCloseDrawer],
  );

  const toggleDirectory = useCallback(() => openDrawerOrClose("toc"), [openDrawerOrClose]);
  const toggleBookmarks = useCallback(() => openDrawerOrClose("bookmarks"), [openDrawerOrClose]);

  return { toggleSearch, toggleSettings, toggleDirectory, toggleBookmarks };
}
