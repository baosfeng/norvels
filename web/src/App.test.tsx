import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { clearToast, showToast } from "./lib/toastStore";

// 阅读器用替身：本用例只验证「toast 容器挂在路由之外」这件事，
// 不依赖阅读器实现（那边归阅读器域），也不会因为阅读器迭代而红。
vi.mock("./features/reader/ReaderPage", () => ({
  ReaderPage: () => <div data-testid="fake-reader">阅读器</div>,
}));

/**
 * B 类缺陷「阅读器 toast 不可见」的复现测试。
 * /read 是顶层路由（阅读器是独立外壳，不在 AppShell 内），所以 toast 容器必须挂在路由之外。
 * 反向验证：把 ToastHost 挪回 AppShell 内，本用例查不到 toast 即红。
 */
describe("toast 在阅读器路由下也能渲染", () => {
  afterEach(() => {
    act(() => clearToast());
    window.history.pushState({}, "", "/");
  });

  it("挂在路由之外：/read 下 showToast 可见，且只有一份容器", async () => {
    window.history.pushState({}, "", "/read/1/2");
    const { App } = await import("./App");
    render(<App />);

    // 先确认这一帧确实在阅读器外壳里：没有侧栏 = 不在 AppShell 内
    expect(screen.getByTestId("fake-reader")).toBeTruthy();
    expect(screen.queryByTestId("sidebar")).toBeNull();

    act(() => showToast("已加入书单"));

    expect(screen.getByTestId("toast").textContent).toContain("已加入书单");
    // 单点渲染：AppShell 里再挂一份就会命中两个容器
    expect(screen.queryAllByTestId("toast")).toHaveLength(1);
  });
});
