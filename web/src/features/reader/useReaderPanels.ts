import { useCallback, useRef, useState } from "react";

/**
 * 阅读器的五个面板（目录 / 设置 / 书签 / 搜索 / 帮助）：
 * 同一时刻只留一个，打开新的会收起其它几个；搜索关闭还要复原滚动位置并清空查询。
 */
export function useReaderPanels({
  canSearch,
  onResetSearch,
}: {
  canSearch: boolean;
  onResetSearch: () => void;
}) {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [bookmarksOpen, setBookmarksOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  // 搜索打开前的滚动位置：关闭搜索条时正文回到原处（契约 C-05 / C-37 / C-40）
  const searchOrigin = useRef(0);

  const closeSearch = useCallback(() => {
    setSearchOpen(false);
    onResetSearch();
    window.scrollTo({ top: searchOrigin.current });
  }, [onResetSearch]);

  const openSearch = useCallback(() => {
    if (!canSearch) return;
    searchOrigin.current = window.scrollY;
    setSearchOpen(true);
  }, [canSearch]);

  const openDirectory = useCallback(() => {
    setBookmarksOpen(false);
    setSettingsOpen(false);
    setDrawerOpen(true);
  }, []);
  const openBookmarks = useCallback(() => {
    setDrawerOpen(false);
    setSettingsOpen(false);
    setBookmarksOpen(true);
  }, []);
  const openSettings = useCallback(() => {
    setDrawerOpen(false);
    setBookmarksOpen(false);
    closeSearch();
    setSettingsOpen(true);
  }, [closeSearch]);
  const toggleHelp = useCallback(() => {
    setDrawerOpen(false);
    setBookmarksOpen(false);
    setSettingsOpen(false);
    closeSearch();
    setHelpOpen((open) => !open);
  }, [closeSearch]);

  return {
    drawerOpen,
    settingsOpen,
    bookmarksOpen,
    searchOpen,
    helpOpen,
    setDrawerOpen,
    setSettingsOpen,
    setBookmarksOpen,
    setSearchOpen,
    setHelpOpen,
    openDirectory,
    openBookmarks,
    openSettings,
    openSearch,
    closeSearch,
    toggleHelp,
  };
}
