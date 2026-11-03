import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReleaseEntry, ReleaseNotes, VersionInfo } from "../../api/types";

const releases = vi.fn();
const version = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      releases: (opts?: unknown) => releases(opts),
      version: (opts?: unknown) => version(opts),
    },
  };
});

const { ReleaseNotesPage } = await import("./ReleaseNotesPage");

function entry(over: Partial<ReleaseEntry> = {}): ReleaseEntry {
  return {
    version: "0.9.0",
    date: "2026-11-02",
    retro: false,
    sections: { added: ["甲功能"], changed: [], fixed: [], upgrade: [] },
    ...over,
  };
}

function notes(items: ReleaseEntry[], current = ""): ReleaseNotes {
  return { current, items };
}

function info(v = "0.9.0"): VersionInfo {
  return { version: v, commit: "abc1234", builtAt: "2026-11-02T10:00:00Z" };
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ReleaseNotesPage />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  releases.mockReset();
  version.mockReset();
  version.mockResolvedValue(info());
});

describe("更新日志页", () => {
  it("按后端给的倒序渲染每个版本", async () => {
    releases.mockResolvedValue(
      notes([entry(), entry({ version: "0.8.1", date: "2026-11-01", retro: true })]),
    );
    renderPage();
    const items = await screen.findAllByTestId(/^release:0\./);
    expect(items.map((el) => el.getAttribute("data-testid"))).toEqual(["release:0.9.0", "release:0.8.1"]);
  });

  it("当前版本打「正在使用」标，其它版本没有", async () => {
    releases.mockResolvedValue(
      notes([entry(), entry({ version: "0.8.1", date: "2026-11-01" })], "0.9.0"),
    );
    renderPage();
    await screen.findByTestId("releases-list");
    expect(screen.getByTestId("releases-current-badge")).toBeTruthy();
    expect(screen.getByTestId("release:0.9.0").getAttribute("data-current")).toBe("true");
    expect(screen.getByTestId("release:0.8.1").getAttribute("data-current")).toBeNull();
  });

  it("页面顶部显示当前运行版本（取自 /api/version）", async () => {
    releases.mockResolvedValue(notes([entry()]));
    renderPage();
    expect((await screen.findByTestId("releases-running-version")).textContent).toBe("v0.9.0");
  });

  it("dev 构建显示 dev 而不是一个假版本号", async () => {
    releases.mockResolvedValue(notes([entry()]));
    version.mockResolvedValue(info("dev"));
    renderPage();
    expect((await screen.findByTestId("releases-running-version")).textContent).toContain("dev");
  });

  it("升级注意单独成组，回溯版本带来源标记", async () => {
    releases.mockResolvedValue(
      notes([entry({ retro: true, sections: { added: [], changed: [], fixed: [], upgrade: ["先备份"] } })]),
    );
    renderPage();
    const group = await screen.findByTestId("release-group:upgrade");
    expect(group.textContent).toContain("升级注意");
    expect(group.textContent).toContain("先备份");
    expect(screen.getByText("据 git 历史回溯")).toBeTruthy();
  });

  it("条目按 markdown 渲染：粗体变成 strong，页面上不再有字面星号", async () => {
    releases.mockResolvedValue(
      notes([
        entry({
          sections: { added: ["**连续翻章**：往下翻就是下一章"], changed: [], fixed: [], upgrade: [] },
        }),
      ]),
    );
    renderPage();
    const list = await screen.findByTestId("releases-list");
    expect(list.querySelector("strong")?.textContent).toBe("连续翻章");
    expect(list.textContent).not.toContain("**");
  });

  it("版本目录列出每个版本，当前版本带「在用」标记", async () => {
    releases.mockResolvedValue(
      notes([entry(), entry({ version: "0.8.1", date: "2026-11-01" })], "0.9.0"),
    );
    renderPage();
    const nav = await screen.findByTestId("releases-nav");
    expect(nav.querySelectorAll("a")).toHaveLength(2);
    expect(nav.querySelector("a[aria-current='location']")?.textContent).toContain("在用");
  });

  it("说明文件为空时给空态，而不是空白页", async () => {
    releases.mockResolvedValue(notes([]));
    renderPage();
    expect(await screen.findByTestId("releases-empty")).toBeTruthy();
  });

  it("接口失败给错误态与重试入口", async () => {
    const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
    // 后端解析失败时是 500 + code=internal；前端 ApiError 的 kind 归类为 http，code 在 .code 上
    releases.mockRejectedValue(new actual.ApiError("http", "发版说明不可用", 500, "internal"));
    renderPage();
    expect(await screen.findByTestId("releases-error")).toBeTruthy();
    expect(screen.getByText("重试")).toBeTruthy();
  });
});
