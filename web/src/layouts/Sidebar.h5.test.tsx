import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { TierProvider } from "../lib/tier";

vi.mock("../api/hooks", () => ({
  useStats: () => ({
    data: { books: 12, chapters: 340, words: 120000, authors: 3 },
    loading: false,
    error: null,
    reload: () => undefined,
  }),
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: () => undefined }),
}));

const { Sidebar } = await import("./Sidebar");

function renderSidebar(tier: "pc" | "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter>
        <Sidebar />
      </MemoryRouter>
    </TierProvider>,
  );
}

function navLinks(): HTMLAnchorElement[] {
  const nav = screen.getByRole("navigation", { name: "主导航" });
  return Array.from(nav.querySelectorAll("a"));
}

function hrefs(): string[] {
  return navLinks().map((link) => link.getAttribute("href") ?? "");
}

describe("侧栏：H5 抽屉补齐两档都开的能力", () => {
  it("H5 主导航含 首页 / 书架 / 搜索 / 系列 / 阅读历史", () => {
    renderSidebar("h5");
    expect(hrefs()).toEqual(["/home", "/shelf", "/search", "/series", "/history"]);
    expect(screen.getByTestId("nav-series")).toBeTruthy();
    expect(screen.getByTestId("nav-history")).toBeTruthy();
  });

  it("H5 不出现管理四项（条件渲染，没有节点）", () => {
    renderSidebar("h5");
    expect(screen.queryByTestId("nav-admin")).toBeNull();
    const nav = screen.getByRole("navigation", { name: "主导航" });
    expect(within(nav).queryByText("驾驶舱")).toBeNull();
    expect(within(nav).queryByText("作者")).toBeNull();
    expect(within(nav).queryByText("导入管理")).toBeNull();
  });

  it("PC 主导航是管理工作台：含系列与历史，但不含 H5 首页", () => {
    renderSidebar("pc");
    expect(hrefs()).toEqual(["/shelf", "/search", "/series", "/history", "/admin", "/dashboard", "/authors", "/import"]);
  });

  it("没有底部 tab bar：导航只有侧栏这一处（不新增外壳层）", () => {
    renderSidebar("h5");
    expect(document.querySelector(".tabbar, .bottom-nav, [role='tablist']")).toBeNull();
  });
});
