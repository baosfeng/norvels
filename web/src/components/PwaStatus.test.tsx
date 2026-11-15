import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PwaStatus } from "./PwaStatus";

/** 离线条：在线时不占位置，掉线时给出可读提示。jsdom 的 navigator.onLine 可被改写。 */

const originalOnLine = Object.getOwnPropertyDescriptor(window.navigator, "onLine");

function setOnline(value: boolean): void {
  Object.defineProperty(window.navigator, "onLine", { value, configurable: true });
}

afterEach(() => {
  if (originalOnLine) Object.defineProperty(window.navigator, "onLine", originalOnLine);
  vi.restoreAllMocks();
});

describe("PwaStatus", () => {
  it("在线时不渲染任何内容", () => {
    setOnline(true);
    const { container } = render(<PwaStatus />);
    expect(container.textContent).toBe("");
    expect(screen.queryByText("离线阅读")).toBeNull();
  });

  it("承载 offline 事件后展示离线标识", () => {
    setOnline(true);
    render(<PwaStatus />);
    act(() => {
      window.dispatchEvent(new Event("offline"));
    });
    expect(screen.getByText("离线阅读")).toBeTruthy();
  });

  it("回到 online 事件后隐藏标识", () => {
    setOnline(true);
    render(<PwaStatus />);
    act(() => {
      window.dispatchEvent(new Event("offline"));
    });
    expect(screen.getByText("离线阅读")).toBeTruthy();

    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    expect(screen.queryByText("离线阅读")).toBeNull();
  });

  it("初始就是离线时直接展示标识", () => {
    setOnline(false);
    render(<PwaStatus />);
    expect(screen.getByText("离线阅读")).toBeTruthy();
  });

  it("卸载后不再响应网络事件（监听已移除）", () => {
    setOnline(true);
    const { unmount } = render(<PwaStatus />);
    unmount();
    expect(() => {
      act(() => {
        fireEvent(window, new Event("offline"));
      });
    }).not.toThrow();
    expect(screen.queryByText("离线阅读")).toBeNull();
  });
});
