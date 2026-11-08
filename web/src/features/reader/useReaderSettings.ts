import { useCallback, useEffect, useRef, useState } from "react";
import { useTheme, type Theme } from "../../lib/theme";
import { showToast } from "../../lib/toastStore";
import { tradToSimp } from "../../lib/trad2simp";
import { api } from "../../api/client";
import { readReaderSettings, writeReaderSettings, type ReaderSettings } from "./readerSettings";

/**
 * 阅读设置的状态与落盘：默认值从 localStorage 读一次，改一次写一次。
 *
 * 多端同步：启动时拉取服务端按账号存储的设置，远程优先但保留本地兜底（合并）；
 * 修改时防抖写回服务端；离线/请求失败时继续用 localStorage，不影响阅读。
 */
export function useReaderSettings(paragraphs: string[]) {
  const [settings, setSettings] = useState<ReaderSettings>(() => readReaderSettings());
  const { theme, setTheme, themes } = useTheme();

  // 挂载时拉取远程设置并合并（远程优先、缺失键保留本地）。
  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const res = await api.readerSettings();
        const remote = res?.settings;
        if (!active || !remote || typeof remote !== "object") return;
        setSettings((prev) => ({ ...prev, ...(remote as Partial<ReaderSettings>) }));
      } catch {
        // 离线或未登录：继续用本地设置。
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  // 本地落盘（每次变更）。
  useEffect(() => {
    writeReaderSettings(settings);
  }, [settings]);

  // 远程写回（防抖），避免每次击键都请求。
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      void api.putReaderSettings(settings).catch(() => {
        // 离线：本地已落盘，下次启动再同步。
      });
    }, 800);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
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
