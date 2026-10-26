import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthorDetail } from "../../api/types";

const author = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { ...actual.api, author: (id: number, opts?: unknown) => author(id, opts) } };
});

const { AuthorWorksPage } = await import("./AuthorWorksPage");

function detail(over: Partial<AuthorDetail> = {}): AuthorDetail {
  return {
    id: 3,
    name: "林砚",
    bookCount: 2,
    wordCount: 5_000_000,
    books: [
      {
        id: 7,
        title: "长夜未明",
        authorId: 3,
        authorName: "林砚",
        chapterCount: 412,
        wordCount: 4_800_000,
        volumeCount: 3,
        intro: "",
        status: "parsed",
        importedAt: "2026-10-01T00:00:00Z",
        updatedAt: "2026-10-01T00:00:00Z",
        hasCover: false,
      },
    ],
    ...over,
  } as AuthorDetail;
}

function renderPage(id = 3) {
  return render(
    <MemoryRouter initialEntries={[`/authors/${id}`]}>
      <Routes>
        <Route path="/authors/:authorId" element={<AuthorWorksPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  author.mockReset();
});

describe("作者作品页（C-22 的落点）", () => {
  it("列出该作者的作品，每本都能进详情", async () => {
    author.mockResolvedValue(detail());
    renderPage();
    const card = await screen.findByTestId("author-book-7");
    expect((card as HTMLAnchorElement).getAttribute("href")).toBe("/book/7");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("林砚");
    expect(author).toHaveBeenCalledWith(3, expect.anything());
  });

  it("没有作品时给空态与回作者列表的出口", async () => {
    author.mockResolvedValue(detail({ books: [], bookCount: 0 }));
    renderPage();
    expect(await screen.findByText("这位作者还没有作品")).toBeTruthy();
    expect(screen.getAllByText("回到作者列表").length).toBeGreaterThan(0);
  });

  it("读取失败给可重试的错误态", async () => {
    author.mockRejectedValue(new Error("离线"));
    renderPage();
    expect(await screen.findByText("作品列表没能加载")).toBeTruthy();
  });
});
