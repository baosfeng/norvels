import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RecommendItem, RecommendResult } from "../../api/types";

/** 详情页「你可能也喜欢」：取数与埋点都换成桩，只断言渲染结果与 track 收到的 payload。 */
const track = vi.hoisted(() => vi.fn());
vi.mock("../../lib/telemetry", () => ({ track, useImpression: () => undefined }));

const h = vi.hoisted(() => ({
  state: {
    data: null as RecommendResult | null,
    loading: false,
    error: null as unknown,
    reload: vi.fn(),
  },
  bookId: 0,
}));

vi.mock("../../api/hooks", () => ({
  useSimilarRecommendations: (bookId: number) => {
    h.bookId = bookId;
    return h.state;
  },
}));

const { SimilarBooks, SIMILAR_LIMIT } = await import("./SimilarBooks");

function item(bookId: number): RecommendItem {
  return {
    bookId,
    title: `样书${bookId}`,
    authorId: 100 + bookId,
    authorName: `作者${bookId}`,
    categoryL1: "武侠",
    categoryL2: "江湖",
    hasCover: false,
    reason: "读过这本书的人也在看",
    score: 1,
  };
}

const feed = (items: RecommendItem[] | null, over: Partial<typeof h.state> = {}) => ({
  data: items === null ? null : { items, generatedAt: "2026-11-01T10:00:00Z" },
  loading: over.loading ?? false,
  error: over.error ?? null,
  reload: over.reload ?? vi.fn(),
});

function renderBlock(bookId = 7) {
  return render(
    <MemoryRouter>
      <SimilarBooks bookId={bookId} />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  track.mockReset();
  h.bookId = 0;
  h.state = feed([item(1), item(2), item(3), item(4), item(5), item(6)]);
});

describe("详情页：你可能也喜欢", () => {
  it("用当前书的 id 取 6 条，横向一张一卡（封面 / 书名 / 作者 / 理由）", () => {
    renderBlock(7);
    expect(h.bookId).toBe(7);
    expect(SIMILAR_LIMIT).toBe(6);
    expect(screen.getByTestId("book:similar")).toBeTruthy();
    expect(screen.getByTestId("book:similar").querySelectorAll(".similar-card")).toHaveLength(6);
    const first = screen.getByTestId("similar-1");
    expect(first.textContent).toContain("样书1");
    expect(first.textContent).toContain("作者1");
    expect(first.textContent).toContain("读过这本书的人也在看");
  });

  it("点一张卡：记 click（bookId / 作者 / 分类 / 理由）并进详情", () => {
    renderBlock(7);
    const link = screen.getByTestId("similar-2").querySelector("a");
    expect(link?.getAttribute("href")).toBe("/book/2");

    fireEvent.click(link as HTMLAnchorElement);
    expect(track).toHaveBeenCalledWith("click", {
      bookId: 2,
      authorId: 102,
      categoryL1: "武侠",
      categoryL2: "江湖",
      extra: "读过这本书的人也在看",
    });
  });

  it("空时不渲染整个区块（不是渲染一个「暂无」空框）", () => {
    h.state = feed([]);
    const { container } = renderBlock(7);
    expect(container.querySelector('[data-testid="book:similar"]')).toBeNull();
    expect(container.textContent).not.toContain("你可能也喜欢");
  });

  it("加载中给骨架（形状跟真卡一致，回来时不跳）", () => {
    h.state = feed(null, { loading: true });
    renderBlock(7);
    expect(screen.getByTestId("book:similar-loading")).toBeTruthy();
    expect(screen.getByTestId("book:similar-skeleton")).toBeTruthy();
    expect(screen.queryByTestId("book:similar")).toBeNull();
  });

  it("失败给可重试的错误态（区块标题仍在，说明这里本该有什么）", () => {
    const reload = vi.fn();
    h.state = feed(null, { error: { kind: "network" }, reload });
    renderBlock(7);
    expect(screen.getByTestId("book:similar-error")).toBeTruthy();
    expect(screen.getByText("你可能也喜欢")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(reload).toHaveBeenCalled();
  });
});
