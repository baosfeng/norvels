import { useCallback, useEffect } from "react";
import type { ChapterBrief } from "../../api/types";
import type { TurnDir } from "../../lib/paging/turnGesture";

/**
 * 阅读器键盘：Esc 关面板 / 取消删除，「?」开帮助，空格与 PageUp/Down 滚屏；
 * 翻页模式下左右方向键翻一页、Home/End 到本章首末页，空格与 PageDown 也翻页。
 * 输入框内的按键一律放过，避免和搜索、设置里的输入打架。
 *
 * 分页与滚动的键位不共用一套出口：分页有「页」这一层，左右键必然先落到页上，
 * 只有章末页再按才会跨章（由分页引擎的 onEdge 决定）。所以这里按 paging 分流，
 * 滚动模式下的行为一字未改。
 */
export function useReaderKeyboard({
  deleteTarget,
  onCancelDelete,
  onClosePanels,
  closeSearch,
  toggleHelp,
  helpOpen,
  searchOpen,
  panelsOpen,
  controlsVisible,
  onHideControls,
  prev,
  next,
  goChapterAnnounce,
  paging,
}: {
  deleteTarget: unknown;
  onCancelDelete: () => void;
  onClosePanels: () => void;
  closeSearch: () => void;
  toggleHelp: () => void;
  helpOpen: boolean;
  searchOpen: boolean;
  /** 目录 / 设置 / 帮助 / 搜索任一开着：Esc 先关面板，不动控制层。 */
  panelsOpen: boolean;
  /** H5 控制层是否展开：没有面板时 Esc 收起它。 */
  controlsVisible: boolean;
  onHideControls: () => void;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  goChapterAnnounce: (target: ChapterBrief | null) => void;
  /** 分页模式下的键盘出口；为 null 时走原来的翻章 / 滚屏。 */
  paging?: {
    turn: (dir: TurnDir) => void;
    first: () => void;
    last: () => void;
  } | null;
}) {
  const scrollByScreen = useCallback((direction: 1 | -1) => {
    window.scrollBy({ top: window.innerHeight * 0.85 * direction, behavior: "smooth" });
  }, []);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      // 顺序固定：先关面板，没有面板时才收起控制层——一次 Esc 只做一件事
      if (event.key === "Escape") {
        if (deleteTarget) {
          onCancelDelete();
          return;
        }
        if (panelsOpen) {
          onClosePanels();
          closeSearch();
          return;
        }
        if (controlsVisible) onHideControls();
        return;
      }
      const target = event.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) return;
      if (event.key === "?" || (event.key === "/" && event.shiftKey)) {
        event.preventDefault();
        toggleHelp();
        return;
      }
      if (helpOpen) return;
      if (searchOpen) return;
      if (paging) {
        if (event.key === "ArrowLeft" || event.key === "PageUp") {
          event.preventDefault();
          paging.turn(-1);
        } else if (event.key === "ArrowRight" || event.key === "PageDown" || event.key === " ") {
          event.preventDefault();
          paging.turn(1);
        } else if (event.key === "Home") {
          event.preventDefault();
          paging.first();
        } else if (event.key === "End") {
          event.preventDefault();
          paging.last();
        }
        return;
      }
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        goChapterAnnounce(prev);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        goChapterAnnounce(next);
      } else if (event.key === " " || event.key === "PageDown") {
        event.preventDefault();
        scrollByScreen(event.shiftKey ? -1 : 1);
      } else if (event.key === "PageUp") {
        event.preventDefault();
        scrollByScreen(-1);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [goChapterAnnounce, prev, next, searchOpen, helpOpen, closeSearch, scrollByScreen, deleteTarget, toggleHelp, onCancelDelete, onClosePanels, panelsOpen, controlsVisible, onHideControls, paging]);

  return { scrollByScreen };
}
