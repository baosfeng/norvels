import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { BookSummary, Page } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      authors: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 20 })),
      tags: vi.fn(async () => ({ items: [] })),
      collections: vi.fn(async () => ({ items: [] })),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

function pageOf(): Page<BookSummary> {
  return {
    items: [
      {
        id: 1,
        title: "示例书",
        authorId: 1,
        authorName: "示例作者",
        chapterCount: 10,
        wordCount: 12000,
        volumeCount: 1,
        intro: "",
        status: "parsed",
        rating: 4,
        importedAt: "2026-10-24T20:24:50.859Z",
        updatedAt: "2026-10-24T20:24:50.859Z",
      },
    ],
    total: 1,
    page: 1,
    size: 24,
  };
}

function renderShelf() {
  return render(
    <TierProvider value="pc">
      <MemoryRouter>
        <ShelfPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  books.mockReset();
  books.mockResolvedValue(pageOf());
  window.sessionStorage.clear();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("书架：评分筛选与按评分排序", () => {
  it("工具栏能按评分筛选，请求带 rating（1–5）", async () => {
    renderShelf();
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    expect(books.mock.calls[0][0].rating).toBeUndefined();

    fireEvent.change(screen.getByTestId("filter-rating"), { target: { value: "4" } });
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ rating: 4, page: 1 });
    // 评分口径提示只在筛了评分时出现
    expect(screen.getByTestId("rating-filter-hint").textContent).toContain("0 星 = 未评");
  });

  it("评分选项只有 1–5 星，没有「未评」（后端不支持筛未评）", async () => {
    renderShelf();
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    const options = Array.from((screen.getByTestId("filter-rating") as HTMLSelectElement).options).map((o) => o.textContent);
    expect(options).toEqual(["全部评分", "5 星", "4 星", "3 星", "2 星", "1 星"]);
  });

  it("排序可选「评分」并下发 sort=rating", async () => {
    renderShelf();
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    const sortSelect = screen.getByLabelText("排序") as HTMLSelectElement;
    fireEvent.change(sortSelect, { target: { value: "rating" } });

    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));
    expect(books.mock.calls[1][0]).toMatchObject({ sort: "rating" });
  });

  it("清除筛选把评分也一起还原", async () => {
    renderShelf();
    await waitFor(() => expect(books).toHaveBeenCalledTimes(1));
    fireEvent.change(screen.getByTestId("filter-rating"), { target: { value: "2" } });
    await waitFor(() => expect(books).toHaveBeenCalledTimes(2));

    fireEvent.click(screen.getAllByRole("button", { name: "清除筛选" })[0]);
    await waitFor(() => expect(books).toHaveBeenCalledTimes(3));
    expect(books.mock.calls[2][0].rating).toBeUndefined();
    expect(screen.queryByTestId("rating-filter-hint")).toBeNull();
  });
});
