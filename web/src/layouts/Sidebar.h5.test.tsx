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

function renderSidebar(tier: "pc" | "h5", path?: string) {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter {...(path ? { initialEntries: [path] } : {})}>
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
  it("H5 主导航含 首页 / 书架 / 搜索 / 系列 / 阅读历史 / 书单", () => {
    renderSidebar("h5");
    expect(hrefs()).toEqual(["/home", "/shelf", "/search", "/series", "/history", "/collections"]);
    expect(screen.getByTestId("nav-series")).toBeTruthy();
    expect(screen.getByTestId("nav-history")).toBeTruthy();
    expect(screen.getByTestId("nav-collections")).toBeTruthy();
  });

  it("H5 不出现管理四项（条件渲染，没有节点）", () => {
    renderSidebar("h5");
    expect(screen.queryByTestId("nav-admin")).toBeNull();
    const nav = screen.getByRole("navigation", { name: "主导航" });
    expect(within(nav).queryByText("驾驶舱")).toBeNull();
    expect(within(nav).queryByText("作者")).toBeNull();
    expect(within(nav).queryByText("导入管理")).toBeNull();
  });

  it("PC 主导航是管理工作台：含系列、历史与书单，但不含 H5 首页", () => {
    renderSidebar("pc");
    expect(hrefs()).toEqual([
      "/shelf",
      "/search",
      "/series",
      "/history",
      "/collections",
      "/admin",
      "/dashboard",
      "/authors",
      "/import",
    ]);
    expect(screen.getByTestId("nav-collections")).toBeTruthy();
  });

  it("没有底部 tab bar：导航只有侧栏这一处（不新增外壳层）", () => {
    renderSidebar("h5");
    expect(document.querySelector(".tabbar, .bottom-nav, [role='tablist']")).toBeNull();
  });
});

/**
 * 选中态的事实源是 aria-current="page"（样式见 styles/ix-nav.css）。
 * NavLink 另外会给当前项加 class="active"，但原型 CSS 只写了 .is-active——
 * 两边对不上时选中态静默失效，所以这里同时钉住 aria-current 与「别指望 .is-active」。
 */
describe("侧栏当前页：靠 aria-current=page 标记，两档同源", () => {
  function currentLinks(): HTMLAnchorElement[] {
    return navLinks().filter((link) => link.getAttribute("aria-current") === "page");
  }

  it("PC：/shelf 上只有「书架」是当前页，且带 aria-current=page", () => {
    renderSidebar("pc", "/shelf");
    const current = currentLinks();
    expect(current).toHaveLength(1);
    expect(current[0].getAttribute("href")).toBe("/shelf");
    expect(current[0].textContent).toContain("书架");
    // NavLink 的 class 是 "active"，不是原型里的 "is-active"
    expect(current[0].className).toContain("active");
    expect(current[0].className).not.toContain("is-active");
  });

  it("PC：/history 上换成「阅读历史」是当前页", () => {
    renderSidebar("pc", "/history");
    expect(currentLinks().map((link) => link.getAttribute("href"))).toEqual(["/history"]);
  });

  it("H5 抽屉：/home 上「首页」是当前页，其余都不带 aria-current", () => {
    renderSidebar("h5", "/home");
    expect(currentLinks().map((link) => link.getAttribute("href"))).toEqual(["/home"]);
    for (const link of navLinks()) {
      if (link.getAttribute("href") === "/home") continue;
      expect(link.getAttribute("aria-current")).toBeNull();
    }
  });
});
