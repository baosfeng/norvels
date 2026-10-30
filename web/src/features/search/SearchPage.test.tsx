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

vi.mock("../../api/hooks", () => ({
  useSearch: (q: string, type: string) => useSearch(q, type),
}));

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
  });

  it("请求恒传 type=book，不传 chapter ——「不搜目录」在请求上也成立", async () => {
    useSearch.mockReturnValue({ data: hitResult, loading: false, error: null, reload: () => {} });
    renderAt("旧城");
    await waitFor(() => expect(useSearch).toHaveBeenCalled());
    expect(useSearch.mock.calls[0][0]).toBe("旧城");
    expect(useSearch.mock.calls[0][1]).toBe("book");
    expect(useSearch.mock.calls.map((call) => call[1])).not.toContain("chapter");
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
    await waitFor(() => expect(useSearch).toHaveBeenLastCalledWith("青川", "book"));
  });
});
