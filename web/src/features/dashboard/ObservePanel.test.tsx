import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { MetricsSnapshot, RouteTiming } from "../../api/types";
import { ObservePanel } from "./ObservePanel";

/** 造一条路由读数：只关心耗时排名的用例不需要关心桶。 */
function route(name: string, p95: number, extra: Partial<RouteTiming> = {}): RouteTiming {
  return {
    route: name,
    count: 10,
    errors: 0,
    clientErrors: 0,
    avgMs: p95 / 2,
    maxMs: p95,
    p50Ms: p95 / 3,
    p95Ms: p95,
    p99Ms: p95,
    buckets: [],
    ...extra,
  };
}

function snapshot(overrides: Partial<MetricsSnapshot> = {}): MetricsSnapshot {
  return {
    startedAt: "2026-11-07T00:00:00Z",
    uptimeSec: 120,
    http: [route("GET /api/books", 40), route("GET /api/health/report", 900)],
    slowRequests: [
      {
        at: "2026-11-07T00:01:00Z",
        route: "GET /api/health/report",
        method: "GET",
        path: "/api/health/report",
        status: 200,
        durMs: 900,
        sqlCount: 7,
        sqlMs: 850,
        requestId: "rid-1",
      },
    ],
    sql: [],
    runtime: {
      goVersion: "go1.27.0",
      goroutines: 11,
      heapMB: 2.5,
      numGC: 3,
      pools: [{ name: "main", open: 8, inUse: 1, idle: 7, waitCount: 0, waitMs: 0 }],
    },
    ...overrides,
  };
}

function renderPanel(props: Partial<Parameters<typeof ObservePanel>[0]> = {}) {
  return render(
    <ObservePanel snapshot={snapshot()} loading={false} error={null} onRetry={vi.fn()} {...props} />,
  );
}

describe("接口耗时面板", () => {
  it("按 p95 降序：最慢的路由排第一，读数带毫秒", () => {
    renderPanel();
    const table = screen.getByTestId("metrics-routes");
    const rows = within(table).getAllByRole("row").slice(1); // 去掉表头
    expect(rows[0].textContent).toContain("GET /api/health/report");
    expect(rows[1].textContent).toContain("GET /api/books");
    expect(table.textContent).toContain("900ms");
  });

  it("只展示 p95 最高的前 10 条，其余不进表（全量走接口）", () => {
    const http = Array.from({ length: 14 }, (_, i) => route(`GET /api/x${i}`, (i + 1) * 10));
    renderPanel({ snapshot: snapshot({ http }) });
    const rows = within(screen.getByTestId("metrics-routes")).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(10);
    expect(rows[0].textContent).toContain("GET /api/x13");
    expect(screen.queryByText("GET /api/x0")).toBeNull();
  });

  it("慢请求表给出时间、方法+路径、状态、耗时与 SQL 次数", () => {
    renderPanel();
    const slow = screen.getByTestId("metrics-slow");
    expect(slow.textContent).toContain("GET /api/health/report");
    expect(slow.textContent).toContain("200");
    expect(slow.textContent).toContain("900ms");
    expect(slow.textContent).toContain("7 次");
  });

  it("没有慢请求时给一句阈值说明，不画空表", () => {
    renderPanel({ snapshot: snapshot({ slowRequests: [] }) });
    expect(screen.getByTestId("metrics-slow-empty").textContent).toContain("阈值内没有慢请求");
  });

  it("运行时摘要带 goroutines / 堆 / 连接池占用", () => {
    renderPanel();
    const line = screen.getByTestId("metrics-runtime").textContent ?? "";
    expect(line).toContain("goroutines 11");
    expect(line).toContain("2.5MB");
    expect(line).toContain("main 1/8");
  });

  it("一条读数都没有时给空态", () => {
    renderPanel({ snapshot: snapshot({ http: [] }) });
    expect(screen.getByTestId("metrics-empty")).toBeTruthy();
  });

  it("读取出错给错误态与重试", () => {
    const onRetry = vi.fn();
    renderPanel({ snapshot: null, error: new ApiError("http", "炸了", 500, "internal"), onRetry });
    const panel = screen.getByTestId("metrics-error");
    expect(panel.textContent).toContain("接口耗时没能加载");
    within(panel).getByRole("button", { name: "重试" }).click();
    expect(onRetry).toHaveBeenCalled();
  });
});
