import { useCallback, useEffect, useRef, useState } from "react";

/**
 * 阅读器与整站共用的主题。
 * 三档显式主题（日间 / 夜间 / 羊皮纸）+ 一档「跟随系统」：跟随系统时按 prefers-color-scheme
 * 在日间与夜间之间自动切换，并持续监听系统外观变化。
 */
export type Theme = "day" | "night" | "parchment" | "system";
/** 真正落到 data-theme 上的主题（system 已解析成 day / night）。 */
export type ResolvedTheme = Exclude<Theme, "system">;

const KEY = "norvels.theme";
const ORDER: Theme[] = ["day", "night", "parchment", "system"];

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

/** 系统当前是否深色。没有 matchMedia（SSR / 老浏览器）时按浅色处理。 */
export function prefersDark(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

/** 「跟随系统」在这里落地：显式主题原样返回，system 按系统外观解析成 day / night。 */
export function resolveTheme(theme: Theme, dark: boolean = prefersDark()): ResolvedTheme {
  if (theme === "system") return dark ? "night" : "day";
  return theme;
}

/**
 * 把主题写到根节点：data-theme 只放解析后的具体主题（样式表只认三档），
 * 读者的「跟随系统」选择单独记在 data-theme-preference 上，便于排查与样式分支。
 */
export function applyTheme(
  theme: Theme,
  root: HTMLElement | null = typeof document === "undefined" ? null : document.documentElement,
  dark: boolean = prefersDark(),
): void {
  if (!root) return;
  const resolved = resolveTheme(theme, dark);
  root.dataset.theme = resolved;
  root.dataset.themePreference = theme;
  root.style.colorScheme = resolved === "night" ? "dark" : "light";
}

/** 主题切换的过渡时长，与 motion.css 里的 --motion-theme 保持一致。 */
export const THEME_SHIFT_MS = 180;

/** 是否开启「减少动效」：开着就不加过渡 class。 */
export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** 监听系统深浅色变化；回调只在变化时触发，卸载时摘掉。 */
function watchSystemDark(onChange: (dark: boolean) => void): () => void {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return () => {};
  const query = window.matchMedia("(prefers-color-scheme: dark)");
  const handler = (event: MediaQueryListEvent) => onChange(event.matches);
  if (typeof query.addEventListener === "function") {
    query.addEventListener("change", handler);
    return () => query.removeEventListener("change", handler);
  }
  // 老 Safari 只有 addListener
  query.addListener(handler);
  return () => query.removeListener(handler);
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(() =>
    typeof window === "undefined" ? "day" : readStoredTheme(window.localStorage),
  );
  const [dark, setDark] = useState<boolean>(() => prefersDark());

  // 系统外观一直在监听：显式主题下多算一次无害，切到「跟随系统」时立即用的是最新值
  useEffect(() => watchSystemDark(setDark), []);

  useEffect(() => {
    applyTheme(theme, undefined, dark);
    try {
      window.localStorage.setItem(KEY, theme);
    } catch {
      /* 隐私模式写入失败时只影响下次默认值，不影响本次显示 */
    }
  }, [theme, dark]);

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

  return { theme, resolvedTheme: resolveTheme(theme, dark), setTheme, themes: ORDER };
}
