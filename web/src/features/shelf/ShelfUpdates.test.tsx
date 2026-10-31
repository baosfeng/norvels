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
      readingProgress: vi.fn(async () => ({ items: [] })),
      authors: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 20 })),
      tags: vi.fn(async () => ({ items: [] })),
      collections: vi.fn(async () => ({ items: [], total: 0 })),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

function bookAt(index: number, over: Partial<BookSummary> = {}): BookSummary {
  return {
    id: index,
    title: `示例书 ${index}`,
    authorId: 1,
    authorName: "示例作者",
    chapterCount: 10,
    wordCount: 12000,
    volumeCount: 1,
    intro: "",
    status: "parsed",
    importedAt: "2026-10-24T20:24:50.859Z",
    updatedAt: "2026-10-24T20:24:50.859Z",
    ...over,
  };
}

function renderShelf(tier: "pc" | "h5" = "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={["/shelf"]}>
        <ShelfPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

function cardIds(): string[] {
  return [...screen.getByTestId("shelf-live").querySelectorAll(".book-card")].map(
    (card) => card.getAttribute("data-book-id") ?? "",
  );
}

beforeEach(() => {
  window.sessionStorage.clear();
  books.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, size: 30 });
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

describe("H5 书架：追更角标", () => {
  it("newChapters>0 的卡片在封面右上角标「+N」，0 的不标", async () => {
    books.mockResolvedValue({
      items: [bookAt(1, { newChapters: 3 }), bookAt(2, { newChapters: 0 })],
      total: 2,
      page: 1,
      size: 30,
    });
    renderShelf();

    const badge = await screen.findByTestId("book-update-1");
    expect(badge.textContent).toContain("+3");
    // 角标是纯信息：读屏念「有 N 章更新」，点在它上面照样进详情页
    expect(badge.textContent).toContain("有 3 章更新");
    expect(screen.queryByTestId("book-update-2")).toBeNull();
  });

  it("章数多到写不下「+N」时退化成「更新」两个字", async () => {
    books.mockResolvedValue({ items: [bookAt(1, { newChapters: 120 })], total: 1, page: 1, size: 30 });
    renderShelf();

    const badge = await screen.findByTestId("book-update-1");
    expect(badge.textContent).toContain("更新");
    expect(badge.textContent).not.toContain("+120");
  });

  it("PC 卡不改：角标仍是信息行里的「新章 +N」，封面角上不重复出现", async () => {
    books.mockResolvedValue({ items: [bookAt(1, { newChapters: 3 })], total: 1, page: 1, size: 24 });
    renderShelf("pc");

    expect(await screen.findByTestId("book-new-chapters-1")).toBeTruthy();
    expect(screen.queryByTestId("book-update-1")).toBeNull();
  });
});

describe("书架：「N 本书有更新」汇总与筛选", () => {
  it("没有更新的书时整条不出现", async () => {
    books.mockResolvedValue({
      items: [bookAt(1), bookAt(2)],
      total: 2,
      page: 1,
      size: 30,
    });
    renderShelf();

    await waitFor(() => expect(screen.getByTestId("shelf-live")).toBeTruthy());
    expect(screen.queryByTestId("shelf-updates")).toBeNull();
  });

  it("数量只算有更新的书，点一下把列表筛到这些书，再点回到全部", async () => {
    books.mockResolvedValue({
      items: [bookAt(1, { newChapters: 2 }), bookAt(2), bookAt(3, { newChapters: 1 })],
      total: 3,
      page: 1,
      size: 30,
    });
    renderShelf();

    const summary = await screen.findByTestId("shelf-updates");
    expect(summary.textContent).toContain("2 本书有更新");
    expect(summary.getAttribute("aria-pressed")).toBe("false");
    expect(cardIds()).toEqual(["1", "2", "3"]);

    fireEvent.click(summary);
    await waitFor(() => expect(cardIds()).toEqual(["1", "3"]));
    expect(screen.getByTestId("shelf-updates").getAttribute("aria-pressed")).toBe("true");
    // 只剩有更新的书，且每张都带角标
    expect(screen.getByTestId("shelf-live").querySelectorAll(".book-update-badge")).toHaveLength(2);

    fireEvent.click(screen.getByTestId("shelf-updates"));
    await waitFor(() => expect(cardIds()).toEqual(["1", "2", "3"]));
  });

  it("PC 上同样可用（汇总条与筛选都不分档）", async () => {
    books.mockResolvedValue({
      items: [bookAt(1, { newChapters: 1 }), bookAt(2)],
      total: 2,
      page: 1,
      size: 24,
    });
    renderShelf("pc");

    const summary = await screen.findByTestId("shelf-updates");
    expect(summary.textContent).toContain("1 本书有更新");
    fireEvent.click(summary);
    await waitFor(() => expect(cardIds()).toEqual(["1"]));
  });
});
