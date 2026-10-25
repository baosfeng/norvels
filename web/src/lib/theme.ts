import { useCallback, useEffect, useRef, useState } from "react";

/** 阅读器与整站共用的三主题。日间是默认，选择存 localStorage。 */
export type Theme = "day" | "night" | "parchment";

const KEY = "norvels.theme";
const ORDER: Theme[] = ["day", "night", "parchment"];

export function isTheme(value: unknown): value is Theme {
  return typeof value === "string" && (ORDER as string[]).includes(value);
}

export function readStoredTheme(storage?: Pick<Storage, "getItem">): Theme {
  try {
    const raw = storage?.getItem(KEY);
    return isTheme(raw) ? raw : "day";
  } catch {
    return "day";
  }
}

export function applyTheme(theme: Theme, root: HTMLElement | null = typeof document === "undefined" ? null : document.documentElement): void {
  if (!root) return;
  root.dataset.theme = theme;
  root.style.colorScheme = theme === "night" ? "dark" : "light";
}

/** 主题切换的过渡时长，与 motion.css 里的 --motion-theme 保持一致。 */
export const THEME_SHIFT_MS = 180;

/** 是否开启「减少动效」：开着就不加过渡 class。 */
export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(() =>
    typeof window === "undefined" ? "day" : readStoredTheme(window.localStorage),
  );

  useEffect(() => {
    applyTheme(theme);
    try {
      window.localStorage.setItem(KEY, theme);
    } catch {
      /* 隐私模式写入失败时只影响下次默认值，不影响本次显示 */
    }
  }, [theme]);

  // 挂载完成即视为「首帧已过」：之后每一次用户切换都走 180ms 颜色过渡，
  // 而首屏那次（effect 里的 applyTheme）不加 class、不会闪。
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
  }, []);

  const setTheme = useCallback((next: Theme) => {
    if (typeof document !== "undefined" && mounted.current && !prefersReducedMotion()) {
      const root = document.documentElement;
      root.classList.add("theme-shifting");
      window.setTimeout(() => root.classList.remove("theme-shifting"), THEME_SHIFT_MS + 20);
    }
    setThemeState(next);
  }, []);
  return { theme, setTheme, themes: ORDER };
}