import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSearchScrollRestore } from "./useSearchScrollRestore";

const KEY = "norvels.search.scroll.v1";

function seed(href: string, top: number) {
  sessionStorage.setItem(KEY, JSON.stringify({ href, top }));
}

beforeEach(() => {
  sessionStorage.clear();
  vi.restoreAllMocks();
});

describe("搜索页滚动位置记忆", () => {
  it("地址一致时，结果渲染出来后滚回原位", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    seed("/search?q=长夜", 420);
    renderHook(() => useSearchScrollRestore("/search?q=长夜", true));
    expect(scrollTo).toHaveBeenCalledWith({ top: 420 });
  });

  it("换过关键词（地址不同）不恢复旧位置", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    seed("/search?q=长夜", 420);
    renderHook(() => useSearchScrollRestore("/search?q=旧城", true));
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("结果还没出来时不滚：骨架屏高度和结果行不同，提前滚会停错", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    seed("/search?q=长夜", 420);
    renderHook(() => useSearchScrollRestore("/search?q=长夜", false));
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("没记过位置（例如首次进入）不滚", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    renderHook(() => useSearchScrollRestore("/search?q=长夜", true));
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("存储里坏了当没记过，不抛错", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    sessionStorage.setItem(KEY, "{不是 JSON");
    expect(() => renderHook(() => useSearchScrollRestore("/search?q=长夜", true))).not.toThrow();
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("位置为 0 时不当作要恢复（等价于回到顶部，没必要记）", () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    seed("/search?q=长夜", 0);
    renderHook(() => useSearchScrollRestore("/search?q=长夜", true));
    expect(scrollTo).not.toHaveBeenCalled();
  });
});