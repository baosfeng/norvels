import { useCallback, useRef, useState } from "react";
import type { ReaderDrawerTab } from "./readerConstants";

/**
 * 阅读器的四个面板（目录 / 设置 / 书签 / 搜索 / 帮助）：
 * 同一时刻只留一个，打开新的会收起其它几个；搜索关闭还要复原滚动位置并清空查询。
 * 目录与书签共用一个抽屉，只是切到不同页签——从底栏点「书签」不会再关掉再开一次。
 */
export function useReaderPanels({
  canSearch,
  onResetSearch,
}: {
  canSearch: boolean;
  onResetSearch: () => void;
}) {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerTab, setDrawerTab] = useState<ReaderDrawerTab>("toc");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  // 搜索打开前的滚动位置：关闭搜索条时正文回到原处
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

  const openDrawer = useCallback(
    (tab: ReaderDrawerTab) => {
      setSettingsOpen(false);
      setDrawerTab(tab);
      setDrawerOpen(true);
    },
    [],
  );
  const openDirectory = useCallback(() => openDrawer("toc"), [openDrawer]);
  const openBookmarks = useCallback(() => openDrawer("bookmarks"), [openDrawer]);
  const closeDrawer = useCallback(() => setDrawerOpen(false), []);

  const openSettings = useCallback(() => {
    setDrawerOpen(false);
    closeSearch();
    setSettingsOpen(true);
  }, [closeSearch]);

  const toggleHelp = useCallback(() => {
    setDrawerOpen(false);
    setSettingsOpen(false);
    closeSearch();
    setHelpOpen((open) => !open);
  }, [closeSearch]);

  return {
    drawerOpen,
    drawerTab,
    settingsOpen,
    searchOpen,
    helpOpen,
    setDrawerOpen,
    setDrawerTab,
    setSettingsOpen,
    setSearchOpen,
    setHelpOpen,
    openDrawer,
    openDirectory,
    openBookmarks,
    closeDrawer,
    openSettings,
    openSearch,
    closeSearch,
    toggleHelp,
  };
}
