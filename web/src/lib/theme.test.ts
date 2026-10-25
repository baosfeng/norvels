import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyTheme, isTheme, prefersDark, readStoredTheme, resolveTheme, useTheme } from "./theme";

/** 可控的 matchMedia 桩：能读当前 matches，也能手动触发 change。 */
function stubMatchMedia(initialDark: boolean) {
  const listeners = new Set<(event: MediaQueryListEvent) => void>();
  const state = { dark: initialDark };
  const query = {
    get matches() {
      return state.dark;
    },
    media: "(prefers-color-scheme: dark)",
    addEventListener: (_: string, handler: (event: MediaQueryListEvent) => void) => listeners.add(handler),
    removeEventListener: (_: string, handler: (event: MediaQueryListEvent) => void) => listeners.delete(handler),
    addListener: (handler: (event: MediaQueryListEvent) => void) => listeners.add(handler),
    removeListener: (handler: (event: MediaQueryListEvent) => void) => listeners.delete(handler),
    dispatchEvent: () => true,
  };
  vi.stubGlobal("matchMedia", (media: string) => ({
    ...query,
    media,
    // reduced-motion 查询一律 false，避免过渡 class 干扰断言
    matches: media.includes("prefers-color-scheme") ? state.dark : false,
  }));
  return {
    flip(dark: boolean) {
      state.dark = dark;
      for (const handler of listeners) handler({ matches: dark } as MediaQueryListEvent);
    },
    listenerCount: () => listeners.size,
  };
}

describe("主题：四档与跟随系统", () => {
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
    stubMatchMedia(true);
    expect(prefersDark()).toBe(true);
    vi.unstubAllGlobals();
    vi.stubGlobal("matchMedia", undefined);
    expect(prefersDark()).toBe(false);
  });

  it("选「跟随系统」后系统切深色，页面跟着变，且存的是 system 而不是 night", () => {
    const media = stubMatchMedia(false);
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

  it("四档主题都在切换列表里，顺序为 日间/夜间/羊皮纸/跟随系统", () => {
    stubMatchMedia(false);
    const { result } = renderHook(() => useTheme());
    expect(result.current.themes).toEqual(["day", "night", "parchment", "system"]);
  });
});
