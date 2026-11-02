import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RecommendItem, RecommendResult } from "../../api/types";

/**
 * 接入点断言（需求 9：凡是展示书籍的地方都有推荐位，并且注明场景）。
 *
 * 组件的三态在自己的测试里覆盖；这里只钉「它真的被挂到了页面上、在什么位置」——
 * 推荐位被别处重排或整块删掉时，这里会红。
 */
const h = vi.hoisted(() => ({
  rec: { data: null as RecommendResult | null, loading: false, error: null as unknown, reload: vi.fn() },
}));

vi.mock("../../api/hooks", () => ({
  useRecommendations: () => h.rec,
  useBooks: () => ({
    data: { items: [{ id: 9, title: "样书九", authorName: "作者九", chapterCount: 12 }], total: 1, page: 1, size: 5 },
    loading: false,
    error: null,
    reload: () => {},
  }),
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: () => {} }),
  useReadingStats: () => ({ data: null, loading: false, error: null, reload: () => {} }),
  useSimilarRecommendations: () => h.rec,
}));

const { HomePage } = await import("../start/HomePage");
const { SimilarBooks } = await import("../book/SimilarBooks");

function item(bookId: number): RecommendItem {
  return {
    bookId,
    title: `样书${bookId}`,
    authorId: 100 + bookId,
    authorName: `作者${bookId}`,
    categoryL1: "武侠",
    categoryL2: "",
    hasCover: false,
    reason: "因为你常看武侠",
    reasonCode: "interest:武侠",
    score: 1,
  };
}

beforeEach(() => {
  h.rec = {
    data: { items: [item(1), item(2)], generatedAt: "2026-11-01T10:00:00Z" },
    loading: false,
    error: null,
    reload: vi.fn(),
  };
});

describe("推荐位接入点", () => {
  it("H5 首页：推荐位挂在「最近在读」之前，标题写场景、说明写来源", () => {
    render(
      <MemoryRouter initialEntries={["/home"]}>
        <HomePage />
      </MemoryRouter>,
    );
    const block = screen.getByTestId("scene-rec:home");
    expect(screen.getByTestId("scene-rec:home:note").textContent).toContain("推荐来源：");
    expect(block.textContent).toContain("今晚还可以看这些");

    // 位置：推荐位在「最近在读」标题之前（首屏决策区靠上，而不是压在页面底部）
    const headings = Array.from(document.querySelectorAll("h2")).map((el) => el.textContent ?? "");
    expect(headings.indexOf("今晚还可以看这些")).toBeGreaterThanOrEqual(0);
    expect(headings.indexOf("今晚还可以看这些")).toBeLessThan(headings.indexOf("最近在读"));
  });

  it("详情页相似书沿用同一份场景文案，并在界面上写明来源", () => {
    render(
      <MemoryRouter>
        <SimilarBooks bookId={7} />
      </MemoryRouter>,
    );
    expect(screen.getByTestId("book:similar-note").textContent).toContain("推荐来源：");
    expect(screen.getByText("你可能也喜欢")).toBeTruthy();
  });
});
