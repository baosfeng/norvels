import { useCallback, useEffect, useRef, useState } from "react";

/**
 * 阅读器与整站共用的主题。
 * 六档显式主题（日间 / 夜间 / 羊皮纸 / 护眼绿 / 纯黑 OLED / 高对比）+ 一档「跟随系统」：
 * 跟随系统时按 prefers-color-scheme 在日间与夜间之间自动切换，并持续监听系统外观变化。
 *
 * 状态栏颜色（theme-color meta）与系统高对比偏好都在这里收口：
 * 前者从 CSS 变量读、不在 TS 里存第二份色表，后者只当「没选过时的缺省值」，不覆盖读者的选择。
 */
export type Theme = "day" | "night" | "parchment" | "green" | "black" | "contrast" | "system";
/** 真正落到 data-theme 上的主题（system 已解析成 day / night）。 */
export type ResolvedTheme = Exclude<Theme, "system">;

const KEY = "norvels.theme";
/** 面板里的展示顺序：由浅到深，跟随系统收在最后。 */
const ORDER: Theme[] = ["day", "night", "parchment", "green", "black", "contrast", "system"];
/** 深色外观的档位：color-scheme 与浏览器原生控件（滚动条 / 输入框）跟着走。 */
const DARK_THEMES: readonly ResolvedTheme[] = ["night", "black"];
const DARK_QUERY = "(prefers-color-scheme: dark)";
const CONTRAST_QUERY = "(prefers-contrast: more)";

/** 这一档是不是深色外观（决定 color-scheme，避免纯黑档里出现白底输入框）。 */
export function isDarkTheme(theme: ResolvedTheme): boolean {
  return DARK_THEMES.includes(theme);
}

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

/**
 * 读者有没有**手动选过**主题。
 * readStoredTheme 把「没存过」与「存了个坏值」都归到 day，光看返回值分不出这两种情况，
 * 而「系统偏好能不能当缺省值」恰好只取决于这一件事：选过就一律不覆盖。
 */
export function hasStoredTheme(storage?: Pick<Storage, "getItem">): boolean {
  try {
    return isTheme(storage?.getItem(KEY));
  } catch {
    return false;
  }
}

/**
 * 首帧主题：读者选过就用他选的（系统偏好无权覆盖）；
 * 没选过才轮到系统偏好——系统要求高对比就落到 contrast，否则维持日间。
 */
export function initialTheme(
  storage?: Pick<Storage, "getItem">,
  contrast: boolean = prefersContrast(),
): Theme {
  const stored = readStoredTheme(storage);
  if (hasStoredTheme(storage)) return stored;
  return contrast ? "contrast" : stored;
}

/** 系统当前是否深色。没有 matchMedia（SSR / 老浏览器）时按浅色处理。 */
export function prefersDark(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  return window.matchMedia(DARK_QUERY).matches;
}

/** 「跟随系统」在这里落地：显式主题原样返回，system 按系统外观解析成 day / night。 */
export function resolveTheme(theme: Theme, dark: boolean = prefersDark()): ResolvedTheme {
  if (theme === "system") return dark ? "night" : "day";
  return theme;
}

/** 主题切换的过渡时长，与 motion.css 里的 --motion-theme 保持一致。 */
export const THEME_SHIFT_MS = 180;

/** 是否开启「减少动效」：开着就不加过渡 class。 */
export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** 系统是否要求高对比（prefers-contrast: more）。 */
export function prefersContrast(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia(CONTRAST_QUERY).matches;
}

/**
 * 读根节点的主题底色（tokens.css 的 --r-bg）。
 * 色值的唯一真值在样式表：在 TS 里再抄一份七档色表，换主题时两处必然漂移。
 */
export function readThemeBackground(root: HTMLElement): string {
  if (typeof getComputedStyle !== "function") return "";
  try {
    return getComputedStyle(root).getPropertyValue("--r-bg").trim();
  } catch {
    return "";
  }
}

/** 拿到 <meta name="theme-color">；老页面 / 被删掉时补一个，避免状态栏颜色无人接管。 */
function themeColorMeta(doc: Document): HTMLMetaElement | null {
  const existing = doc.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
  if (existing) return existing;
  if (!doc.head) return null;
  const meta = doc.createElement("meta");
  meta.name = "theme-color";
  doc.head.appendChild(meta);
  return meta;
}

/**
 * 状态栏 / 任务栏颜色跟主题走。
 * dataset 刚写完、样式还没重算也没关系：computedStyle 会同步触发一次样式计算，
 * 同一帧就能读到新主题的 --r-bg；读不到（style 尚未加载）就保留 index.html 里的中性值。
 * manifest 的 theme_color 是静态的安装期兜底，运行时一律以这条 meta 为准。
 */
function syncThemeColorMeta(root: HTMLElement): void {
  const doc = root.ownerDocument;
  // 主题色是文档级状态：脱挂元素上的 applyTheme（部分单测）不该改 head
  if (!doc || doc.documentElement !== root) return;
  const color = readThemeBackground(root);
  if (!color) return;
  const meta = themeColorMeta(doc);
  if (meta && meta.content !== color) meta.content = color;
}

/**
 * 把主题写到根节点：data-theme 只放解析后的具体主题（样式表只认具体档位），
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
  root.style.colorScheme = isDarkTheme(resolved) ? "dark" : "light";
  // 必须在 dataset 之后：颜色要从这一档的样式里读出来
  syncThemeColorMeta(root);
}

/** 监听一条媒体查询的变化；回调只在变化时触发，卸载时摘掉。 */
function watchMedia(query: string, onChange: (matches: boolean) => void): () => void {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return () => {};
  const list = window.matchMedia(query);
  const handler = (event: MediaQueryListEvent) => onChange(event.matches);
  if (typeof list.addEventListener === "function") {
    list.addEventListener("change", handler);
    return () => list.removeEventListener("change", handler);
  }
  // 老 Safari 只有 addListener
  list.addListener(handler);
  return () => list.removeListener(handler);
}

/** 监听系统深浅色变化。 */
function watchSystemDark(onChange: (dark: boolean) => void): () => void {
  return watchMedia(DARK_QUERY, onChange);
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(() =>
    typeof window === "undefined" ? "day" : initialTheme(window.localStorage),
  );
  const [dark, setDark] = useState<boolean>(() => prefersDark());
  /** 读者在本实例里手动改过主题：之后系统偏好不再改他的选择。 */
  const chosen = useRef(false);
  /**
   * 挂载时就已有显式选择，等于读者选过了——哪怕这份记录是同一页面里另一份 useTheme 写的。
   * 同一页面上 App 根与阅读器各有一份 useTheme，只认自己的 ref 就会互相顶掉对方的选择。
   */
  const lockedAtMount = useRef<boolean | null>(null);
  if (lockedAtMount.current === null) {
    lockedAtMount.current = typeof window !== "undefined" && hasStoredTheme(window.localStorage);
  }

  // 系统外观一直在监听：显式主题下多算一次无害，切到「跟随系统」时立即用的是最新值
  useEffect(() => watchSystemDark(setDark), []);

  // 系统要求高对比时补一步「升级」：只升不降，且从不覆盖读者手动选过的主题
  // （手动选过 = 本实例里改过，或挂载时 storage 里就已经有选择）。
  useEffect(
    () =>
      watchMedia(CONTRAST_QUERY, (more) => {
        if (!more || chosen.current || lockedAtMount.current) return;
        setThemeState("contrast");
      }),
    [],
  );

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
    chosen.current = true;
    if (typeof document !== "undefined" && mounted.current && !prefersReducedMotion()) {
      const root = document.documentElement;
      root.classList.add("theme-shifting");
      window.setTimeout(() => root.classList.remove("theme-shifting"), THEME_SHIFT_MS + 20);
    }
    setThemeState(next);
  }, []);

  return { theme, resolvedTheme: resolveTheme(theme, dark), setTheme, themes: ORDER };
}
