import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PwaStatus } from "./PwaStatus";
import { OFFLINE_UNAVAILABLE_HINT } from "../lib/offlineStatus";

/**
 * 离线条只承诺做得到的事：在线不占位置；断网时**只有** Service Worker 已接管
 * （controller 存在、真有缓存）才说「离线阅读」，否则说「离线不可用 · 需 https 或 localhost 访问」。
 * 三态的纯逻辑在 lib/offlineStatus.test.ts，这里守组件按状态渲染与事件响应。
 */

const originalOnLine = Object.getOwnPropertyDescriptor(window.navigator, "onLine");

function setOnline(value: boolean): void {
  Object.defineProperty(window.navigator, "onLine", { value, configurable: true });
}

/** 模拟已注册并接管本页的 Service Worker（controller 是对象即算接管）。 */
function setController(has: boolean): void {
  Object.defineProperty(window.navigator, "serviceWorker", {
    value: { controller: has ? { scriptURL: "/sw.js" } : null },
    configurable: true,
  });
}

function goOffline(): void {
  act(() => {
    window.dispatchEvent(new Event("offline"));
  });
}

function goOnline(): void {
  act(() => {
    window.dispatchEvent(new Event("online"));
  });
}

afterEach(() => {
  if (originalOnLine) Object.defineProperty(window.navigator, "onLine", originalOnLine);
  Reflect.deleteProperty(window.navigator, "serviceWorker");
  vi.restoreAllMocks();
});

describe("PwaStatus", () => {
  it("在线时不渲染任何内容（有没有 SW 接管结果一样）", () => {
    setOnline(true);
    setController(false);
    const first = render(<PwaStatus />);
    expect(first.container.textContent).toBe("");
    first.unmount();

    setController(true);
    const second = render(<PwaStatus />);
    expect(second.container.textContent).toBe("");
    expect(screen.queryByText("离线阅读")).toBeNull();
  });

  it("断网且没有 SW 接管：说离线不可用并给出访问方式，不承诺离线阅读", () => {
    setOnline(true);
    setController(false);
    render(<PwaStatus />);
    goOffline();
    expect(screen.getByText(OFFLINE_UNAVAILABLE_HINT)).toBeTruthy();
    expect(screen.queryByText("离线阅读")).toBeNull();
  });

  it("断网且 SW 已接管：承诺离线阅读（真有缓存才说）", () => {
    setOnline(true);
    setController(true);
    render(<PwaStatus />);
    goOffline();
    expect(screen.getByText("离线阅读")).toBeTruthy();
    expect(screen.queryByText(OFFLINE_UNAVAILABLE_HINT)).toBeNull();
  });

  it("回到 online 事件后隐藏标识，即便此前是离线可用", () => {
    setOnline(true);
    setController(true);
    render(<PwaStatus />);
    goOffline();
    expect(screen.getByText("离线阅读")).toBeTruthy();

    goOnline();
    expect(screen.queryByText("离线阅读")).toBeNull();
    expect(screen.queryByText(OFFLINE_UNAVAILABLE_HINT)).toBeNull();
  });

  it("初始就是离线且无 SW 接管：直接展示不可用说明", () => {
    setOnline(false);
    setController(false);
    render(<PwaStatus />);
    expect(screen.getByText(OFFLINE_UNAVAILABLE_HINT)).toBeTruthy();
  });

  it("卸载后不再响应网络事件（监听已移除）", () => {
    setOnline(true);
    setController(false);
    const { unmount } = render(<PwaStatus />);
    unmount();
    expect(() => {
      act(() => {
        fireEvent(window, new Event("offline"));
      });
    }).not.toThrow();
    expect(screen.queryByText("离线阅读")).toBeNull();
    expect(screen.queryByText(OFFLINE_UNAVAILABLE_HINT)).toBeNull();
  });
});
