import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SearchHit, SearchResult } from "../../api/types";
import { findForbiddenTerms } from "../../lib/audit";
import { TierProvider } from "../../lib/tier";

const book = {
  id: 1,
  title: "长夜书简",
  authorId: 9,
  authorName: "青川",
  chapterCount: 120,
  wordCount: 380000,
  volumeCount: 3,
  intro: "",
  status: "parsed" as const,
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-24T20:24:50.859Z",
};

const hit: SearchHit = {
  bookId: 1,
  bookTitle: "长夜书简",
  chapterId: 12,
  chapterIdx: 11,
  chapterTitle: "第十二章 旧城的雨",
  // 后端契约：字段保留但恒为空串
  snippet: "",
  score: 1,
};

const emptyResult: SearchResult = {
  q: "青川剑砚",
  books: { items: [], total: 0, page: 1, size: 20 },
  chapters: { items: [], total: 0, page: 1, size: 20 },
};

const hitResult: SearchResult = {
  q: "旧城",
  books: { items: [book], total: 1, page: 1, size: 20 },
  chapters: { items: [hit], total: 1, page: 1, size: 20 },
};

/** 检索范围收敛后，这些旧说法不应再出现在搜索页可见文案里（源码里用转义构造，不留字面量）。 */
const BANNED_COPY = new RegExp(["\\u5168\\u6587", "\\u6b63\\u6587\\u68c0\\u7d22", "\\u5b57\\u7ea7"].join("|"));

const useSearch = vi.fn();

vi.mock("../../api/hooks", () => ({
  useSearch: (q: string) => useSearch(q),
}));

const { SearchPage } = await import("./SearchPage");

function renderAt(q: string) {
  return render(
    <TierProvider value="pc">
      <MemoryRouter initialEntries={[`/search?q=${encodeURIComponent(q)}`]}>
        <SearchPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

describe("搜索页：检索范围只剩书名 / 作者 / 章节标题", () => {
  beforeEach(() => {
    useSearch.mockReset();
  });

  it("有命中：章节命中不出空片段行，书名与作者结果照常显示", async () => {
    useSearch.mockReturnValue({ data: hitResult, loading: false, error: null, reload: () => {} });
    const { container } = renderAt("旧城");

    await waitFor(() => expect(screen.getByText("第十二章 旧城的雨")).toBeTruthy());
    // 命中项在，但片段段落整行不渲染
    expect(container.querySelectorAll('[data-testid="search-hit"]')).toHaveLength(1);
    expect(container.querySelectorAll(".hit-snippet")).toHaveLength(0);
    // 命中项只剩「书名 · 章节标题」一行：没有片段就没有第二个子节点，也就没有空的高亮块
    const hitNode = container.querySelector("article.hit") as HTMLElement;
    expect(hitNode.children).toHaveLength(1);
    expect(hitNode.innerHTML).not.toContain("<b>");
    // 检索范围文案是用户语言
    expect(screen.getByText("书名 · 作者 · 章节标题")).toBeTruthy();
    expect(screen.getByText("书名和作者")).toBeTruthy();
    expect(screen.getByText("章节标题")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(BANNED_COPY);
    // 项目既有去术语化规则：可见文案不出现实现词
    expect(findForbiddenTerms(screen.getByTestId("view:search").textContent ?? "")).toEqual([]);
  });

  it("无结果：提示明确说出检索范围", async () => {
    useSearch.mockReturnValue({ data: emptyResult, loading: false, error: null, reload: () => {} });
    renderAt("青川剑砚");

    const empty = await screen.findByTestId("search-empty");
    expect(empty.textContent).toContain("没有匹配的书名、作者或章节标题");
    expect(document.body.textContent).not.toMatch(BANNED_COPY);
    expect(findForbiddenTerms(screen.getByTestId("view:search").textContent ?? "")).toEqual([]);
  });

  it("切到只有另一档有命中的那一档时，说清这一档查的是什么，不留空白", async () => {
    // 只有章节命中：切到「书名和作者」应当给一句说明
    useSearch.mockReturnValue({
      data: { ...hitResult, books: { items: [], total: 0, page: 1, size: 20 } },
      loading: false,
      error: null,
      reload: () => {},
    });
    const { unmount } = renderAt("旧城");
    await waitFor(() => expect(screen.getByText("章节标题")).toBeTruthy());
    fireEvent.click(screen.getByRole("tab", { name: /书名和作者/ }));
    const hint = await screen.findByTestId("search-books-empty");
    expect(hint.textContent).toContain("没有匹配的书名或作者");
    unmount();

    // 只有书名命中：切到「章节标题」应当给一句说明
    useSearch.mockReturnValue({
      data: { ...hitResult, chapters: { items: [], total: 0, page: 1, size: 20 } },
      loading: false,
      error: null,
      reload: () => {},
    });
    renderAt("旧城");
    await waitFor(() => expect(screen.getByText("书名和作者")).toBeTruthy());
    fireEvent.click(screen.getByRole("tab", { name: /章节标题/ }));
    const hint2 = await screen.findByTestId("search-chapters-empty");
    expect(hint2.textContent).toContain("没有匹配的章节标题");
  });
});
