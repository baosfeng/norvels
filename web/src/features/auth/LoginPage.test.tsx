import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { AuthUser, AuthUsersResult } from "../../api/types";
import { AuthProvider } from "../../lib/AuthProvider";
import type { AuthState } from "../../lib/auth";
import { TierProvider } from "../../lib/tier";
import { ToastHost } from "../../lib/toast";
import { clearToast } from "../../lib/toastStore";
import { LoginPage } from "./LoginPage";

const authUsers = vi.fn<() => Promise<AuthUsersResult>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { ...actual.api, authUsers: () => authUsers() } };
});

const user: AuthUser = { id: 2, name: "夜读的人", createdAt: "2026-10-01T00:00:00Z", lastSeenAt: "" };

/** 列表里的两个账号：一个五分钟前还看过书，一个建号后从没登录过。 */
function accountList(): AuthUsersResult {
  return {
    items: [
      {
        id: 2,
        name: "夜读的人",
        createdAt: "2026-10-01T00:00:00Z",
        lastSeenAt: new Date(Date.now() - 5 * 60_000).toISOString(),
      },
      { id: 3, name: "白天读的人", createdAt: "2026-09-01T00:00:00Z", lastSeenAt: null },
    ],
    total: 2,
  };
}

function makeAuth(over: Partial<AuthState> = {}): AuthState {
  return {
    user: null,
    loggedIn: false,
    loading: false,
    register: vi.fn(async () => user),
    login: vi.fn(async () => user),
    logout: vi.fn(async () => undefined),
    subscribe: () => () => undefined,
    clearLocal: () => undefined,
    ...over,
  };
}

/** 落地页：显示当前地址，用来断言"登录成功后回到来源页"。 */
function Landing() {
  const location = useLocation();
  return <p data-testid="landing">{location.pathname + location.search}</p>;
}

function renderLogin(path = "/login", auth: AuthState = makeAuth(), tier: "pc" | "h5" = "pc") {
  return render(
    <TierProvider value={tier}>
      <AuthProvider initialState={auth}>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/book/:id" element={<Landing />} />
            <Route path="/shelf" element={<Landing />} />
          </Routes>
          <ToastHost />
        </MemoryRouter>
      </AuthProvider>
    </TierProvider>,
  );
}

beforeEach(() => {
  // 默认没有账号：老表单形态（不 mock 就会真发请求，用例就随环境飘）
  authUsers.mockReset();
  authUsers.mockResolvedValue({ items: [], total: 0 });
  clearToast();
});

describe("登录 / 注册页：一个输入框 + 两个动作，无密码", () => {
  it("没有密码框，只有账号名一个输入", async () => {
    renderLogin();
    // 等账号列表这次取数落定（空列表 → 老表单），避免断言撞上加载中的一帧
    await screen.findByTestId("auth-login");
    const inputs = document.querySelectorAll("input");
    expect(inputs).toHaveLength(1);
    expect((inputs[0] as HTMLInputElement).type).toBe("text");
    expect(screen.getByTestId("auth-login").textContent).toBe("进入");
    expect(screen.getByTestId("auth-register").textContent).toBe("注册新账号");
  });

  it("①注册成功 → 进入，并回到来源页", async () => {
    const auth = makeAuth();
    renderLogin("/login?next=%2Fbook%2F7", auth);
    fireEvent.change(screen.getByTestId("auth-name"), { target: { value: "夜读的人" } });
    fireEvent.click(screen.getByTestId("auth-register"));
    await waitFor(() => expect(auth.register).toHaveBeenCalledWith("夜读的人"));
    await waitFor(() => expect(screen.getByTestId("landing").textContent).toBe("/book/7"));
  });

  it("②登录 404 → 提示「账号不存在，请先注册」并给出注册按钮，点了带上已输入的名字", async () => {
    const register = vi.fn(async () => user);
    const login = vi.fn(async () => {
      throw new ApiError("http", "账号不存在，请先注册", 404, "account_not_found");
    });
    renderLogin("/login", makeAuth({ login, register }));
    fireEvent.change(screen.getByTestId("auth-name"), { target: { value: "新来的" } });
    fireEvent.click(screen.getByTestId("auth-login"));
    await waitFor(() => expect(screen.getByTestId("auth-hint").textContent).toContain("账号不存在，请先注册"));
    const guide = screen.getByTestId("auth-hint-register");
    expect(guide.textContent).toBe("注册新账号");
    fireEvent.click(guide);
    await waitFor(() => expect(register).toHaveBeenCalledWith("新来的"));
  });

  it("③重名 409 → 提示「该账号已存在，直接进入即可」并给出登录按钮", async () => {
    const register = vi.fn(async () => {
      throw new ApiError("http", "这个账号名已经有人用了", 409, "conflict");
    });
    const login = vi.fn(async () => user);
    renderLogin("/login", makeAuth({ register, login }));
    fireEvent.change(screen.getByTestId("auth-name"), { target: { value: "老账号" } });
    fireEvent.click(screen.getByTestId("auth-register"));
    await waitFor(() => expect(screen.getByTestId("auth-hint").textContent).toContain("该账号已存在，直接进入即可"));
    const guide = screen.getByTestId("auth-hint-login");
    expect(guide.textContent).toBe("直接进入");
    fireEvent.click(guide);
    await waitFor(() => expect(login).toHaveBeenCalledWith("老账号"));
  });

  it("空名与超 32 字在前端就拦下，不发请求", async () => {
    const auth = makeAuth();
    renderLogin("/login", auth);
    fireEvent.click(screen.getByTestId("auth-login"));
    await waitFor(() => expect(screen.getByTestId("auth-hint").textContent).toBe("先输入账号名。"));
    fireEvent.change(screen.getByTestId("auth-name"), { target: { value: "字".repeat(33) } });
    fireEvent.click(screen.getByTestId("auth-register"));
    await waitFor(() => expect(screen.getByTestId("auth-hint").textContent).toBe("账号名最多 32 字。"));
    expect(auth.login).not.toHaveBeenCalled();
    expect(auth.register).not.toHaveBeenCalled();
  });

  it("提交中两个动作都禁用（防重复提交）", async () => {
    let release: (u: AuthUser) => void = () => undefined;
    const login = vi.fn(() => new Promise<AuthUser>((resolve) => (release = resolve)));
    renderLogin("/login", makeAuth({ login }));
    fireEvent.change(screen.getByTestId("auth-name"), { target: { value: "夜读的人" } });
    fireEvent.click(screen.getByTestId("auth-login"));
    await waitFor(() => expect((screen.getByTestId("auth-login") as HTMLButtonElement).disabled).toBe(true));
    expect((screen.getByTestId("auth-register") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("auth-login").textContent).toBe("正在进入…");
    release(user);
    await waitFor(() => expect(screen.getByTestId("landing")).toBeTruthy());
  });

  it("已登录的人打开登录页直接回来源页（不做中间页）", async () => {
    renderLogin("/login?next=%2Fshelf", makeAuth({ user, loggedIn: true }));
    await waitFor(() => expect(screen.getByTestId("landing").textContent).toBe("/shelf"));
  });
});

describe("账号选择器：点一个已有账号就进去", () => {
  it("列出账号名与上次活跃（相对时间 + title 里绝对时间）", async () => {
    authUsers.mockResolvedValue(accountList());
    renderLogin("/login");
    await waitFor(() => expect(screen.getByTestId("auth-user-list")).toBeTruthy());

    const first = screen.getByTestId("auth-user-2");
    expect(first.textContent).toContain("夜读的人");
    expect(first.textContent).toContain("上次活跃 5 分钟前");
    expect(first.getAttribute("title")).toMatch(/年.+月.+日 \d{2}:\d{2}/);

    const second = screen.getByTestId("auth-user-3");
    expect(second.textContent).toContain("白天读的人");
    expect(second.textContent).toContain("从未登录");
    // 未登录时没有「当前」标记
    expect(screen.queryByTestId("auth-user-current-2")).toBeNull();
  });

  it("点一行 = 直接登录，并回到来源页", async () => {
    authUsers.mockResolvedValue(accountList());
    const auth = makeAuth();
    renderLogin("/login?next=%2Fbook%2F7", auth);
    await waitFor(() => expect(screen.getByTestId("auth-user-2")).toBeTruthy());

    fireEvent.click(screen.getByTestId("auth-user-2"));
    await waitFor(() => expect(auth.login).toHaveBeenCalledWith("夜读的人"));
    await waitFor(() => expect(screen.getByTestId("landing").textContent).toBe("/book/7"));
  });

  it("列表下方是「或者新建账号」：输入框 + 注册按钮，不再显示「进入」", async () => {
    authUsers.mockResolvedValue(accountList());
    const auth = makeAuth();
    renderLogin("/login", auth);
    await waitFor(() => expect(screen.getByTestId("auth-user-list")).toBeTruthy());

    expect(screen.getByText("或者新建账号")).toBeTruthy();
    expect(screen.queryByTestId("auth-login")).toBeNull();
    expect(screen.getByTestId("auth-register").textContent).toBe("注册并进入");

    fireEvent.change(screen.getByTestId("auth-name"), { target: { value: "新来的" } });
    fireEvent.click(screen.getByTestId("auth-register"));
    await waitFor(() => expect(auth.register).toHaveBeenCalledWith("新来的"));
    await waitFor(() => expect(screen.getByTestId("landing").textContent).toBe("/shelf"));
  });
});

describe("成功提示：登录 / 注册都要说一句", () => {
  it("登录成功弹「已登录为 X」", async () => {
    authUsers.mockResolvedValue(accountList());
    renderLogin("/login");
    await waitFor(() => expect(screen.getByTestId("auth-user-2")).toBeTruthy());
    fireEvent.click(screen.getByTestId("auth-user-2"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("已登录为 夜读的人"));
  });

  it("注册成功弹「已创建并登录为 X」", async () => {
    renderLogin("/login");
    fireEvent.change(screen.getByTestId("auth-name"), { target: { value: "夜读的人" } });
    fireEvent.click(screen.getByTestId("auth-register"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("已创建并登录为 夜读的人"));
  });
});

describe("退化：列表为空 / 取数失败都退回老表单", () => {
  it("空列表：只有输入框 + 两个动作", async () => {
    renderLogin("/login");
    await waitFor(() => expect(authUsers).toHaveBeenCalled());
    expect(screen.queryByTestId("auth-user-list")).toBeNull();
    expect(screen.getByTestId("auth-login").textContent).toBe("进入");
    expect(screen.getByTestId("auth-register").textContent).toBe("注册新账号");
  });

  it("取数失败：照常能登录，不显示错误页", async () => {
    authUsers.mockRejectedValue(new ApiError("network", "没能连上服务"));
    const auth = makeAuth();
    renderLogin("/login", auth);
    await waitFor(() => expect(screen.getByTestId("auth-login")).toBeTruthy());
    expect(screen.queryByTestId("auth-user-list")).toBeNull();

    fireEvent.change(screen.getByTestId("auth-name"), { target: { value: "夜读的人" } });
    fireEvent.click(screen.getByTestId("auth-login"));
    await waitFor(() => expect(auth.login).toHaveBeenCalledWith("夜读的人"));
  });
});

describe("?switch=1：已登录也要能进到换号页", () => {
  it("带 switch=1 不跳走，列表里标出当前账号", async () => {
    authUsers.mockResolvedValue(accountList());
    renderLogin("/login?next=%2Fshelf&switch=1", makeAuth({ user, loggedIn: true }));

    await waitFor(() => expect(screen.getByTestId("auth-user-list")).toBeTruthy());
    expect(screen.queryByTestId("landing")).toBeNull();
    expect(screen.getByTestId("auth-user-current-2").textContent).toBe("当前");
    expect(screen.getByTestId("auth-foot").textContent).toContain("当前登录的是「夜读的人」");
  });

  it("换号成功后回到来源页", async () => {
    authUsers.mockResolvedValue(accountList());
    const other: AuthUser = { id: 3, name: "白天读的人", createdAt: "2026-09-01T00:00:00Z", lastSeenAt: "" };
    const auth = makeAuth({ user, loggedIn: true, login: vi.fn(async () => other) });
    renderLogin("/login?next=%2Fshelf&switch=1", auth);

    await waitFor(() => expect(screen.getByTestId("auth-user-3")).toBeTruthy());
    fireEvent.click(screen.getByTestId("auth-user-3"));
    await waitFor(() => expect(auth.login).toHaveBeenCalledWith("白天读的人"));
    await waitFor(() => expect(screen.getByTestId("landing").textContent).toBe("/shelf"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("已登录为 白天读的人"));
  });
});

/**
 * H5 的竖屏预算：390×844 下首屏必须同时看到动作。
 * 实测（390 宽）说明把主操作顶出首屏的是三行长说明——所以短文案是硬要求，不是措辞偏好。
 */
describe("登录页 H5：短说明 + 主操作占主位 + 错误提示贴输入框", () => {
  it("H5 用短说明（≤2 行份量），PC 保留完整说明", async () => {
    const { unmount } = renderLogin("/login", makeAuth(), "h5");
    await screen.findByTestId("auth-login");
    const h5Sub = screen.getByTestId("auth-sub").textContent ?? "";
    expect(h5Sub).toBe("阅读记录跟着账号走。没有密码，输入账号名就能进。");
    expect(h5Sub).not.toContain("注册新账号");
    unmount();

    renderLogin("/login", makeAuth(), "pc");
    await screen.findByTestId("auth-login");
    const pcSub = screen.getByTestId("auth-sub").textContent ?? "";
    expect(pcSub).toContain("第一次来点「注册新账号」");
  });

  it("错误提示排在输入框之后、动作按钮之前（H5 上不落到按钮下方）", async () => {
    renderLogin("/login", makeAuth(), "h5");
    fireEvent.click(screen.getByTestId("auth-login"));
    await waitFor(() => expect(screen.getByTestId("auth-hint")).toBeTruthy());

    const name = screen.getByTestId("auth-name");
    const hint = screen.getByTestId("auth-hint");
    const action = screen.getByTestId("auth-login");
    // 同一棵 form 内，且文档顺序为 输入框 → 提示 → 按钮（compareDocumentPosition: 4 = FOLLOWING）
    expect(name.closest("form")).toBe(action.closest("form"));
    expect(hint.closest("form")).toBe(action.closest("form"));
    expect(name.compareDocumentPosition(hint) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(hint.compareDocumentPosition(action) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("两个动作在 H5 与 PC 都可达（层级差由样式承担，DOM 不变）", async () => {
    renderLogin("/login", makeAuth(), "h5");
    await screen.findByTestId("auth-login");
    expect(screen.getByTestId("auth-login").textContent).toBe("进入");
    expect(screen.getByTestId("auth-register").textContent).toBe("注册新账号");
  });
});
