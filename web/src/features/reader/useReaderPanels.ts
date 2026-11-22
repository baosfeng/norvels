import { useCallback, useRef, useState } from "react";
import type { ReaderDrawerTab } from "./readerConstants";
import { pageScrollTop, readerSurface, setScrollTop, type Surface } from "./readerStreamLayout";

/** 搜索打开前的阅读位置：当时滚的是正文容器（H5）还是整页（PC），记下来才能原路回去。 */
interface SearchOrigin {
  surface: Surface | null;
  top: number;
}

/** 当前滚动位置：与 readerStreamLayout 同一口径（容器可滚用容器，否则用整页）。 */
function currentScrollTop(): number {
  const surface = readerSurface();
  return surface?.scrolls ? surface.el.scrollTop : pageScrollTop();
}

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
  /**
   * 搜索打开前的阅读位置；null = 这次没打开过搜索。
   *
   * 不能默认成 0：设置面板 / 帮助也在收起搜索，它们会把「没记过的 0」当成原处，
   * 于是只要打开设置面板（改字号必经此路），PC 滚动模式的页面就被拉到章首——
   * 实测 1280×800 下 y 1200 → 202，读者回来时已经不在读的那一段。
   */
  const searchOrigin = useRef<SearchOrigin | null>(null);

  const closeSearch = useCallback(() => {
    setSearchOpen(false);
    onResetSearch();
    const origin = searchOrigin.current;
    // 先清再复原：复原本身可能触发 scroll 事件回调，留着 origin 会被二次套用
    searchOrigin.current = null;
    if (!origin) return;
    // 同一根上回同一个位置：搜索期间命中会被 scrollIntoView 带走，这里把人送回来
    if (origin.surface) setScrollTop(origin.surface, origin.top);
    else window.scrollTo(0, origin.top);
  }, [onResetSearch]);

  const openSearch = useCallback(() => {
    if (!canSearch) return;
    // H5 正文容器自己滚时 window.scrollY 恒为 0，只记它等于没记
    searchOrigin.current = { surface: readerSurface(), top: currentScrollTop() };
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
