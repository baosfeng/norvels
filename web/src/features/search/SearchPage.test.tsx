import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SearchResult } from "../../api/types";
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

const emptyResult: SearchResult = {
  q: "青川剑砚",
  books: { items: [], total: 0, page: 1, size: 20 },
};

const hitResult: SearchResult = {
  q: "旧城",
  books: { items: [book], total: 1, page: 1, size: 20 },
};

/** 检索范围收敛后，这些旧说法不应再出现在搜索页可见文案里（用转义构造，源码里不留字面量）。 */
const BANNED_COPY = new RegExp(["\\u5168\\u6587", "\\u6b63\\u6587\\u68c0\\u7d22", "\\u5b57\\u7ea7"].join("|"));

/** 同一条口径：章节标题不再是检索范围，页面上不该再出现这四个字。 */
const BANNED_SCOPE = new RegExp("\\u7ae0\\u8282\\u6807\\u9898");

const useSearch = vi.fn();
const useCategories = vi.fn();
const useTags = vi.fn();
const useAuthors = vi.fn();

vi.mock("../../api/hooks", () => ({
  // 搜索页空态接入了场景推荐位（SceneRecommendBlock），mock 必须把它用到的 hook 补齐
  useRecommendations: () => ({ data: null, loading: false, error: null, reload: () => {} }),
  useSearch: (q: string, params: unknown) => useSearch(q, params),
  useCategories: () => useCategories(),
  useTags: () => useTags(),
  useAuthors: () => useAuthors(),
}));

/** 页面请求参数：筛选项缺省时不下发（空串 / 零值即不过滤）。 */
function lastSearchParams(): Record<string, unknown> {
  const calls = useSearch.mock.calls;
  return calls[calls.length - 1][1] as Record<string, unknown>;
}

const { SearchPage } = await import("./SearchPage");

function renderAt(q: string, tier: "pc" | "h5" = "pc") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={[`/search?q=${encodeURIComponent(q)}`]}>
        <SearchPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

/** 章节 tab / 章节结果 / 分栏键盘导航整块删除：这些节点一个都不该再出现在 DOM 里。 */
function expectNoChapterScope() {
  expect(document.querySelectorAll('[data-tab="chapter"]')).toHaveLength(0);
  expect(document.querySelectorAll('[data-od-id="search-chapters-col"]')).toHaveLength(0);
  expect(document.querySelectorAll('[data-testid="search-hit"]')).toHaveLength(0);
  expect(screen.queryByRole("tablist")).toBeNull();
  expect(screen.queryByTestId("search-chapters-empty")).toBeNull();
}

describe("搜索页：只按书名与作者搜", () => {
  beforeEach(() => {
    useSearch.mockReset();
    useCategories.mockReset().mockReturnValue({ data: { items: [], total: 0 }, loading: false, error: null, reload: () => {} });
    useTags.mockReset().mockReturnValue({ data: { items: [] }, loading: false, error: null, reload: () => {} });
    useAuthors.mockReset().mockReturnValue({ data: { items: [], total: 0 }, loading: false, error: null, reload: () => {} });
  });

  it("请求恒传 type=book，不传 chapter ——「不搜目录」在请求上也成立", async () => {
    useSearch.mockReturnValue({ data: hitResult, loading: false, error: null, reload: () => {} });
    renderAt("旧城");
    await waitFor(() => expect(useSearch).toHaveBeenCalled());
    expect(useSearch.mock.calls[0][0]).toBe("旧城");
    // 请求参数是对象：type=book 固定，筛选默认不下发
    expect(lastSearchParams()).toMatchObject({ type: "book", page: 1, size: 8 });
    expect(JSON.stringify(useSearch.mock.calls.map((call) => call[1]))).not.toContain("chapter");
  });

  it("有命中：书名与作者结果保留，章节 tab / 章节结果整块不存在", async () => {
    useSearch.mockReturnValue({ data: hitResult, loading: false, error: null, reload: () => {} });
    renderAt("旧城");

    await waitFor(() => expect(screen.getByText("长夜书简")).toBeTruthy());
    expect(screen.getByText("青川")).toBeTruthy();
    expect(screen.getByText("书名和作者")).toBeTruthy();
    // 检索范围文案是用户语言，明确写「按书名、作者搜索」
    expect(screen.getByText("按书名、作者搜索")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(BANNED_COPY);
    expect(document.body.textContent).not.toMatch(BANNED_SCOPE);
    expectNoChapterScope();
    // 项目既有去术语化规则：可见文案不出现实现词
    expect(findForbiddenTerms(screen.getByTestId("view:search").textContent ?? "")).toEqual([]);
  });

  it("无结果：提示明确说出检索范围", async () => {
    useSearch.mockReturnValue({ data: emptyResult, loading: false, error: null, reload: () => {} });
    renderAt("青川剑砚");

    const empty = await screen.findByTestId("search-empty");
    expect(empty.textContent).toContain("没有匹配的书名或作者");
    expect(document.body.textContent).not.toMatch(BANNED_COPY);
    expect(document.body.textContent).not.toMatch(BANNED_SCOPE);
    expect(findForbiddenTerms(screen.getByTestId("view:search").textContent ?? "")).toEqual([]);
  });

  // 回归：0 本时页头曾同时渲染「找到 0 本书」与「没有匹配的书名或作者」，同一件事说两遍，
  // 还把空态标题挤到下一屏。计数摘要只在真的有结果时出现。
  it("0 本时不显示计数摘要（不把同一件事说两遍）", async () => {
    useSearch.mockReturnValue({ data: emptyResult, loading: false, error: null, reload: () => {} });
    renderAt("青川剑砚");

    await screen.findByTestId("search-empty");
    expect(document.querySelector(".search-summary")).toBeNull();
    expect(document.body.textContent).not.toContain("找到 0 本书");
  });

  it("有结果时计数摘要照常出现", async () => {
    useSearch.mockReturnValue({ data: hitResult, loading: false, error: null, reload: () => {} });
    renderAt("旧城");

    await waitFor(() => expect(document.querySelector(".search-summary")).not.toBeNull());
    expect(screen.getByText("找到 1 本书")).toBeTruthy();
  });

  it("H5 与 PC 同一份结果：没有 tab、没有分栏", async () => {
    useSearch.mockReturnValue({ data: hitResult, loading: false, error: null, reload: () => {} });
    renderAt("旧城", "h5");
    await waitFor(() => expect(screen.getByText("长夜书简")).toBeTruthy());
    expect(screen.getByText("按书名、作者搜索")).toBeTruthy();
    expectNoChapterScope();
  });

  it("空关键词的默认空态也写同一口径", async () => {
    useSearch.mockReturnValue({ data: null, loading: false, error: null, reload: () => {} });
    renderAt("");
    const section = screen.getByTestId("view:search");
    expect(section.textContent).toContain("按书名、作者搜索");
    expect(section.textContent).not.toMatch(BANNED_SCOPE);
    // 输入框占位也说明范围
    expect((screen.getByLabelText("搜索关键词") as HTMLInputElement).placeholder).toBe("输入书名或作者");
  });

  it("只有一页命中时分页条整块不出现", async () => {
    useSearch.mockReturnValue({ data: hitResult, loading: false, error: null, reload: () => {} });
    renderAt("旧城");
    await waitFor(() => expect(screen.getByText("长夜书简")).toBeTruthy());
    expect(screen.queryByTestId("search-pagination")).toBeNull();
  });

  it("提交新关键词重新取数（仍走书名 + 作者）", async () => {
    useSearch.mockReturnValue({ data: hitResult, loading: false, error: null, reload: () => {} });
    renderAt("旧城");
    const input = screen.getByLabelText("搜索关键词") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "青川" } });
    fireEvent.click(screen.getByRole("button", { name: "搜索" }));
    await waitFor(() =>
      expect(useSearch).toHaveBeenLastCalledWith("青川", expect.objectContaining({ type: "book" })),
    );
  });
});
