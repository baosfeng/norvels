import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { AuthUser } from "../api/types";
import type { AuthState } from "../lib/auth";
import { AuthProvider } from "../lib/AuthProvider";
import type { ShelfIdsState } from "../features/shelf/shelfIds";
import { ShelfIdsProvider } from "../features/shelf/ShelfIdsProvider";
import { TierProvider } from "../lib/tier";
import { ToastHost } from "../lib/toast";
import { SideAccount } from "./SideAccount";

const USER: AuthUser = { id: 2, name: "夜读的人", createdAt: "2026-10-01T00:00:00Z", lastSeenAt: "" };

function makeAuth(over: Partial<AuthState> = {}): AuthState {
  return {
    user: null,
    loggedIn: false,
    loading: false,
    enter: vi.fn(),
    register: vi.fn(),
    login: vi.fn(),
    deleteAccount: vi.fn(async () => undefined),
    logout: vi.fn(async () => undefined),
    subscribe: () => () => undefined,
    clearLocal: () => undefined,
    ...over,
  };
}

function makeShelf(over: Partial<ShelfIdsState> = {}): ShelfIdsState {
  return {
    ids: new Set<number>(),
    has: () => false,
    ready: true,
    loading: false,
    toggle: vi.fn(async () => true),
    refresh: () => undefined,
    ...over,
  };
}

function Where() {
  const location = useLocation();
  return <p data-testid="where">{location.pathname + location.search}</p>;
}

function renderAccount(path: string, auth: AuthState, tier: "pc" | "h5" = "h5") {
  return render(
    <TierProvider value={tier}>
      <AuthProvider initialState={auth}>
        <ShelfIdsProvider initialState={makeShelf()}>
          <MemoryRouter initialEntries={[path]}>
            <Routes>
              <Route
                path="*"
                element={
                  <>
                    <SideAccount />
                    <Where />
                  </>
                }
              />
            </Routes>
            <ToastHost />
          </MemoryRouter>
        </ShelfIdsProvider>
      </AuthProvider>
    </TierProvider>,
  );
}

/** 账号操作收在菜单里：点开 trigger 才看得到换号 / 删号 / 退出。 */
function openMenu() {
  fireEvent.click(screen.getByTestId("side-account-trigger"));
}

describe("侧栏 / H5 抽屉底部的账号区", () => {
  it("已登录：一行账号，操作收在菜单里（默认不占四行）", () => {
    renderAccount("/shelf", makeAuth({ user: USER, loggedIn: true }));
    expect(screen.getByTestId("side-account-name").textContent).toBe("夜读的人");
    expect(screen.queryByTestId("side-account-login")).toBeNull();
    // 菜单默认收起：换号 / 退出 / 删号都不占版面
    expect(screen.queryByTestId("side-account-menu")).toBeNull();
    openMenu();
    expect(screen.getByTestId("side-account-menu")).toBeTruthy();
    expect(screen.getByTestId("side-account-logout").textContent).toBe("退出登录");
  });

  it("菜单可以收起：再点一次 trigger，或按 Esc", () => {
    renderAccount("/shelf", makeAuth({ user: USER, loggedIn: true }));
    openMenu();
    expect(screen.getByTestId("side-account-menu")).toBeTruthy();
    fireEvent.click(screen.getByTestId("side-account-trigger"));
    expect(screen.queryByTestId("side-account-menu")).toBeNull();

    openMenu();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByTestId("side-account-menu")).toBeNull();
  });

  it("已登录：给「切换账号」入口，带 ?switch=1（否则登录页会立刻把已登录的人送走）", () => {
    renderAccount("/shelf?view=mine", makeAuth({ user: USER, loggedIn: true }));
    openMenu();
    const link = screen.getByTestId("side-account-switch") as HTMLAnchorElement;
    expect(link.textContent).toBe("切换账号");
    expect(link.getAttribute("href")).toBe("/login?next=%2Fshelf%3Fview%3Dmine&switch=1");
  });

  it("未登录：给「登录 / 注册」入口并带回跳地址（不强制拦截）", () => {
    renderAccount("/book/7", makeAuth());
    const link = screen.getByTestId("side-account-login") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/login?next=%2Fbook%2F7");
    // 没登录就没有可展开的菜单
    expect(screen.queryByTestId("side-account-trigger")).toBeNull();
    expect(screen.queryByTestId("side-account-logout")).toBeNull();
    // 没登录就没有"切换"的对象
    expect(screen.queryByTestId("side-account-switch")).toBeNull();
  });

  it("菜单里没有「删除此账号」：不可逆操作只放在登录页的账号列表", () => {
    renderAccount("/shelf", makeAuth({ user: USER, loggedIn: true }));
    openMenu();
    const menu = screen.getByTestId("side-account-menu");
    expect(menu.textContent).not.toContain("删除");
    expect(screen.queryByTestId("side-account-delete")).toBeNull();
    // 换号与退出仍在
    expect(screen.getByTestId("side-account-switch")).toBeTruthy();
    expect(screen.getByTestId("side-account-logout")).toBeTruthy();
  });

  it("当前账号展示活跃时间（me() 的 lastSeenAt 不为空时不再是「从未登录」）", () => {
    const seen = new Date(Date.now() - 3 * 60_000).toISOString();
    renderAccount("/shelf", makeAuth({ user: { ...USER, lastSeenAt: seen }, loggedIn: true }));
    expect(screen.getByTestId("side-account-seen").textContent).toContain("3 分钟前");
  });

  it("⑧ 退出登录：清账号态 + 回「全部书」视图（不留在空白的我的书架）", async () => {
    const logout = vi.fn(async () => undefined);
    renderAccount("/shelf?view=mine", makeAuth({ user: USER, loggedIn: true, logout }));

    openMenu();
    fireEvent.click(screen.getByTestId("side-account-logout"));
    await waitFor(() => expect(logout).toHaveBeenCalledTimes(1));
    // 退出后「我的书架」对当前账号已不成立：落到全部书，而不是空列表
    await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/shelf?view=all"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("已退出登录"));
  });
});
