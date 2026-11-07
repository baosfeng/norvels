import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DashboardPayload } from "../../api/types";
import { DashboardPage } from "./DashboardPage";

/**
 * 驾驶舱的请求数口径（本轮接入 GET /api/dashboard 的核心收益）。
 *
 * 这里**不 mock hooks**：走真实 api 客户端，只把 fetch 换成一个记账的替身，
 * 于是「首屏到底发了几个业务请求」是从调用链里数出来的，不是照着代码猜的。
 * 旧实现首屏要发 10 个（stats / health / import-trend / reading / progress×2 /
 * authors / import-jobs / metrics / recommend），聚合端点接上后是 3 个。
 * 侧栏的 /api/stats、/api/version 与 /api/auth/me 属于外壳，不在本页统计口径里
 * —— 本文件只渲染 DashboardPage，外壳不挂载。
 */

const payload: DashboardPayload = {
  generatedAt: "2026-11-07T00:00:00Z",
  cached: false,
  ageSeconds: 0,
  ttlSeconds: 5,
  warnings: [],
  stats: {
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
  },
  health: {
    totalBooks: 19,
    thinBooks: 2,
    missingAuthor: 3,
    missingIntro: 4,
    hugeChapters: 1,
    chapterBuckets: [{ label: "1", count: 3 }],
    chapterLengthBuckets: [{ label: "<1千", count: 4 }],
    warnings: [],
  },
  importTrend: [{ date: "2026-10-24", books: 19, chapters: 11668 }],
  readingStats: { totalSeconds: 3600, todaySeconds: 600, weekSeconds: 1200, streak: 1, days: [] },
  topAuthors: { items: [{ id: 1, name: "演示作者", bookCount: 2, wordCount: 1200 }], total: 1, page: 1, size: 10 },
  readingProgress: [],
  importJobs: [],
};

const metrics = {
  startedAt: "2026-11-07T00:00:00Z",
  uptimeSec: 60,
  http: [],
  slowRequests: [],
  sql: [],
  runtime: { goVersion: "go1.27.0", goroutines: 8, heapMB: 2.4, numGC: 1, pools: [] },
};

const recommend = {
  summary: { days: 14, impressions: 0, clicks: 0, reads: 0, readSeconds: 0, shelfAdds: 0 },
  daily: [],
  interests: [],
  hotBooks: [],
};

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

/** 首屏请求过的业务路径（按 /api 前缀切出来，便于断言集合）。 */
let paths: string[] = [];

beforeEach(() => {
  paths = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      const path = url.split("?")[0];
      paths.push(path);
      if (path === "/api/dashboard") return Promise.resolve(json(payload));
      if (path === "/api/metrics") return Promise.resolve(json(metrics));
      if (path === "/api/recommend/stats") return Promise.resolve(json(recommend));
      return Promise.resolve(json({}));
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/dashboard"]}>
      <DashboardPage />
    </MemoryRouter>,
  );
}

describe("驾驶舱的请求数", () => {
  it("首屏只发 3 个业务请求：dashboard + metrics + recommend/stats", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByTestId("chart:storage")).toBeTruthy());
    expect(paths.slice().sort()).toEqual(["/api/dashboard", "/api/metrics", "/api/recommend/stats"]);
  });

  it("点「刷新概览」再发 3 个：与首屏同一组，不逐个重取旧端点", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByTestId("chart:storage")).toBeTruthy());
    paths = [];
    fireEvent.click(screen.getByRole("button", { name: "刷新概览" }));
    await waitFor(() => expect(paths).toHaveLength(3));
    expect(paths.slice().sort()).toEqual(["/api/dashboard", "/api/metrics", "/api/recommend/stats"]);
  });
});
