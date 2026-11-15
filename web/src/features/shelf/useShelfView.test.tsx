import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AuthContext, type AuthState } from "../../lib/auth";
import { useShelfView } from "./useShelfView";

/**
 * 视图 ⇄ 地址：本地切换立即生效，外部导航（后退 / 分享链接）以地址为准。
 * 这里只挂 hook + 两个按钮，地址由外部传进来的 initialEntries 模拟。
 */

const member = { id: 1, name: "读者", createdAt: "", lastSeenAt: "" };

const anonymous: AuthState = {
  user: null,
  loggedIn: false,
  loading: false,
  enter: () => Promise.resolve(member),
  register: () => Promise.resolve(member),
  login: () => Promise.resolve(member),
  deleteAccount: () => Promise.resolve(),
  logout: () => Promise.resolve(),
  subscribe: () => () => undefined,
  clearLocal: () => undefined,
};

const loggedIn: AuthState = { ...anonymous, loggedIn: true, user: { ...member, id: 2, name: "读者乙" } };

/** 只负责渲染：账号态由外层 Provider 给，hook 才读得到。 */
function Harness() {
  const view = useShelfView();
  return (
    <div>
      <span data-testid="view">{view.view}</span>
      <span data-testid="explicit">{String(view.explicitView)}</span>
      <span data-testid="logged">{String(view.loggedIn)}</span>
      <button type="button" onClick={() => view.setView("mine")}>我的书架</button>
      <button type="button" onClick={() => view.setView("all")}>全部书</button>
    </div>
  );
}

function tree(entry: string, auth: AuthState) {
  return (
    <AuthContext.Provider value={auth}>
      <MemoryRouter initialEntries={[entry]}>
        <Harness />
      </MemoryRouter>
    </AuthContext.Provider>
  );
}

function mount(entry: string, auth: AuthState = anonymous) {
  return render(tree(entry, auth));
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useShelfView 默认视图", () => {
  it("地址没写 view 时按登录态取默认值", () => {
    const view = mount("/shelf");
    expect(screen.getByTestId("view").textContent).toBe("all");
    expect(screen.getByTestId("explicit").textContent).toBe("null");
    view.unmount();

    mount("/shelf", loggedIn);
    expect(screen.getByTestId("view").textContent).toBe("mine");
    expect(screen.getByTestId("logged").textContent).toBe("true");
  });

  it("地址上的 view 合法时直接用，非法值当没给", () => {
    const view = mount("/shelf?view=mine", loggedIn);
    expect(screen.getByTestId("view").textContent).toBe("mine");
    expect(screen.getByTestId("explicit").textContent).toBe("mine");
    view.unmount();

    mount("/shelf?view=bogus", loggedIn);
    expect(screen.getByTestId("view").textContent).toBe("mine");
    expect(screen.getByTestId("explicit").textContent).toBe("null");
  });

  it("地址带共享书库筛选时一律落在全部书", () => {
    mount("/shelf?author=8", loggedIn);
    expect(screen.getByTestId("view").textContent).toBe("all");
  });
});

describe("useShelfView 本地切换", () => {
  it("点按钮立刻切换视图", () => {
    mount("/shelf", loggedIn);
    expect(screen.getByTestId("view").textContent).toBe("mine");

    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "全部书" }));
    });
    expect(screen.getByTestId("view").textContent).toBe("all");
  });

  it("地址随后补上相同视图时，本地选择继续有效", () => {
    const view = mount("/shelf?view=mine", loggedIn);
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "全部书" }));
    });
    // 写地址 effect 还没跑，本地已生效
    expect(screen.getByTestId("view").textContent).toBe("all");

    // 地址补齐成同一视图 → 仍是「全部书」
    view.rerender(tree("/shelf?view=all", loggedIn));
    expect(screen.getByTestId("view").textContent).toBe("all");
  });
});
