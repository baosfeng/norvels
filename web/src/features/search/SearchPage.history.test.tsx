import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SearchResult } from "../../api/types";
import { SEARCH_HISTORY_KEY } from "../../lib/searchHistory";
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

const hit: SearchResult = { q: "夜", books: { items: [book], total: 1, page: 1, size: 8 } };
const empty: SearchResult = { q: "旧城", books: { items: [], total: 0, page: 1, size: 8 } };

const useSearch = vi.fn();
const emptyList = { data: { items: [], total: 0 }, loading: false, error: null, reload: () => {} };
vi.mock("../../api/hooks", () => ({
  useSearch: (q: string, params: unknown) => useSearch(q, params),
  useCategories: () => emptyList,
  useTags: () => ({ data: { items: [] }, loading: false, error: null, reload: () => {} }),
  useAuthors: () => emptyList,
}));

const { SearchPage } = await import("./SearchPage");

function renderAt(q: string, tier: "pc" | "h5" = "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={[q ? `/search?q=${encodeURIComponent(q)}` : "/search"]}>
        <SearchPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

function seedHistory(items: string[]) {
  localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(items));
}

beforeEach(() => {
  useSearch.mockReset();
  useSearch.mockReturnValue({ data: null, loading: false, error: null, reload: () => {} });
  localStorage.clear();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("搜索页：历史 / 高亮 / 空态 / 骨架", () => {
  it("没有关键词且有历史：列出历史 chip，点一下就按这个词搜", async () => {
    seedHistory(["长夜", "旧城"]);
    renderAt("");
    const chips = await screen.findByTestId("search-history");
    expect(within(chips).getByRole("button", { name: "长夜" })).toBeTruthy();
    fireEvent.click(within(chips).getByRole("button", { name: "旧城" }));
    await waitFor(() =>
      expect(useSearch).toHaveBeenLastCalledWith("旧城", expect.objectContaining({ type: "book", page: 1, size: 8 })),
    );
  });

  it("清空要两步：先变「确认清空」，点确认才真的清掉", async () => {
    seedHistory(["长夜", "旧城"]);
    renderAt("");
    await screen.findByTestId("search-history");

    fireEvent.click(screen.getByRole("button", { name: "清空" }));
    // 第一步只进入确认态，历史还在
    expect(screen.queryByTestId("search-history")).toBeTruthy();
    expect(localStorage.getItem(SEARCH_HISTORY_KEY)).toContain("长夜");

    fireEvent.click(screen.getByRole("button", { name: "确认清空" }));
    await waitFor(() => expect(screen.queryByTestId("search-history")).toBeNull());
    expect(localStorage.getItem(SEARCH_HISTORY_KEY)).toBeNull();
  });

  it("清空的「取消」不改数据", async () => {
    seedHistory(["长夜"]);
    renderAt("");
    await screen.findByTestId("search-history");
    fireEvent.click(screen.getByRole("button", { name: "清空" }));
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.getByTestId("search-history")).toBeTruthy();
    expect(localStorage.getItem(SEARCH_HISTORY_KEY)).toContain("长夜");
  });

  it("提交搜索成功后写入历史", async () => {
    useSearch.mockReturnValue({ data: hit, loading: false, error: null, reload: () => {} });
    renderAt("");
    fireEvent.change(screen.getByLabelText("搜索关键词"), { target: { value: "青川" } });
    fireEvent.click(screen.getByRole("button", { name: "搜索" }));
    await waitFor(() => expect(localStorage.getItem(SEARCH_HISTORY_KEY)).toContain("青川"));
  });

  it("结果里书名与作者的命中词用 <mark> 标出，而不是整段变色", async () => {
    useSearch.mockReturnValue({ data: hit, loading: false, error: null, reload: () => {} });
    renderAt("长夜");
    const title = await screen.findByTestId("search-book-title");
    const marks = title.querySelectorAll("mark");
    expect(marks).toHaveLength(1);
    expect(marks[0].textContent).toBe("长夜");
    // 未命中的部分仍是纯文本，整段文本不变
    expect(title.textContent).toBe("长夜书简");
    expect(screen.getByTestId("search-book-author").textContent).toBe("青川");
  });

  it("书名里的尖括号按文本渲染，命中词不会变成真实标签", async () => {
    const htmlTitle = { ...book, title: "<b>夜行</b>记", authorName: "<i>青川</i>" };
    useSearch.mockReturnValue({
      data: { q: "夜行", books: { items: [htmlTitle], total: 1, page: 1, size: 8 } },
      loading: false,
      error: null,
      reload: () => {},
    });
    renderAt("夜行");
    const title = await screen.findByTestId("search-book-title");
    expect(title.querySelector("b")).toBeNull();
    expect(title.textContent).toBe("<b>夜行</b>记");
    expect(screen.getByTestId("search-book-author").querySelector("i")).toBeNull();
  });

  it("结果项带作者、章数、字数", async () => {
    useSearch.mockReturnValue({ data: hit, loading: false, error: null, reload: () => {} });
    renderAt("长夜");
    const item = await screen.findByTestId("search-book-1");
    expect(item.textContent).toContain("青川");
    expect(item.textContent).toContain("120 章");
    expect(item.textContent).toContain("38 万 字");
  });

  it("无结果：给搜索建议 + 回书架出口 + 历史 chip", async () => {
    useSearch.mockReturnValue({ data: empty, loading: false, error: null, reload: () => {} });
    seedHistory(["长夜"]);
    renderAt("旧城");

    await screen.findByTestId("search-empty");
    const tips = screen.getByRole("list", { name: "搜索建议" });
    expect(tips.textContent).toContain("更短的关键词");
    expect(screen.getByRole("link", { name: "回书架挑一本" }).getAttribute("href")).toBe("/shelf");
    const chips = screen.getByTestId("search-history");
    expect(within(chips).getByRole("button", { name: "长夜" })).toBeTruthy();
  });

  it("搜索中给骨架屏，不出现转圈", async () => {
    useSearch.mockReturnValue({ data: null, loading: true, error: null, reload: () => {} });
    renderAt("长夜");
    expect(await screen.findByTestId("search-skeleton")).toBeTruthy();
    expect(document.querySelector(".spinner")).toBeNull();
  });

  it("骨架屏对读屏隐藏，不出现在无障碍树里", async () => {
    useSearch.mockReturnValue({ data: null, loading: true, error: null, reload: () => {} });
    renderAt("长夜");
    const skeleton = await screen.findByTestId("search-skeleton");
    expect(skeleton.getAttribute("aria-hidden")).toBe("true");
  });
});
