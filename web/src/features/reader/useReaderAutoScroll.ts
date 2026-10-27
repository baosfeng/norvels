import { useEffect, useRef, useState } from "react";
import type { ChapterBrief } from "../../api/types";
import { createAutoScroll, isAtBottom } from "../../lib/autoScroll";
import { showToast } from "../../lib/toastStore";
import type { ReaderSettings } from "./readerSettings";

/**
 * 自动滚动：运行条件（开关开 + 未暂停 + 面板没开 + 窗口可见）、滚到底后的
 * 自动翻章或停下、以及切后台自动暂停。
 */
export function useReaderAutoScroll({
  settings,
  updateSettings,
  overlayOpen,
  next,
  index,
  goChapter,
  isH5,
}: {
  settings: ReaderSettings;
  updateSettings: (patch: Partial<ReaderSettings>) => void;
  overlayOpen: boolean;
  next: ChapterBrief | null;
  index: number;
  goChapter: (target: ChapterBrief | null) => void;
  isH5: boolean;
}) {
  const [autoPaused, setAutoPaused] = useState(false);
  const [windowActive, setWindowActive] = useState(true);

  useEffect(() => {
    const onVisibility = () => setWindowActive(document.visibilityState === "visible");
    const onBlur = () => setWindowActive(false);
    const onFocus = () => setWindowActive(true);
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("blur", onBlur);
    window.addEventListener("focus", onFocus);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("blur", onBlur);
      window.removeEventListener("focus", onFocus);
    };
  }, []);

  const autoRunning = settings.autoScroll && !autoPaused && !overlayOpen && windowActive;
  const autoBottomRef = useRef<() => void>(() => {});
  useEffect(() => {
    autoBottomRef.current = () => {
      if (settings.autoScrollNext && next) {
        showToast(`已自动翻到第 ${index + 2} 章`);
        goChapter(next);
      } else {
        updateSettings({ autoScroll: false });
        showToast("已滚到章末，自动滚动暂停");
      }
    };
  });

  useEffect(() => {
    if (!autoRunning) return;
    const controller = createAutoScroll(
      {
        scrollBy: (deltaY) => window.scrollBy({ top: deltaY }),
        atBottom: () =>
          isAtBottom({
            scrollTop: window.scrollY || document.documentElement.scrollTop || 0,
            clientHeight: window.innerHeight || 0,
            scrollHeight: document.documentElement.scrollHeight || 0,
          }),
        onBottom: () => autoBottomRef.current(),
      },
      settings.autoScrollSpeed,
    );
    controller.start();
    return () => controller.stop();
  }, [autoRunning, settings.autoScrollSpeed]);

  function exitAutoScroll() {
    setAutoPaused(false);
    updateSettings({ autoScroll: false });
    showToast("已关闭自动滚动");
    window.setTimeout(() => {
      document.querySelector<HTMLElement>(`[data-od-id="${isH5 ? "C-22" : "C-08"}"]`)?.focus();
    }, 0);
  }

  return { autoRunning, autoPaused, windowActive, setAutoPaused, exitAutoScroll };
}
