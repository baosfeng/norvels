import { useCallback, useEffect } from "react";
import type { ChapterBrief } from "../../api/types";

/**
 * 阅读器键盘：Esc 关面板 / 取消删除，「?」开帮助，左右翻章，空格与 PageUp/Down 滚屏。
 * 输入框内的按键一律放过，避免和搜索、设置里的输入打架。
 */
export function useReaderKeyboard({
  deleteTarget,
  onCancelDelete,
  onClosePanels,
  closeSearch,
  toggleHelp,
  helpOpen,
  searchOpen,
  prev,
  next,
  goChapterAnnounce,
}: {
  deleteTarget: unknown;
  onCancelDelete: () => void;
  onClosePanels: () => void;
  closeSearch: () => void;
  toggleHelp: () => void;
  helpOpen: boolean;
  searchOpen: boolean;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  goChapterAnnounce: (target: ChapterBrief | null) => void;
}) {
  const scrollByScreen = useCallback((direction: 1 | -1) => {
    window.scrollBy({ top: window.innerHeight * 0.85 * direction, behavior: "smooth" });
  }, []);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.key === "Escape") {
        if (deleteTarget) {
          onCancelDelete();
          return;
        }
        onClosePanels();
        closeSearch();
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
  }, [goChapterAnnounce, prev, next, searchOpen, helpOpen, closeSearch, scrollByScreen, deleteTarget, toggleHelp, onCancelDelete, onClosePanels]);

  return { scrollByScreen };
}
