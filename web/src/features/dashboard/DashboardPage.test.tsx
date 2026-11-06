import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
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
  recommend: vi.fn(),
  metrics: vi.fn(),
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
  recommend: {
    data: {
      summary: { days: 14, impressions: 200, clicks: 18, reads: 6, readSeconds: 840, shelfAdds: 3 },
      daily: [
        { date: "2026-10-18", impressions: 120, clicks: 6, reads: 2, seconds: 240 },
        { date: "2026-10-19", impressions: 80, clicks: 12, reads: 4, seconds: 600 },
      ],
      interests: [
        { dim: "category_l1", key: "武侠", name: "武侠", weight: 4 },
        { dim: "category_l2", key: "江湖", name: "江湖", weight: 4 },
        { dim: "author", key: "12", name: "演示作者甲", weight: 3 },
      ],
      hotBooks: [{ bookId: 1, title: "剑影山河", authorName: "示例作者甲", reads: 3, clicks: 5, seconds: 600 }],
      reasonMix: [
        { reason: "因为你常看武侠", impressions: 150, clicks: 12 },
        { reason: "最近很多人读", impressions: 50, clicks: 6 },
      ],
      categoryClicks: [
        { categoryL1: "武侠", clicks: 9 },
        { categoryL1: "", clicks: 3 },
      ],
    } as unknown,
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
  useRecommendStats: () => ({ ...state.recommend, reload: reloads.recommend }),
  useMetrics: () => ({
    data: {
      startedAt: "2026-11-07T00:00:00Z",
      uptimeSec: 3600,
      http: [
        {
          route: "GET /api/health/report",
          count: 3,
          errors: 0,
          clientErrors: 0,
          avgMs: 1840.2,
          maxMs: 2100,
          p50Ms: 1800,
          p95Ms: 2050,
          p99Ms: 2100,
          buckets: [],
        },
        {
          route: "GET /api/books",
          count: 40,
          errors: 1,
          clientErrors: 2,
          avgMs: 42.5,
          maxMs: 610,
          p50Ms: 30,
          p95Ms: 180,
          p99Ms: 420,
          buckets: [],
        },
      ],
      slowRequests: [
        {
          at: "2026-11-07T00:05:00Z",
          route: "GET /api/health/report",
          method: "GET",
          path: "/api/health/report",
          status: 200,
          durMs: 2100,
          sqlCount: 7,
          sqlMs: 1980,
          requestId: "abc123",
        },
      ],
      sql: [],
      runtime: {
        goVersion: "go1.27.0",
        goroutines: 12,
        heapMB: 3.2,
        numGC: 4,
        pools: [{ name: "main", open: 8, inUse: 1, idle: 7, waitCount: 0, waitMs: 0 }],
      },
    },
    loading: false,
    error: null,
    reload: reloads.metrics,
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

describe("驾驶舱入口", () => {
  it("三个入口都指向正确去处，且不再渲染已删除的章节统计入口", () => {
    renderPage();
    expect(screen.getByText("全部书架 →").getAttribute("href")).toBe("/shelf");
    expect(screen.getByText("前往导入管理 →").getAttribute("href")).toBe("/import");
    // 章节统计入口已删除：这一整块不再渲染
    expect(screen.queryByText("章节统计")).toBeNull();
  });

  it("健康卡给出下一步入口", () => {
    renderPage();
    const admin = screen.getByRole("link", { name: "去书籍管理" }) as HTMLAnchorElement;
    const importer = screen.getByRole("link", { name: "去导入" }) as HTMLAnchorElement;
    expect(admin.getAttribute("href")).toBe("/admin");
    expect(admin.textContent).toBe("去书籍管理");
    expect(importer.getAttribute("href")).toBe("/import");
    expect(importer.textContent).toBe("去导入");
  });

  it("刷新概览：按钮变「正在刷新…」并禁用，重取全部数据源，完成后提示", async () => {
    renderPage();
    const button = screen.getByRole("button", { name: "刷新概览" }) as HTMLButtonElement;
    expect(button.textContent).toBe("刷新概览");
    fireEvent.click(button);
    expect((screen.getByRole("button", { name: "正在刷新…" }) as HTMLButtonElement).disabled).toBe(true);
    expect(reloads.stats).toHaveBeenCalled();
    expect(reloads.health).toHaveBeenCalled();
    expect(reloads.reading).toHaveBeenCalled();
    expect(reloads.metrics).toHaveBeenCalled();
    await waitFor(() => expect(screen.getByTestId("toast").textContent).toContain("概览已刷新"), { timeout: 1500 });
    expect((screen.getByRole("button", { name: "刷新概览" }) as HTMLButtonElement).disabled).toBe(false);
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

describe("回归：「全部书架」要清掉上次筛选", () => {
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

describe("推荐与埋点区块", () => {
  it("区块头有「去发现页」入口，指标卡行给出四项实测值", () => {
    renderPage();
    expect(screen.getByTestId("dashboard:recommend")).toBeTruthy();
    expect(screen.getByTestId("dashboard:discover-link").getAttribute("href")).toBe("/discover");

    const metrics = screen.getByTestId("recommend-metrics");
    // 「今日」按 UTC 日期匹配；测试数据里没有今天，退回窗口内最近的一天（80）
    expect(within(metrics).getAllByText("今日曝光")).toHaveLength(1);
    expect(screen.getByTestId("metric-impressions").textContent).toContain("80");
    // 18 / 200 = 9%
    expect(screen.getByTestId("metric-ctr").textContent).toContain("9%");
    // 840 秒 = 14 分钟
    expect(screen.getByTestId("metric-reads").textContent).toContain("14");
    expect(screen.getByTestId("metric-shelf").textContent).toContain("3");
  });

  it("六张图进同一个 chart-grid（与既有七张同一套卡片体系）", () => {
    renderPage();
    const charts = screen.getByTestId("recommend-charts");
    expect(charts.className).toContain("chart-grid");
    for (const id of ["events", "interest", "hot", "funnel", "reasonmix", "categoryclicks"]) {
      expect(screen.getByTestId(`chart:${id}`)).toBeTruthy();
    }
    // 既有区块没被挤掉：原来的七张图还在
    expect(screen.getByTestId("dashboard:charts").querySelectorAll(".chart-card")).toHaveLength(7);
  });

  it("两张分布图读出真数字：理由按点击排序、空分类归「未分类」", () => {
    renderPage();
    const reason = screen.getByTestId("chart:reasonmix");
    expect(reason.textContent).toContain("最抓人的是「因为你常看武侠」");
    expect(reason.textContent).toContain("12 点");
    const category = screen.getByTestId("chart:categoryclicks");
    expect(category.textContent).toContain("未分类");
    expect(category.textContent).toContain("点得最多的分类是「武侠」");
  });

  it("后端没给 reasonMix / categoryClicks 时两张图各自空态，其它图照常出", () => {
    const saved = state.recommend.data as Record<string, unknown>;
    state.recommend.data = { ...saved, reasonMix: undefined, categoryClicks: undefined } as unknown;
    renderPage();
    expect(screen.getByTestId("chart:reasonmix:empty")).toBeTruthy();
    expect(screen.getByTestId("chart:categoryclicks:empty")).toBeTruthy();
    // 上面四张不受影响
    expect(screen.getByTestId("chart:events")).toBeTruthy();
    expect(screen.queryByTestId("chart:events:empty")).toBeNull();
    state.recommend.data = saved;
  });

  it("一条埋点都没有时给整块空态 + 去发现页按钮，不画四张空图占两屏", () => {
    const saved = state.recommend.data;
    state.recommend.data = {
      summary: { days: 14, impressions: 0, clicks: 0, reads: 0, readSeconds: 0, shelfAdds: 0 },
      daily: [],
      interests: [],
      hotBooks: [],
    };
    renderPage();
    const empty = screen.getByTestId("recommend-empty");
    expect(empty.textContent).toContain("还没有埋点数据");
    expect(within(empty).getByRole("link", { name: "去发现页" }).getAttribute("href")).toBe("/discover");
    expect(screen.queryByTestId("recommend-charts")).toBeNull();
    expect(screen.queryByTestId("recommend-metrics")).toBeNull();
    state.recommend.data = saved;
  });

  it("读不到埋点数据（推荐库未启用 503）给错误态，重试按钮重取", () => {
    const saved = { data: state.recommend.data, error: state.recommend.error };
    state.recommend.data = null;
    state.recommend.error = new ApiError("http", "推荐库未启用", 503, "unavailable");
    renderPage();
    expect(screen.getByTestId("recommend-error")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(reloads.recommend).toHaveBeenCalled();
    state.recommend.data = saved.data;
    state.recommend.error = saved.error;
  });

  it("刷新概览会把埋点数据一起重取", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "刷新概览" }));
    expect(reloads.recommend).toHaveBeenCalled();
    await waitFor(() => expect(screen.getByTestId("toast").textContent).toContain("概览已刷新"), { timeout: 1500 });
  });
});

describe("健康卡下钻", () => {
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
