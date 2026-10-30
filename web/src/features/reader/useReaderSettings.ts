import { useCallback, useEffect, useState } from "react";
import { useTheme, type Theme } from "../../lib/theme";
import { showToast } from "../../lib/toastStore";
import { tradToSimp } from "../../lib/trad2simp";
import { readReaderSettings, writeReaderSettings, type ReaderSettings } from "./readerSettings";

/**
 * 阅读设置的状态与落盘：默认值从 localStorage 读一次，改一次写一次。
 * 两个开关需要即时反馈（繁转简换了字形、自动滚动开始动起来），所以提示也在这里发；
 * 「跟随系统外观」同理——它不只是换配色，还会跟着系统变深变浅。
 */
export function useReaderSettings(paragraphs: string[]) {
  const [settings, setSettings] = useState<ReaderSettings>(() => readReaderSettings());
  const { theme, setTheme, themes } = useTheme();

  useEffect(() => {
    writeReaderSettings(settings);
  }, [settings]);

  const changeSettings = useCallback(
    (patch: Partial<ReaderSettings>) => {
      if (patch.script === true && paragraphs.some((para) => tradToSimp(para) !== para)) {
        showToast("已转换为简体，位置不变");
      }
      if (patch.autoScroll === true) showToast("已开始自动滚动");
      setSettings((prev) => ({ ...prev, ...patch }));
    },
    [paragraphs],
  );

  const changeTheme = useCallback(
    (value: Theme) => {
      setTheme(value);
      if (value === "system") showToast("已跟随系统外观");
    },
    [setTheme],
  );

  return { settings, changeSettings, theme, themes, changeTheme };
}
