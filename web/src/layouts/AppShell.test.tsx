import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { TierProvider } from "../lib/tier";

// 侧栏取数走 hooks：这里给一份最小桩，外壳测试只关心顶栏与分档
vi.mock("../api/hooks", () => ({
  useStats: () => ({ data: { books: 0, chapters: 0, words: 0, authors: 0 }, loading: false, error: null, reload: () => undefined }),
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: () => undefined }),
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
