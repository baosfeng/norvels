import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { ErrorPanel } from "./StatePanel";

function renderPanel(error: ApiError | null) {
  const onRetry = vi.fn();
  render(<ErrorPanel error={error} title="没有读出来" onRetry={onRetry} testId="state:error" />);
  return { onRetry };
}

describe("ErrorPanel：错误文案按 code 分流", () => {
  it("单章正文损坏（503 + content_corrupted）只说这一章，不说数据库", () => {
    renderPanel(new ApiError("http", "本章正文读取失败", 503, "content_corrupted"));
    const text = screen.getByTestId("state:error").textContent ?? "";
    expect(text).toContain("这一章");
    expect(text).toContain("下一章");
    expect(text).not.toContain("数据库");
  });

  it("503 但 code 为 content_corrupted 时不落到「数据库不可用」", () => {
    renderPanel(new ApiError("http", "本章正文读取失败", 503, "content_corrupted"));
    const text = screen.getByTestId("state:error").textContent ?? "";
    expect(text).not.toContain("数据库暂时不可用");
    expect(text).not.toContain("暂时不可用");
  });

  it("推荐库未启用（503 + unavailable）说「未启用」，与超时不同文案", () => {
    renderPanel(new ApiError("http", "推荐功能未启用（推荐库不可用）", 503, "unavailable"));
    const unavailable = screen.getByTestId("state:error").textContent ?? "";
    expect(unavailable).toContain("未启用");
    expect(unavailable).not.toContain("数据库");
  });

  it("超时与 503 未启用不再是同一句，且都不提数据库", () => {
    const { unmount } = render(
      <ErrorPanel error={new ApiError("http", "超时", 503, "timeout")} title="没有读出来" onRetry={() => {}} testId="state:error" />,
    );
    const timeoutText = screen.getByTestId("state:error").textContent ?? "";
    unmount();
    renderPanel(new ApiError("http", "推荐功能未启用", 503, "unavailable"));
    const unavailableText = screen.getByTestId("state:error").textContent ?? "";
    expect(timeoutText).not.toBe(unavailableText);
    expect(timeoutText).not.toContain("数据库");
    expect(unavailableText).not.toContain("数据库");
  });

  it("只剩状态码 503（没有 code）也不指向数据库", () => {
    renderPanel(new ApiError("http", "请求失败（503）", 503));
    const text = screen.getByTestId("state:error").textContent ?? "";
    expect(text).not.toContain("数据库");
    expect(text).toContain("稍后再试");
  });

  it("unauthorized 走整页：主操作变成去登录，不再给重试", () => {
    const { onRetry } = renderPanel(new ApiError("http", "未登录", 401, "unauthorized"));
    const login = screen.getByTestId("state:login");
    expect(login.getAttribute("href")).toBe("/login");
    expect(login.textContent).toContain("去登录");
    expect(screen.queryByRole("button", { name: "重试" })).toBeNull();
    expect(onRetry).not.toHaveBeenCalled();
  });

  it("未知 code 有兜底文案：不为空，也不出现 undefined", () => {
    renderPanel(new ApiError("http", "怪错误", 418, "no_such_code"));
    const text = screen.getByTestId("state:error").textContent ?? "";
    expect(text.trim().length).toBeGreaterThan(0);
    expect(text).not.toContain("undefined");
    expect(screen.getByRole("button", { name: "重试" })).toBeTruthy();
  });

  it("error 为 null 时有兜底文案，重试仍可用", () => {
    const { onRetry } = renderPanel(null);
    const text = screen.getByTestId("state:error").textContent ?? "";
    expect(text.trim().length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
