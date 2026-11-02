import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TierProvider } from "../lib/tier";

/**
 * 侧栏页脚的版本位：两档都要能看到「我现在跑的是哪个版本」。
 *
 * 验收时全站含版本号的叶子节点是 0——用户升级后除了翻更新日志无从确认版本，
 * 所以这里钉住三件事：两档都渲染、点了跳 /changelog、拿不到版本时不渲染（不出 vundefined）。
 */
const { version } = vi.hoisted(() => ({ version: vi.fn() }));

vi.mock("../api/hooks", () => ({
  useStats: () => ({
    data: { books: 12, chapters: 340, words: 120000, authors: 3 },
    loading: false,
    error: null,
    reload: () => undefined,
  }),
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: () => undefined }),
  useVersion: () => version(),
}));

const { Sidebar } = await import("./Sidebar");

function renderSidebar(tier: "pc" | "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={["/shelf"]}>
        <Routes>
          <Route path="/shelf" element={<Sidebar />} />
          <Route path="/changelog" element={<p>更新日志页</p>} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  version.mockReset();
  version.mockReturnValue({
    data: { version: "0.9.0", commit: "abc1234", builtAt: "2026-11-02T10:00:00Z" },
    loading: false,
    error: null,
    reload: () => undefined,
  });
});

describe("侧栏页脚的版本位", () => {
  it("PC 侧栏显示 v0.9.0", () => {
    renderSidebar("pc");
    expect(screen.getByTestId("side-version").textContent).toBe("v0.9.0");
  });

  it("H5 抽屉（同一份 DOM）也显示 v0.9.0", () => {
    renderSidebar("h5");
    expect(screen.getByTestId("side-version").textContent).toBe("v0.9.0");
  });

  it("点版本位跳到 /changelog", () => {
    renderSidebar("pc");
    fireEvent.click(screen.getByTestId("side-version"));
    expect(screen.getByText("更新日志页")).toBeTruthy();
  });

  it("加载中不渲染（不出占位符/空链接）", () => {
    version.mockReturnValue({ data: null, loading: true, error: null, reload: () => undefined });
    renderSidebar("pc");
    expect(screen.queryByTestId("side-version")).toBeNull();
  });

  it("接口失败不渲染（拿不到可信版本号就不显示）", () => {
    version.mockReturnValue({
      data: null,
      loading: false,
      error: new Error("boom"),
      reload: () => undefined,
    });
    renderSidebar("pc");
    expect(screen.queryByTestId("side-version")).toBeNull();
    expect(document.body.textContent).not.toContain("vundefined");
  });

  it("data 为空但既不在加载也无错误，同样不渲染", () => {
    version.mockReturnValue({ data: null, loading: false, error: null, reload: () => undefined });
    renderSidebar("h5");
    expect(screen.queryByTestId("side-version")).toBeNull();
    expect(document.body.textContent).not.toContain("vundefined");
  });

  it("dev 构建照实说「开发构建」，不编版本号", () => {
    version.mockReturnValue({
      data: { version: "dev", commit: "abc1234", builtAt: "unknown" },
      loading: false,
      error: null,
      reload: () => undefined,
    });
    renderSidebar("pc");
    expect(screen.getByTestId("side-version").textContent).toBe("开发构建");
  });
});

describe("versionLabel：版本位文案的纯函数口径", () => {
  it("正常版本补 v 前缀", async () => {
    const { versionLabel } = await import("./versionText");
    expect(versionLabel("0.9.0", false, false)).toBe("v0.9.0");
  });

  it("加载中 / 失败 / 空值一律 null（调用方不渲染整行）", async () => {
    const { versionLabel } = await import("./versionText");
    expect(versionLabel("0.9.0", true, false)).toBeNull();
    expect(versionLabel("0.9.0", false, true)).toBeNull();
    expect(versionLabel(undefined, false, false)).toBeNull();
    expect(versionLabel("", false, false)).toBeNull();
  });

  it("dev 不写成 vdev", async () => {
    const { versionLabel } = await import("./versionText");
    expect(versionLabel("dev", false, false)).toBe("开发构建");
  });
});
