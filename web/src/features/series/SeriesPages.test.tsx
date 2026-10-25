import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookSummary, SeriesDetail, SeriesPage as SeriesPageData } from "../../api/types";

const seriesList = vi.fn();
const seriesDetail = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      seriesList: (params: unknown, opts?: unknown) => seriesList(params, opts),
      seriesDetail: (name: string, opts?: unknown) => seriesDetail(name, opts),
    },
  };
});

const { SeriesPage } = await import("./SeriesPage");
const { SeriesDetailPage } = await import("./SeriesDetailPage");

function summary(over: Partial<SeriesPageData["items"][number]> = {}) {
  return {
    name: "示例系列",
    bookCount: 3,
    wordCount: 120000,
    lastUpdatedAt: "2026-10-24T12:00:00Z",
    coverBookId: 0,
    ...over,
  };
}

function book(over: Partial<BookSummary> = {}): BookSummary {
  return {
    id: 1,
    title: "示例书",
    authorId: 1,
    authorName: "示例作者",
    chapterCount: 10,
    wordCount: 1000,
    volumeCount: 1,
    intro: "",
    status: "parsed",
    importedAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    ...over,
  };
}

function renderSeries() {
  return render(
    <MemoryRouter initialEntries={["/series"]}>
      <Routes>
        <Route path="/series" element={<SeriesPage />} />
        <Route path="/series/:name" element={<SeriesDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  seriesList.mockReset();
  seriesDetail.mockReset();
});

describe("系列列表页", () => {
  it("正常：卡片显示书数 / 字数 / 最近更新，点进去就是详情", async () => {
    seriesList.mockResolvedValue({
      items: [summary({ name: "示例系列" }), summary({ name: "另一个系列", bookCount: 2 })],
      total: 2,
    });
    renderSeries();

    await waitFor(() => expect(screen.getByTestId("series-live")).toBeTruthy());
    const card = screen.getByTestId("series-card-示例系列");
    expect(card.textContent).toContain("3 本");
    expect(card.textContent).toContain("12 万 字");
    expect(card.textContent).toContain("最近更新");
    expect(card.querySelector("a")?.getAttribute("href")).toBe("/series/" + encodeURIComponent("示例系列"));
    expect(seriesList.mock.calls[0][0]).toMatchObject({ page: 1, size: 24 });
  });

  it("搜索：提交后重新取数并去掉首尾空白", async () => {
    seriesList.mockResolvedValue({ items: [summary()], total: 1 });
    renderSeries();
    await waitFor(() => expect(seriesList).toHaveBeenCalledTimes(1));

    fireEvent.change(screen.getByTestId("series-search-input"), { target: { value: "  示例  " } });
    fireEvent.click(screen.getByRole("button", { name: "搜索" }));

    await waitFor(() => expect(seriesList).toHaveBeenCalledTimes(2));
    expect(seriesList.mock.calls[1][0]).toMatchObject({ q: "示例", page: 1 });
    // 有关键词时才出现「清除」
    expect(screen.getByTestId("series-search-clear")).toBeTruthy();
  });

  it("分页：总数超过一页时出现分页控件，翻页带 page", async () => {
    seriesList.mockResolvedValue({ items: [summary()], total: 50 });
    renderSeries();
    await waitFor(() => expect(screen.getByTestId("series-pagination")).toBeTruthy());
    expect(screen.getByTestId("series-pagination").textContent).toContain("/ 3");

    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    await waitFor(() => expect(seriesList).toHaveBeenCalledTimes(2));
    expect(seriesList.mock.calls[1][0]).toMatchObject({ page: 2 });
  });

  it("空态：没有系列时给出说明而不是空白", async () => {
    seriesList.mockResolvedValue({ items: [], total: 0 });
    renderSeries();
    await waitFor(() => expect(screen.getByTestId("series-empty")).toBeTruthy());
    expect(screen.getByTestId("series-empty").textContent).toContain("还没有系列");
  });

  it("错误态：失败后可重试", async () => {
    seriesList.mockRejectedValueOnce(new Error("离线")).mockResolvedValueOnce({ items: [summary()], total: 1 });
    renderSeries();
    await waitFor(() => expect(screen.getByTestId("series-error")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await waitFor(() => expect(screen.getByTestId("series-live")).toBeTruthy());
  });
});

describe("系列详情页", () => {
  function renderDetail(name = "示例系列") {
    return render(
      <MemoryRouter initialEntries={["/series/" + encodeURIComponent(name)]}>
        <Routes>
          <Route path="/series/:name" element={<SeriesDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );
  }

  it("按系列序号排书目，卡片带「第 N 部」", async () => {
    const detail: SeriesDetail = {
      name: "示例系列",
      books: [book({ id: 2, title: "第二部", seriesIndex: 2 }), book({ id: 1, title: "第一部", seriesIndex: 1 })],
    };
    seriesDetail.mockResolvedValue(detail);
    renderDetail();

    await waitFor(() => expect(screen.getByTestId("series-detail-live")).toBeTruthy());
    expect(seriesDetail.mock.calls[0][0]).toBe("示例系列");
    const cards = screen.getByTestId("series-detail-live").querySelectorAll(".book-card");
    expect(cards).toHaveLength(2);
    expect(cards[0].textContent).toContain("第一部");
    expect(screen.getByTestId("series-book-order-1").textContent).toContain("系列内第 1 本");
  });

  it("系列不存在（404）走空态而不是错误态", async () => {
    const { ApiError } = await vi.importActual<typeof import("../../api/client")>("../../api/client");
    seriesDetail.mockRejectedValue(new ApiError("http", "not found", 404, "not_found"));
    renderDetail("没有的系列");

    await waitFor(() => expect(screen.getByTestId("series-detail-empty")).toBeTruthy());
    expect(screen.getByTestId("series-detail-empty").textContent).toContain("没有这个系列");
  });
});
