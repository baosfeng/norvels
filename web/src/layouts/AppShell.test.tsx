import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { TierProvider } from "../lib/tier";

// 侧栏取数走 hooks：这里给一份最小桩，外壳测试只关心顶栏与分档
vi.mock("../api/hooks", () => ({
  useStats: () => ({ data: { books: 0, chapters: 0, words: 0, authors: 0 }, loading: false, error: null, reload: () => undefined }),
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: () => undefined }),
  useVersion: () => ({ data: { version: "0.9.0", commit: "abc1234", builtAt: "2026-11-02T10:00:00Z" }, loading: false, error: null, reload: () => undefined }),
}));

const { AppShell } = await import("./AppShell");

function renderShell(tier: "pc" | "h5", path = "/shelf") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<AppShell />}>
            <Route path="/shelf" element={<p>书架页</p>} />
            <Route path="/book/:bookId" element={<p>详情页</p>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

describe("外壳顶栏的视图切换", () => {
  it("PC 书架有切换按钮，点「列表」把模式切过去", () => {
    renderShell("pc");
    const group = screen.getByRole("group", { name: "视图切换" });
    const grid = group.querySelector('[aria-pressed="true"]') as HTMLButtonElement;
    expect(grid.textContent).toContain("网格");

    fireEvent.click(screen.getByRole("button", { name: "列表" }));
    // 同一次渲染里两个按钮的 aria-pressed 互斥
    expect(screen.getByRole("button", { name: "列表" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: "网格" }).getAttribute("aria-pressed")).toBe("false");
  });

  it("H5 书架不渲染切换按钮（单列简表，切了也不会变）", () => {
    renderShell("h5");
    expect(screen.getByText("书架页")).toBeTruthy();
    expect(screen.queryByRole("group", { name: "视图切换" })).toBeNull();
  });

  it("非书架路由（PC /book/7）也不渲染切换按钮", () => {
    renderShell("pc", "/book/7");
    expect(screen.getByText("详情页")).toBeTruthy();
    expect(screen.queryByRole("group", { name: "视图切换" })).toBeNull();
  });
});

// 回归：H5 的侧栏是用 translateX(-248px) 移出屏幕的抽屉，不是 display:none。
// 收起时那 8 个可聚焦项仍在渲染树里，Tab 会把焦点送到屏幕外看不见的地方
// （实测 sidebar 里 a/button 的 x 全是 -232，且 first.focus() 能成功）。
describe("侧栏抽屉收起时不进焦点顺序", () => {
  it("H5 收起态给侧栏打 inert，打开后移除", () => {
    renderShell("h5");
    expect(screen.getByTestId("sidebar").hasAttribute("inert")).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "打开导航" }));
    expect(screen.getByTestId("sidebar").hasAttribute("inert")).toBe(false);
  });

  it("PC 侧栏常驻，任何时候都不加 inert", () => {
    renderShell("pc");
    expect(screen.getByTestId("sidebar").hasAttribute("inert")).toBe(false);
  });
});
