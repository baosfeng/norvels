import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ApiError } from "../../api/client";
import { ReaderStateArea } from "./ReaderStateArea";

function renderReader(error: ApiError | null) {
  return render(
    <ReaderStateArea
      loading={false}
      error={error}
      retrying={false}
      retryFailures={0}
      next={{ id: 11, title: "第12章", idx: 11 } as never}
      onRetry={() => {}}
      onNext={() => {}}
    />,
  );
}

describe("ReaderStateArea：正文错误按 code 分流", () => {
  it("本章正文损坏（503 + content_corrupted）只说这一章，可读下一章，不提数据库", () => {
    renderReader(new ApiError("http", "本章正文读取失败", 503, "content_corrupted"));
    const text = screen.getByTestId("reader-error").textContent ?? "";
    expect(text).toContain("这一章");
    expect(text).toContain("下一章");
    expect(text).not.toContain("数据库");
  });

  it("503 但 code 为 content_corrupted 时不能说成整库/服务不可用", () => {
    renderReader(new ApiError("http", "本章正文读取失败", 503, "content_corrupted"));
    const text = screen.getByTestId("reader-error").textContent ?? "";
    expect(text).not.toContain("数据库暂时不可用");
    expect(text).not.toContain("暂时不可用");
  });

  it("推荐库未启用与超时不再同文案", () => {
    const { unmount } = render(
      <ReaderStateArea
        loading={false}
        error={new ApiError("http", "推荐功能未启用", 503, "unavailable")}
        retrying={false}
        retryFailures={0}
        next={null}
        onRetry={() => {}}
        onNext={() => {}}
      />,
    );
    const unavailable = screen.getByTestId("reader-error").textContent ?? "";
    unmount();
    renderReader(new ApiError("http", "超时", 503, "timeout"));
    const timeout = screen.getByTestId("reader-error").textContent ?? "";
    expect(unavailable).not.toBe(timeout);
    expect(unavailable).toContain("未启用");
    expect(timeout).toContain("超时");
  });

  it("未鉴权时按整页口径提示重新登录", () => {
    renderReader(new ApiError("http", "未登录", 401, "unauthorized"));
    expect(screen.getByTestId("reader-error").textContent ?? "").toContain("重新登录");
  });

  it("未知 code 与空错误都有兜底文案（含「先读下一章」的出口），不出现 undefined", () => {
    const { unmount } = render(
      <ReaderStateArea
        loading={false}
        error={new ApiError("http", "怪错误", 500, "no_such_code")}
        retrying={false}
        retryFailures={0}
        next={null}
        onRetry={() => {}}
        onNext={() => {}}
      />,
    );
    const unknown = screen.getByTestId("reader-error").textContent ?? "";
    expect(unknown.trim().length).toBeGreaterThan(0);
    expect(unknown).not.toContain("undefined");
    unmount();
    renderReader(new ApiError("http", "请求失败", 503));
    const bare503 = screen.getByTestId("reader-error").textContent ?? "";
    expect(bare503.trim().length).toBeGreaterThan(0);
    expect(bare503).not.toContain("数据库");
  });
});
