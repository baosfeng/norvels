import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { AuthUser } from "../../api/types";
import { AuthProvider } from "../../lib/AuthProvider";
import type { AuthState } from "../../lib/auth";
import { TierProvider } from "../../lib/tier";
import { LoginPage } from "./LoginPage";

const user: AuthUser = { id: 2, name: "夜读的人", createdAt: "2026-10-01T00:00:00Z", lastSeenAt: "" };

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
        </MemoryRouter>
      </AuthProvider>
    </TierProvider>,
  );
}

describe("登录 / 注册页：一个输入框 + 两个动作，无密码", () => {
  it("没有密码框，只有账号名一个输入", () => {
    renderLogin();
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

/**
 * H5 的竖屏预算：390×844 下首屏必须同时看到两个动作。
 * 实测（390 宽）说明把主操作顶出首屏的是三行长说明——所以短文案是硬要求，不是措辞偏好。
 */
describe("登录页 H5：短说明 + 主操作占主位 + 错误提示贴输入框", () => {
  it("H5 用短说明（≤2 行份量），PC 保留完整说明", () => {
    const { unmount } = renderLogin("/login", makeAuth(), "h5");
    const h5Sub = screen.getByTestId("auth-sub").textContent ?? "";
    expect(h5Sub).toBe("阅读记录跟着账号走。没有密码，输入账号名就能进。");
    expect(h5Sub).not.toContain("注册新账号");
    unmount();

    renderLogin("/login", makeAuth(), "pc");
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

  it("两个动作在 H5 与 PC 都可达（层级差由样式承担，DOM 不变）", () => {
    renderLogin("/login", makeAuth(), "h5");
    expect(screen.getByTestId("auth-login").textContent).toBe("进入");
    expect(screen.getByTestId("auth-register").textContent).toBe("注册新账号");
  });
});
