import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  applyTheme,
  hasStoredTheme,
  initialTheme,
  isTheme,
  prefersContrast,
  prefersDark,
  readStoredTheme,
  readThemeBackground,
  resolveTheme,
  useTheme,
} from "./theme";

const DARK = "(prefers-color-scheme: dark)";
const CONTRAST = "(prefers-contrast: more)";

/**
 * 可控的 matchMedia 桩：按媒体查询分别维护 matches 与监听者，
 * 能手动触发任意一条查询的 change（深浅色用 flip，高对比用 setContrast）。
 */
function stubMatchMedia(options: { dark?: boolean; contrast?: boolean } = {}) {
  const state = new Map<string, boolean>([
    [DARK, options.dark ?? false],
    [CONTRAST, options.contrast ?? false],
    ["(prefers-reduced-motion: reduce)", false],
  ]);
  const listeners = new Map<string, Set<(event: MediaQueryListEvent) => void>>();
  const listenersOf = (media: string) => {
    const set = listeners.get(media) ?? new Set<(event: MediaQueryListEvent) => void>();
    listeners.set(media, set);
    return set;
  };

  vi.stubGlobal("matchMedia", (media: string) => ({
    get matches() {
      return state.get(media) ?? false;
    },
    media,
    addEventListener: (_: string, handler: (event: MediaQueryListEvent) => void) => listenersOf(media).add(handler),
    removeEventListener: (_: string, handler: (event: MediaQueryListEvent) => void) => listenersOf(media).delete(handler),
    addListener: (handler: (event: MediaQueryListEvent) => void) => listenersOf(media).add(handler),
    removeListener: (handler: (event: MediaQueryListEvent) => void) => listenersOf(media).delete(handler),
    dispatchEvent: () => true,
  }));

  return {
    flip(dark: boolean) {
      state.set(DARK, dark);
      for (const handler of listenersOf(DARK)) handler({ matches: dark } as MediaQueryListEvent);
    },
    setContrast(more: boolean) {
      state.set(CONTRAST, more);
      for (const handler of listenersOf(CONTRAST)) handler({ matches: more } as MediaQueryListEvent);
    },
  };
}

describe("主题：五档与跟随系统", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    document.documentElement.removeAttribute("data-theme-preference");
    document.documentElement.className = "";
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("isTheme 认 system，仍不认瞎写的值", () => {
    expect(isTheme("system")).toBe(true);
    expect(isTheme("day")).toBe(true);
    expect(isTheme("dark")).toBe(false);
    expect(readStoredTheme({ getItem: () => "system" })).toBe("system");
  });

  it("resolveTheme：system 按系统外观落到日间 / 夜间，显式主题不受系统影响", () => {
    expect(resolveTheme("system", true)).toBe("night");
    expect(resolveTheme("system", false)).toBe("day");
    expect(resolveTheme("parchment", true)).toBe("parchment");
    expect(resolveTheme("day", true)).toBe("day");
  });

  it("applyTheme：data-theme 写解析后的主题，选择本身留在 data-theme-preference", () => {
    const el = document.createElement("div");
    applyTheme("system", el, true);
    expect(el.dataset.theme).toBe("night");
    expect(el.dataset.themePreference).toBe("system");
    expect(el.style.colorScheme).toBe("dark");

    applyTheme("system", el, false);
    expect(el.dataset.theme).toBe("day");
    expect(el.style.colorScheme).toBe("light");

    applyTheme("parchment", el, true);
    expect(el.dataset.theme).toBe("parchment");
    expect(el.dataset.themePreference).toBe("parchment");
  });

  it("prefersDark 读系统外观；没有 matchMedia 时按浅色", () => {
    stubMatchMedia({ dark: true });
    expect(prefersDark()).toBe(true);
    vi.unstubAllGlobals();
    vi.stubGlobal("matchMedia", undefined);
    expect(prefersDark()).toBe(false);
  });

  it("选「跟随系统」后系统切深色，页面跟着变，且存的是 system 而不是 night", () => {
    const media = stubMatchMedia({ dark: false });
    const { result } = renderHook(() => useTheme());

    act(() => result.current.setTheme("system"));
    expect(document.documentElement.dataset.theme).toBe("day");
    expect(result.current.resolvedTheme).toBe("day");
    // 存的选择是 system：刷新后仍然跟系统
    expect(localStorage.getItem("norvels.theme")).toBe("system");

    act(() => media.flip(true));
    expect(document.documentElement.dataset.theme).toBe("night");
    expect(result.current.resolvedTheme).toBe("night");
    expect(localStorage.getItem("norvels.theme")).toBe("system");

    act(() => media.flip(false));
    expect(document.documentElement.dataset.theme).toBe("day");
  });

  it("七档主题都在切换列表里，顺序为 日间/夜间/羊皮纸/护眼绿/纯黑/高对比/跟随系统", () => {
    stubMatchMedia();
    const { result } = renderHook(() => useTheme());
    expect(result.current.themes).toEqual(["day", "night", "parchment", "green", "black", "contrast", "system"]);
  });
});

describe("主题底色同步到 theme-color", () => {
  const STYLE_ID = "theme-color-test-tokens";
  let meta: HTMLMetaElement;

  beforeEach(() => {
    document.documentElement.removeAttribute("data-theme");
    // 色值留在样式表里（与 tokens.css 同一真值来源），TS 侧只负责读取
    const style = document.createElement("style");
    style.id = STYLE_ID;
    style.textContent =
      'html[data-theme="day"]{--r-bg:#fbf6ee}html[data-theme="night"]{--r-bg:#171514}html[data-theme="black"]{--r-bg:#000000}';
    document.head.appendChild(style);
    meta = document.createElement("meta");
    meta.name = "theme-color";
    meta.content = "#8a6a3b";
    document.head.appendChild(meta);
  });

  afterEach(() => {
    document.getElementById(STYLE_ID)?.remove();
    document.querySelectorAll('meta[name="theme-color"]').forEach((el) => el.remove());
  });

  it("切到夜间/纯黑时把 meta 换成该档的 --r-bg，不再停在羊皮纸色", () => {
    applyTheme("night", document.documentElement);
    expect(meta.content).toBe("#171514");
    applyTheme("black", document.documentElement);
    expect(meta.content).toBe("#000000");
    applyTheme("day", document.documentElement);
    expect(meta.content).toBe("#fbf6ee");
  });

  it("meta 不存在时补一个（老页面 / 被删掉的情况）", () => {
    meta.remove();
    applyTheme("night", document.documentElement);
    const created = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    expect(created).not.toBeNull();
    expect(created?.content).toBe("#171514");
  });

  it("读不到 --r-bg（样式尚未生效）时保留中性兜底，不写空值", () => {
    document.getElementById(STYLE_ID)?.remove();
    applyTheme("night", document.documentElement);
    expect(meta.content).toBe("#8a6a3b");
  });

  it("脱挂元素上调用 applyTheme 不动文档 head", () => {
    const el = document.createElement("div");
    applyTheme("night", el);
    expect(meta.content).toBe("#8a6a3b");
  });
});

describe("prefers-contrast 接入主题初值", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("没存过主题时，系统要求高对比就落到 contrast", () => {
    stubMatchMedia({ contrast: true });
    expect(prefersContrast()).toBe(true);
    expect(hasStoredTheme(localStorage)).toBe(false);
    expect(initialTheme(localStorage)).toBe("contrast");
  });

  it("系统没要求高对比时维持日间", () => {
    stubMatchMedia({ contrast: false });
    expect(initialTheme(localStorage)).toBe("day");
  });

  it("读者手动选过的主题不被系统高对比覆盖", () => {
    stubMatchMedia({ contrast: true });
    localStorage.setItem("norvels.theme", "night");
    expect(hasStoredTheme(localStorage)).toBe(true);
    expect(initialTheme(localStorage)).toBe("night");
  });

  it("存了个坏值等于没选过：仍按系统高对比落到 contrast", () => {
    stubMatchMedia({ contrast: true });
    localStorage.setItem("norvels.theme", "blue");
    expect(initialTheme(localStorage)).toBe("contrast");
  });

  it("useTheme 的初始 theme 与 data-theme 都跟着系统高对比", () => {
    stubMatchMedia({ contrast: true });
    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("contrast");
    expect(result.current.resolvedTheme).toBe("contrast");
    expect(document.documentElement.dataset.theme).toBe("contrast");
  });

  it("系统随后开启高对比：没手动选过就切到 contrast", () => {
    const media = stubMatchMedia({ contrast: false });
    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("day");
    act(() => media.setContrast(true));
    expect(result.current.theme).toBe("contrast");
    expect(document.documentElement.dataset.theme).toBe("contrast");
  });

  it("App 根与阅读器两份 useTheme 并存时，系统开高对比不顶掉读者选的主题", () => {
    const media = stubMatchMedia({ contrast: false });
    const appRoot = renderHook(() => useTheme());
    // 同一页面上 App 根与阅读器各有一份 useTheme（实例状态本就不同步，这里只关心别互相覆盖选择）
    const reader = renderHook(() => useTheme());

    act(() => appRoot.result.current.setTheme("green"));
    act(() => media.setContrast(true));

    // 阅读器那份没「自己手动选过」，但挂载时 storage 里已经有选择：它不该推 contrast 上去
    expect(appRoot.result.current.theme).toBe("green");
    expect(reader.result.current.theme).not.toBe("contrast");
    expect(localStorage.getItem("norvels.theme")).toBe("green");
  });

  it("读者手动选过主题后，系统再开高对比也不改动他的选择", () => {
    const media = stubMatchMedia({ contrast: false });
    const { result } = renderHook(() => useTheme());
    act(() => result.current.setTheme("green"));
    act(() => media.setContrast(true));
    expect(result.current.theme).toBe("green");
    // 系统关掉高对比同样不该动显式主题
    act(() => media.setContrast(false));
    expect(result.current.theme).toBe("green");
  });
});
describe("兜底出口", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("storage 读抛错时按「没存过」处理，不把异常抛给主题初始化", () => {
    const storage = {
      getItem: () => {
        throw new Error("SecurityError");
      },
    };
    expect(readStoredTheme(storage)).toBe("day");
    expect(hasStoredTheme(storage)).toBe(false);
  });

  it("没有 getComputedStyle 时不取色，调用方保留中性兜底", () => {
    vi.stubGlobal("getComputedStyle", undefined);
    expect(readThemeBackground(document.documentElement)).toBe("");
  });

  it("computedStyle 抛错时同样静默返回空值", () => {
    vi.stubGlobal("getComputedStyle", () => {
      throw new Error("boom");
    });
    expect(readThemeBackground(document.documentElement)).toBe("");
  });

  it("老 Safari 只有 addListener：高对比变化照样能监听到", () => {
    let handler: ((event: MediaQueryListEvent) => void) | null = null;
    vi.stubGlobal("matchMedia", (media: string) => ({
      matches: false,
      media,
      // 只保留旧接口，强制走 addListener 分支
      addListener: (next: (event: MediaQueryListEvent) => void) => {
        if (media === CONTRAST) handler = next;
      },
      removeListener: () => undefined,
      dispatchEvent: () => true,
    }));
    localStorage.clear();
    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("day");
    act(() => handler?.({ matches: true } as MediaQueryListEvent));
    expect(result.current.theme).toBe("contrast");
  });
});
