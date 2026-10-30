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
    register: vi.fn(),
    login: vi.fn(),
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

describe("侧栏 / H5 抽屉底部的账号区", () => {
  it("已登录：显示账号名 + 退出登录", () => {
    renderAccount("/shelf", makeAuth({ user: USER, loggedIn: true }));
    expect(screen.getByTestId("side-account-name").textContent).toBe("夜读的人");
    expect(screen.getByTestId("side-account-logout").textContent).toBe("退出登录");
    expect(screen.queryByTestId("side-account-login")).toBeNull();
  });

  it("未登录：给「登录 / 注册」入口并带回跳地址（不强制拦截）", () => {
    renderAccount("/book/7", makeAuth());
    const link = screen.getByTestId("side-account-login") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/login?next=%2Fbook%2F7");
    expect(screen.queryByTestId("side-account-logout")).toBeNull();
  });

  it("⑧ 退出登录：清账号态 + 回「全部书」视图（不留在空白的我的书架）", async () => {
    const logout = vi.fn(async () => undefined);
    renderAccount("/shelf?view=mine", makeAuth({ user: USER, loggedIn: true, logout }));

    fireEvent.click(screen.getByTestId("side-account-logout"));
    await waitFor(() => expect(logout).toHaveBeenCalledTimes(1));
    // 退出后「我的书架」对当前账号已不成立：落到全部书，而不是空列表
    await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/shelf?view=all"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("已退出登录"));
  });
});
