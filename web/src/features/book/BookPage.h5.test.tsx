import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, BookMetaPatch, ReadingProgress } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 2,
  wordCount: 34000,
  volumeCount: 1,
  intro: "一段示例简介。",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  hasCover: false,
  series: "示例系列",
  seriesIndex: 2,
  tags: ["武侠"],
  serialStatus: "ongoing",
  readStatus: "reading",
  categoryL1: "武侠",
  categoryL2: "群像",
  publisher: "示例出版社",
  pubDate: "2024-01-01",
  bookmarkCount: 3,
  volumes: [
    {
      id: 1,
      idx: 0,
      title: "正文",
      chapters: [{ id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 }],
    },
  ],
};

const progressState: { items: ReadingProgress[] } = { items: [] };
const bookState = { data: book as BookDetail | null, loading: false, error: null, reload: vi.fn() };
const updateBook = vi.fn<(id: number, patch: BookMetaPatch) => Promise<BookDetail>>();

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (options: { count: number; estimateSize: () => number }) => {
    const items = Array.from({ length: options.count }, (_, index) => ({
      index,
      key: index,
      start: index * options.estimateSize(),
      size: options.estimateSize(),
    }));
    return {
      getTotalSize: () => options.count * options.estimateSize(),
      getVirtualItems: () => items,
      scrollToIndex: vi.fn(),
    };
  },
}));

vi.mock("../../api/hooks", () => ({
  useBook: () => bookState,
  useReadingProgress: () => ({ data: { items: progressState.items }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook } };
});

const { BookPage } = await import("./BookPage");

function renderPage(tier: "pc" | "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={["/book/7"]}>
        <Routes>
          <Route path="/book/:bookId" element={<BookPage />} />
          <Route path="/shelf" element={<p>书架页</p>} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  bookState.data = book;
  bookState.reload = vi.fn();
  updateBook.mockReset().mockResolvedValue({} as BookDetail);
});

describe("详情页事实区：H5 不再被裁掉", () => {
  it("H5 渲染完整书籍信息（字数/分类/系列/阅读状态/评分/书签/出版）", () => {
    renderPage("h5");

    const facts = screen.getByTestId("book-facts");
    expect(facts).toBeTruthy();
    const text = facts.textContent ?? "";
    expect(text).toContain("3.4 万"); // 字数
    expect(text).toContain("武侠"); // 分类 l1
    expect(text).toContain("群像"); // 分类 l2
    expect(text).toContain("示例系列"); // 系列
    expect(text).toContain("3 条"); // 书签数
    expect(text).toContain("示例出版社"); // 出版
    // 阅读状态下拉与评分组都在事实区里
    expect(screen.getByTestId("book-read-status")).toBeTruthy();
    // 评分组在事实区内（外层行 + 星级按钮组共用同一 testId，取第一个即可）
    expect(facts.querySelector('[data-testid="book-rating"]')).not.toBeNull();
  });

  it("H5 事实区落在主栏（书名与简介之后），左栏不再承载它", () => {
    renderPage("h5");
    const facts = screen.getByTestId("book-facts");
    // 主栏容器包含事实区；左栏（book-aside）里没有
    expect(facts.closest(".book-main")).not.toBeNull();
    expect(document.querySelector(".book-aside")?.querySelector('[data-testid="book-facts"]')).toBeNull();
  });

  it("PC 事实区仍在左栏，且只有一份（不重复渲染）", () => {
    renderPage("pc");
    const all = screen.getAllByTestId("book-facts");
    expect(all).toHaveLength(1);
    expect(all[0].closest(".book-aside")).not.toBeNull();
  });

  it("H5 改阅读状态照常保存并即时回显", async () => {
    renderPage("h5");
    fireEvent.change(screen.getByTestId("book-read-status"), { target: { value: "done" } });
    await waitFor(() => expect(updateBook).toHaveBeenCalledWith(7, { readStatus: "done" }));
    expect((screen.getByTestId("book-read-status") as HTMLSelectElement).value).toBe("done");
  });

  it("H5 封面 / 开读 / 加入书单 / 加入书架四个入口都在", () => {
    renderPage("h5");
    expect(screen.getByTestId("book-add-collection")).toBeTruthy();
    // H5 的「开读」与「加入书架」都在底部吸底条（H5StickyCta）：主操作常驻拇指区。
    // 顶部不再重复渲染同一组按钮，否则同屏会出现两个主操作。
    expect(screen.getByTestId("book-h5-cta")).toBeTruthy();
    expect(screen.getByTestId("book-h5-read")).toBeTruthy();
    expect(screen.getByTestId("book-h5-shelf")).toBeTruthy();
    expect(screen.queryByText("从第一章开始")).toBeNull();
  });

  it("H5 署名作者不跳 PC 专属的作者页（否则会被 TierRoute 弹回首页）", () => {
    renderPage("h5");
    const link = screen.getByTestId("book-byline").querySelector("a") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/shelf?author=1");
    expect(link.getAttribute("href")).not.toContain("/authors");
  });

  it("PC 署名作者仍去作者页", () => {
    renderPage("pc");
    const link = screen.getByTestId("book-byline").querySelector("a") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/authors");
  });
});
