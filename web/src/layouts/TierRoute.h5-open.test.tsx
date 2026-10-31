import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReadingHistoryResult } from "../api/types";
import { TierProvider } from "../lib/tier";
import { routeTier } from "../lib/tier";
import { TierRoute } from "./TierRoute";

const readingHistory = vi.fn(async (): Promise<ReadingHistoryResult> => ({ items: [], total: 0 }));
const seriesList = vi.fn(async () => ({ items: [], total: 0 }));
const seriesDetail = vi.fn(async () => ({ name: "示例系列", books: [] }));

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      readingHistory: () => readingHistory(),
      seriesList: () => seriesList(),
      seriesDetail: () => seriesDetail(),
    },
  };
});

const { SeriesPage } = await import("../features/series/SeriesPage");
const { SeriesDetailPage } = await import("../features/series/SeriesDetailPage");
const { HistoryPage } = await import("../features/history/HistoryPage");

function Probe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}</div>;
}

/** 按真实路由表的方式挂载：TierRoute 只给一级段名，档位由 tier.ts 查表。 */
function renderAt(segment: string, path: string, element: React.ReactNode, tier: "pc" | "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route
            path={path}
            element={
              <TierRoute segment={segment}>
                {element}
              </TierRoute>
            }
          />
          <Route path="/home" element={<p>H5 首页</p>} />
          <Route path="/shelf" element={<p>书架页</p>} />
        </Routes>
        <Probe />
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  readingHistory.mockClear();
  seriesList.mockClear();
  seriesDetail.mockClear();
});

describe("H5 开放系列与阅读历史（TierRoute 放行）", () => {
  it("档位表：series / history 两档都开，管理四页才是 PC 专属", () => {
    expect(routeTier("series").minTier).toBeNull();
    expect(routeTier("history").minTier).toBeNull();
    // 书单页两档都开：书单管理不再挂在书架工具栏上，H5 也要有入口
    expect(routeTier("collections").minTier).toBeNull();
    expect(routeTier("import").minTier).toBe("pc");
    expect(routeTier("authors").minTier).toBe("pc");
    expect(routeTier("dashboard").minTier).toBe("pc");
    expect(routeTier("admin").minTier).toBe("pc");
    expect(routeTier("home").minTier).toBe("h5");
    expect(routeTier("shelf").minTier).toBeNull();
  });

  it("H5 打开 /series 渲染系列页，不被送回首页", async () => {
    renderAt("series", "/series", <SeriesPage />, "h5");
    await waitFor(() => expect(screen.getByTestId("view:series")).toBeTruthy());
    expect(screen.getByTestId("location").textContent).toBe("/series");
    expect(seriesList).toHaveBeenCalled();
  });

  it("H5 打开 /series/:name 渲染系列详情", async () => {
    renderAt("series", "/series/:name", <SeriesDetailPage />, "h5");
    await waitFor(() => expect(screen.getByTestId("view:series-detail")).toBeTruthy());
    expect(screen.getByTestId("location").textContent).not.toBe("/home");
  });

  it("H5 打开 /history 渲染阅读历史（卡片形态），不被送回首页", async () => {
    renderAt("history", "/history", <HistoryPage />, "h5");
    await waitFor(() => expect(screen.getByTestId("view:history")).toBeTruthy());
    expect(screen.getByTestId("location").textContent).toBe("/history");
    expect(readingHistory).toHaveBeenCalled();
    expect(document.querySelector("table")).toBeNull();
  });

  it("回归：管理页在 H5 仍被拦住（系列历史放开不等于全放开）", async () => {
    renderAt("dashboard", "/dashboard", <p>驾驶舱</p>, "h5");
    await waitFor(() => expect(screen.getByText("H5 首页")).toBeTruthy());
    expect(screen.getByTestId("location").textContent).toBe("/home");
  });

  it("回归：PC 访问首页仍落回书架", async () => {
    renderAt("home", "/home", <p>H5 首页内容</p>, "pc");
    await waitFor(() => expect(screen.getByText("书架页")).toBeTruthy());
    expect(screen.getByTestId("location").textContent).toBe("/shelf");
  });
});
