import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useReaderPanels } from "./useReaderPanels";

/** 正文容器：H5 时它自己滚，PC 时不滚、由整页滚。 */
function mountSurface(options: { scrolls: boolean }) {
  document.body.innerHTML = "";
  const surface = document.createElement("div");
  surface.dataset.testid = "reader-swipe-area";
  Object.defineProperty(surface, "scrollHeight", { configurable: true, value: options.scrolls ? 3200 : 800 });
  Object.defineProperty(surface, "clientHeight", { configurable: true, value: 800 });
  document.body.appendChild(surface);
  return surface;
}

/** 整页滚：jsdom 的 scrollY 是只读访问器、scrollTo 未实现，两处都接管。 */
function stubPageScroll(start: number) {
  let y = start;
  const calls: Array<{ top: number | undefined }> = [];
  Object.defineProperty(window, "scrollY", { configurable: true, get: () => y });
  vi.spyOn(window, "scrollTo").mockImplementation((_x?: number | ScrollToOptions, next?: number) => {
    const top = typeof next === "number" ? next : typeof _x === "object" && _x ? _x.top : undefined;
    calls.push({ top });
    y = Math.max(0, top ?? 0);
  });
  return { calls, get: () => y };
}

function mount(canSearch = true) {
  const onResetSearch = vi.fn();
  const hook = renderHook(() => useReaderPanels({ canSearch, onResetSearch }));
  return { ...hook, onResetSearch };
}

afterEach(() => {
  document.body.innerHTML = "";
  Reflect.deleteProperty(window, "scrollY");
  vi.restoreAllMocks();
});

/**
 * 防复发：搜索「没打开过」和「打开后又关了」是两件事。
 * 旧实现把 origin 的初值当成了「搜索打开前的 0」，于是任何收起搜索的动作
 * （openSettings / toggleHelp 都会调 closeSearch）都把页面拉到章首——
 * 1280×800 实测 y 1200 → 202，读者回来时已经不在读的那一段。
 */
describe("收起搜索不搬动阅读位置", () => {
  it("没打开过搜索就开设置面板：不写滚动位置", () => {
    const page = stubPageScroll(1200);
    const { result } = mount();
    act(() => result.current.openSettings());
    expect(page.calls).toHaveLength(0);
    expect(result.current.settingsOpen).toBe(true);
    expect(page.get()).toBe(1200);
  });

  it("没打开过搜索就切帮助：同样不写滚动位置", () => {
    const page = stubPageScroll(1200);
    const { result } = mount();
    act(() => result.current.toggleHelp());
    expect(page.calls).toHaveLength(0);
    expect(result.current.helpOpen).toBe(true);
  });

  it("搜索打开后关闭：回到打开搜索时的位置，且只写一次", () => {
    const page = stubPageScroll(1200);
    const { result } = mount();
    act(() => result.current.openSearch());
    // 搜索期间命中把读者带到了别处
    act(() => { window.scrollTo(0, 4300); });
    page.calls.length = 0;
    act(() => result.current.closeSearch());
    expect(page.calls).toHaveLength(1);
    expect(page.calls[0].top).toBe(1200);
    expect(page.get()).toBe(1200);
    expect(result.current.searchOpen).toBe(false);
  });

  it("复原后 origin 清空：再开设置面板不会二次滚动", () => {
    const page = stubPageScroll(1200);
    const { result } = mount();
    act(() => result.current.openSearch());
    act(() => { window.scrollTo(0, 4300); });
    act(() => result.current.closeSearch());
    page.calls.length = 0;
    act(() => { window.scrollTo(0, 2600); });
    page.calls.length = 0;
    act(() => result.current.openSettings());
    expect(page.calls).toHaveLength(0);
    expect(page.get()).toBe(2600);
  });

  it("搜索打开时直接开设置面板：先把读者送回原处，再开面板", () => {
    const page = stubPageScroll(1200);
    const { result } = mount();
    act(() => result.current.openSearch());
    act(() => { window.scrollTo(0, 4300); });
    page.calls.length = 0;
    act(() => result.current.openSettings());
    expect(page.calls).toHaveLength(1);
    expect(page.calls[0].top).toBe(1200);
    expect(result.current.searchOpen).toBe(false);
    expect(result.current.settingsOpen).toBe(true);
  });

  it("关搜索仍然清空查询（复原位置不影响原有口径）", () => {
    stubPageScroll(1200);
    const { result, onResetSearch } = mount();
    act(() => result.current.openSearch());
    act(() => result.current.closeSearch());
    expect(onResetSearch).toHaveBeenCalledTimes(1);
  });
});

/**
 * H5：正文容器自己滚，window.scrollY 恒为 0。
 * 只记 scrollY 等于没记——关搜索时什么都复原不了，读者被命中甩到哪就停在哪。
 */
describe("H5：搜索的阅读位置记在正文容器上", () => {
  beforeEach(() => {
    stubPageScroll(0);
  });

  it("打开搜索记容器 scrollTop，关闭时写回容器（不动 window）", () => {
    const page = stubPageScroll(0);
    const surface = mountSurface({ scrolls: true });
    surface.scrollTop = 1800;
    const { result } = mount();
    act(() => result.current.openSearch());
    surface.scrollTop = 5200;
    page.calls.length = 0;
    act(() => result.current.closeSearch());
    expect(surface.scrollTop).toBe(1800);
    expect(page.calls).toHaveLength(0);
  });

  it("容器自己滚时 openSettings 不动整页", () => {
    const page = stubPageScroll(0);
    const surface = mountSurface({ scrolls: true });
    surface.scrollTop = 1800;
    const { result } = mount();
    act(() => result.current.openSettings());
    expect(page.calls).toHaveLength(0);
    expect(surface.scrollTop).toBe(1800);
  });
});
