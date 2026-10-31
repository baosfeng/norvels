import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReaderErrorBoundary, ReaderErrorPanel } from "./ReaderErrorBoundary";

/** 可控的抛错子组件：retry 用例里把开关关掉，验证边界真的重新渲染。 */
let boom = true;

function Boom() {
  if (boom) throw new Error("渲染炸了");
  return <p>正文回来了</p>;
}

function DetailProbe() {
  const location = useLocation();
  return <p>书籍详情 {location.pathname}</p>;
}

function ShelfProbe() {
  const location = useLocation();
  const state = location.state as { restore?: boolean } | null;
  return <p>书架页 {state?.restore ? "restore" : "none"}</p>;
}

function renderWithBoundary(bookId = 7) {
  return render(
    <MemoryRouter initialEntries={["/read/7/10"]}>
      <Routes>
        <Route
          path="/read/:bookId/:chapterId"
          element={
            <ReaderErrorBoundary fallback={(reset) => <ReaderErrorPanel bookId={bookId} onRetry={reset} />}>
              <Boom />
            </ReaderErrorBoundary>
          }
        />
        <Route path="/book/:bookId" element={<DetailProbe />} />
        <Route path="/shelf" element={<ShelfProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
  boom = true;
});

/**
 * 口径：渲染期异常必须落到兜底面板，而不是整页白屏。
 * 兜底面板保留阅读器外壳的出口：重试（重置边界）、回书籍详情、回书架。
 */
describe("阅读器渲染兜底", () => {
  it("子组件抛错：出兜底面板与三个出口，自己的 console.error 只打一次", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    renderWithBoundary();

    const panel = screen.getByTestId("reader-error-panel");
    expect(panel.textContent).toContain("这一页没能渲染");
    expect(screen.getByTestId("reader-error-retry").textContent).toBe("重试");
    expect(screen.getByTestId("reader-error-detail").textContent).toBe("回书籍详情");
    expect(screen.getByTestId("reader-error-shelf").textContent).toBe("回书架");

    // React 自己也会往 console.error 打一条；这里只数我们自己打的那条
    const ours = spy.mock.calls.filter((call) => String(call[0]).includes("[reader]"));
    expect(ours).toHaveLength(1);
  });

  it("点「重试」：边界重置，子组件重新渲染", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    renderWithBoundary();

    boom = false; // 这次不会再抛
    fireEvent.click(screen.getByTestId("reader-error-retry"));

    expect(screen.queryByTestId("reader-error-panel")).toBeNull();
    expect(screen.getByText("正文回来了")).toBeTruthy();
  });

  it("「回书籍详情」与「回书架」都能跳走", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    renderWithBoundary();

    fireEvent.click(screen.getByTestId("reader-error-detail"));
    expect(screen.getByText("书籍详情 /book/7")).toBeTruthy();

    vi.spyOn(console, "error").mockImplementation(() => {});
    boom = true;
    renderWithBoundary();
    fireEvent.click(screen.getByTestId("reader-error-shelf"));
    expect(screen.getByText("书架页 restore")).toBeTruthy();
  });
});
