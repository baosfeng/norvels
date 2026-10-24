import { useCallback, useEffect, useState } from "react";

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

  const setTheme = useCallback((next: Theme) => setThemeState(next), []);
  return { theme, setTheme, themes: ORDER };
}
