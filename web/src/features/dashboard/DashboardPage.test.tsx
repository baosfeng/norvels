import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastHost } from "../../lib/toast";

const reloads = {
  stats: vi.fn(),
  health: vi.fn(),
  jobs: vi.fn(),
  trend: vi.fn(),
  topAuthors: vi.fn(),
  reading: vi.fn(),
  progress: vi.fn(),
  progressAll: vi.fn(),
};

const state = {
  stats: {
    data: {
      books: 19,
      chapters: 11668,
      authors: 17,
      words: 30_418_864,
      dbBytes: 326_553_600,
      lastImportAt: "2026-10-24T20:25:26.409Z",
      textBytes: 101_563_175,
      textBytesRaw: 300_000_000,
      indexBytes: 73_533_895,
      otherBytes: 151_456_530,
      stale: false,
    },
    loading: false,
    error: null as unknown,
  },
  health: {
    data: {
      totalBooks: 19,
      thinBooks: 2,
      missingAuthor: 3,
      missingIntro: 4,
      hugeChapters: 1,
      chapterBuckets: [],
      chapterLengthBuckets: [],
      readingBuckets: { unread: 10, reading: 4, finished: 5 },
      stale: false,
    },
    loading: false,
    error: null as unknown,
  },
};

vi.mock("../../api/hooks", () => ({
  useStats: () => ({ ...state.stats, reload: reloads.stats }),
  useHealthReport: () => ({ ...state.health, reload: reloads.health }),
  useImportJobs: () => ({ data: { items: [] }, loading: false, error: null, reload: reloads.jobs }),
  useImportTrend: () => ({ data: [], loading: false, error: null, reload: reloads.trend }),
  useTopAuthors: () => ({ data: { items: [] }, loading: false, error: null, reload: reloads.topAuthors }),
  useReadingStats: () => ({
    data: { totalSeconds: 0, todaySeconds: 0, weekSeconds: 0, streak: 0, days: [] },
    loading: false,
    error: null,
    reload: reloads.reading,
  }),
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: reloads.progress }),
  useReadingProgressAll: () => ({ data: { items: [] }, loading: false, error: null, reload: reloads.progressAll }),
  useHealthIssues: () => ({
    data: {
      kind: "missing_author",
      total: 3,
      page: 1,
      size: 50,
      items: [{ bookId: 7, title: "缺作者之书", author: "", chapterCount: 12, wordCount: 345_600 }],
    },
    loading: false,
    error: null,
    reload: reloads.health,
  }),
}));

const { DashboardPage } = await import("./DashboardPage");

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/dashboard"]}>
      <DashboardPage />
      <ToastHost />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  for (const fn of Object.values(reloads)) fn.mockReset();
});

describe("驾驶舱锚点", () => {
  it("C-23 / C-24 / C-25 三个入口都挂了锚点并指向正确去处", () => {
    renderPage();
    expect(screen.getByText("全部书架 →").getAttribute("data-od-id")).toBe("C-23");
    expect(screen.getByText("全部书架 →").getAttribute("href")).toBe("/shelf");
    expect(screen.getByText("前往导入管理 →").getAttribute("data-od-id")).toBe("C-24");
    expect(screen.getByText("前往导入管理 →").getAttribute("href")).toBe("/import");
    expect(document.querySelectorAll('[data-od-id="C-25"]')).toHaveLength(0);
  });

  it("C-27 / C-28（新增）健康卡给出下一步入口", () => {
    renderPage();
    const admin = document.querySelector('[data-od-id="C-27"]') as HTMLAnchorElement;
    const importer = document.querySelector('[data-od-id="C-28"]') as HTMLAnchorElement;
    expect(admin.getAttribute("href")).toBe("/admin");
    expect(admin.textContent).toBe("去书籍管理");
    expect(importer.getAttribute("href")).toBe("/import");
    expect(importer.textContent).toBe("去导入");
  });

  it("C-29 刷新概览：按钮变「正在刷新…」并禁用，重取全部数据源，完成后提示", async () => {
    renderPage();
    const button = document.querySelector('[data-od-id="C-29"]') as HTMLButtonElement;
    expect(button.textContent).toBe("刷新概览");
    fireEvent.click(button);
    expect((document.querySelector('[data-od-id="C-29"]') as HTMLButtonElement).textContent).toBe("正在刷新…");
    expect(reloads.stats).toHaveBeenCalled();
    expect(reloads.health).toHaveBeenCalled();
    expect(reloads.reading).toHaveBeenCalled();
    await waitFor(() => expect(screen.getByTestId("toast").textContent).toContain("概览已刷新"), { timeout: 1500 });
    expect((document.querySelector('[data-od-id="C-29"]') as HTMLButtonElement).textContent).toBe("刷新概览");
  });

  it("概览读不到时给错误态与重试（不渲染数据区）", () => {
    const saved = state.stats.data;
    state.stats.loading = false;
    state.stats.error = new Error("离线");
    state.stats.data = null as unknown as typeof state.stats.data;
    renderPage();
    expect(screen.getByText("概览没能加载")).toBeTruthy();
    // 还原全局桩，避免后续用例拿到错误态
    state.stats.error = null;
    state.stats.data = saved;
  });
});

describe("回归：C-23「全部书架」要清掉上次筛选", () => {
  it("链接带上 resetFilters，落到书架时 state 可读", async () => {
    function ShelfProbe() {
      const location = useLocation();
      return <div data-testid="shelf-state">{JSON.stringify(location.state)}</div>;
    }
    render(
      <MemoryRouter initialEntries={["/dashboard"]}>
        <Routes>
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/shelf" element={<ShelfProbe />} />
        </Routes>
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByText("全部书架 →"));
    await waitFor(() => expect(screen.getByTestId("shelf-state")).toBeTruthy());
    expect(screen.getByTestId("shelf-state").textContent).toContain('"resetFilters":true');
  });
});
describe("健康卡下钻（新增）", () => {
  it("四类问题各有「查看 N 本/个」入口，点开抽屉列出明细并链到单书管理页", () => {
    renderPage();
    expect(screen.getByTestId("health-entry-missing_author").textContent).toContain("查看 3 本");
    expect(screen.getByTestId("health-entry-missing_intro").textContent).toContain("查看 4 本");
    expect(screen.getByTestId("health-entry-huge_chapter").textContent).toContain("查看 1 个");
    expect(screen.getByTestId("health-entry-thin_book").textContent).toContain("查看 2 本");

    fireEvent.click(screen.getByTestId("health-entry-missing_author"));
    expect(screen.getByTestId("health-drawer").getAttribute("data-kind")).toBe("missing_author");
    expect(screen.getByTestId("health-drawer-title").textContent).toBe("缺作者的书");
    // 条目标题就是入口：点进单书管理页
    expect(screen.getByTestId("health-issue-7").getAttribute("href")).toBe("/admin/books/7");
  });

  it("关掉抽屉后回到健康卡", () => {
    renderPage();
    fireEvent.click(screen.getByTestId("health-entry-huge_chapter"));
    expect(screen.getByTestId("health-drawer")).toBeTruthy();
    fireEvent.click(screen.getByTestId("health-drawer-close"));
    expect(screen.queryByTestId("health-drawer")).toBeNull();
  });

  it("Esc 关闭抽屉", () => {
    renderPage();
    fireEvent.click(screen.getByTestId("health-entry-thin_book"));
    expect(screen.getByTestId("health-drawer")).toBeTruthy();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(screen.queryByTestId("health-drawer")).toBeNull();
  });

  it("没有这类问题时入口禁用，并给出这一类专属的一句说明", () => {
    const saved = state.health.data;
    state.health.data = { ...saved, missingAuthor: 0 };
    renderPage();
    const entry = screen.getByTestId("health-entry-missing_author") as HTMLButtonElement;
    expect(entry.disabled).toBe(true);
    expect(entry.textContent).toBe("没有缺作者的书");
    // 另一类不受影响：各自一句，不共用文案
    expect(screen.getByTestId("health-entry-missing_intro").textContent).toContain("查看 4 本");
    state.health.data = saved;
  });
});
