import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AISettings } from "../../api/types";

const settingsState = {
  data: { enabled: false, apiKeyConfigured: true } as AISettings | null,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const putAISettings = vi.fn<(enabled: boolean) => Promise<void>>();

vi.mock("../../api/hooks", () => ({ useAISettings: () => settingsState }));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { putAISettings } };
});

const { AIPanel, NO_KEY_HINT } = await import("./AIPanel");
const { ApiError } = await import("../../api/client");

function toggle(): HTMLElement {
  return screen.getByTestId("ai-toggle");
}

describe("AI 审核开关面板", () => {
  beforeEach(() => {
    settingsState.data = { enabled: false, apiKeyConfigured: true };
    settingsState.loading = false;
    settingsState.error = null;
    settingsState.reload.mockReset();
    putAISettings.mockReset();
  });

  it("读回并渲染：服务端 enabled=true 时开关是开着的", () => {
    settingsState.data = { enabled: true, apiKeyConfigured: true };
    render(<AIPanel />);

    expect(screen.getByRole("switch", { name: "AI 审核" }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByTestId("ai-state").textContent).toContain("开启");
  });

  it("读回并渲染：服务端 enabled=false 时开关是关着的", () => {
    render(<AIPanel />);

    expect(toggle().getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByTestId("ai-no-key")).toBeNull();
  });

  it("切换后调用 PUT 并回报新值", async () => {
    putAISettings.mockResolvedValueOnce(undefined);
    render(<AIPanel />);

    fireEvent.click(toggle());

    await waitFor(() => expect(putAISettings).toHaveBeenCalledWith(true));
    expect(toggle().getAttribute("aria-checked")).toBe("true");
    await waitFor(() => expect(screen.getByTestId("ai-saved").textContent).toContain("已开启"));
  });

  it("关闭方向也走 PUT：已开启时点一下写 false", async () => {
    settingsState.data = { enabled: true, apiKeyConfigured: true };
    putAISettings.mockResolvedValueOnce(undefined);
    render(<AIPanel />);

    fireEvent.click(toggle());

    await waitFor(() => expect(putAISettings).toHaveBeenCalledWith(false));
  });

  it("没配 API key 时开关禁用并给提示", () => {
    settingsState.data = { enabled: false, apiKeyConfigured: false };
    render(<AIPanel />);

    expect((toggle() as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("ai-no-key").textContent).toBe(NO_KEY_HINT);
    expect(putAISettings).not.toHaveBeenCalled();
  });

  it("保存失败回滚到原值并显示错误", async () => {
    putAISettings.mockRejectedValueOnce(new ApiError("http", "boom", 500));
    render(<AIPanel />);

    fireEvent.click(toggle());

    await waitFor(() => expect(screen.getByTestId("ai-error-text")).toBeTruthy());
    expect(toggle().getAttribute("aria-checked")).toBe("false"); // 回滚
    expect(putAISettings).toHaveBeenCalledWith(true);
  });

  it("保存中禁用开关，避免连点写出两个值", async () => {
    let release: () => void = () => {};
    putAISettings.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );
    render(<AIPanel />);

    fireEvent.click(toggle());
    await waitFor(() => expect((toggle() as HTMLButtonElement).disabled).toBe(true));
    fireEvent.click(toggle());
    expect(putAISettings).toHaveBeenCalledTimes(1);

    release();
    await waitFor(() => expect((toggle() as HTMLButtonElement).disabled).toBe(false));
  });

  it("读取失败时给错误面板而不是空白开关", () => {
    settingsState.error = new ApiError("http", "boom", 500);
    render(<AIPanel />);

    expect(screen.getByTestId("ai-error")).toBeTruthy();
    expect(screen.queryByTestId("ai-toggle")).toBeNull();
  });
});
