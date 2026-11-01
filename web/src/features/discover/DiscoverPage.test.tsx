import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RecommendItem, RecommendResult } from "../../api/types";
import { TierProvider } from "../../lib/tier";

/**
 * 发现页：取数与埋点都换成桩。
 * 断言落在「渲染出来的卡」「跳转目标」与「track 收到的 payload」，不碰内部 state。
 */
const track = vi.hoisted(() => vi.fn());
const impression = vi.hoisted(() => vi.fn());
vi.mock("../../lib/telemetry", () => ({
  track,
  useImpression: (_ref: unknown, payload: unknown) => impression(payload),
}));

const h = vi.hoisted(() => ({
  rec: {
    data: null as RecommendResult | null,
    loading: false,
    error: null as { kind: string; status?: number } | null,
    reload: vi.fn(),
  },
  limit: 0,
}));

vi.mock("../../api/hooks", () => ({
  useRecommendations: (limit: number) => {
    h.limit = limit;
    return h.rec;
  },
}));

const { DiscoverPage, RECOMMEND_LIMIT } = await import("./DiscoverPage");

function item(bookId: number, overrides: Partial<RecommendItem> = {}): RecommendItem {
  return {
    bookId,
    title: `样书${bookId}`,
    authorName: `作者${bookId}`,
    categoryL1: "武侠",
    categoryL2: "江湖",
    hasCover: false,
    reason: "因为你常看武侠",
    score: 1.2,
    ...overrides,
  };
}

function result(items: RecommendItem[]): RecommendResult {
  return { items, generatedAt: "2026-11-01T10:00:00Z" };
}

/** 跳转落点：详情页用桩接住，断言 URL 而不是渲染整页详情。 */
function BookStub() {
  const location = useLocation();
  return <div data-testid="book-stub">{location.pathname}</div>;
}

function renderPage(tier: "pc" | "h5" = "pc") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={["/discover"]}>
        <Routes>
          <Route path="/discover" element={<DiscoverPage />} />
          <Route path="/book/:bookId" element={<BookStub />} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

function setRec(next: Partial<typeof h.rec>) {
  h.rec = { ...h.rec, ...next };
}

beforeEach(() => {
  track.mockReset();
  impression.mockReset();
  h.limit = 0;
  h.rec = { data: result([item(1), item(2), item(3)]), loading: false, error: null, reload: vi.fn() };
});

describe("发现页：卡片流", () => {
  it("每本一张卡：封面、书名、作者、分类、推荐理由都在", () => {
    renderPage();
    expect(screen.getByTestId("view:discover")).toBeTruthy();
    expect(screen.getByTestId("discover-grid").children).toHaveLength(3);

    const card = screen.getByTestId("rec-2");
    expect(card.textContent).toContain("样书2");
    expect(card.textContent).toContain("作者2");
    expect(card.textContent).toContain("武侠");
    expect(card.textContent).toContain("因为你常看武侠");
    // 封面走共享组件：没有封面图时用书名首字占位，不留空位
    expect(card.querySelector(".cover")?.textContent).toContain("样");
  });

  it("请求条数用契约内的默认值（1–50）", () => {
    renderPage();
    expect(RECOMMEND_LIMIT).toBeGreaterThanOrEqual(1);
    expect(RECOMMEND_LIMIT).toBeLessThanOrEqual(50);
    expect(h.limit).toBe(RECOMMEND_LIMIT);
  });

  it("每张卡都挂一次曝光观察（可见性与时长由 SDK 判定）", () => {
    renderPage();
    expect(impression).toHaveBeenCalledTimes(3);
    expect(impression.mock.calls.map(([payload]) => (payload as { bookId: number }).bookId)).toEqual([1, 2, 3]);
  });

  it("H5 与 PC 同一套 DOM：只在封面占位文案上分档", () => {
    renderPage("h5");
    expect(screen.getByTestId("discover-grid").children).toHaveLength(3);
  });
});

describe("发现页：换一批", () => {
  it("点「换一批」重新取数，取回期间按钮禁用且文案改为进行中", () => {
    const { rerender } = renderPage();
    const button = screen.getByTestId("discover-refresh") as HTMLButtonElement;
    expect(button.textContent).toBe("换一批");

    fireEvent.click(button);
    expect(h.rec.reload).toHaveBeenCalledTimes(1);

    // 重取未完成：列表保持原样（不闪骨架），按钮禁用
    rerender(
      <TierProvider value="pc">
        <MemoryRouter initialEntries={["/discover"]}>
          <Routes>
            <Route path="/discover" element={<DiscoverPage />} />
          </Routes>
        </MemoryRouter>
      </TierProvider>,
    );
    const busy = screen.getByTestId("discover-refresh") as HTMLButtonElement;
    expect(busy.disabled).toBe(true);
    expect(busy.textContent).toBe("换一批中…");
    expect(screen.getByTestId("discover-grid")).toBeTruthy();
  });

  it("新一批到位后按钮恢复可用", () => {
    const { rerender } = renderPage();
    fireEvent.click(screen.getByTestId("discover-refresh"));

    setRec({ data: result([item(9)]) });
    rerender(
      <TierProvider value="pc">
        <MemoryRouter initialEntries={["/discover"]}>
          <Routes>
            <Route path="/discover" element={<DiscoverPage />} />
          </Routes>
        </MemoryRouter>
      </TierProvider>,
    );
    expect((screen.getByTestId("discover-refresh") as HTMLButtonElement).disabled).toBe(false);
    expect(screen.getByTestId("discover-grid").children).toHaveLength(1);
  });
});

describe("发现页：点击埋点与跳转", () => {
  it("点卡片先记 click（带 bookId 与分类）再进详情", () => {
    renderPage();
    const link = screen.getByTestId("rec-2").querySelector("a");
    expect(link).not.toBeNull();

    fireEvent.click(link as HTMLAnchorElement);
    expect(track).toHaveBeenCalledTimes(1);
    expect(track).toHaveBeenCalledWith("click", {
      bookId: 2,
      categoryL1: "武侠",
      categoryL2: "江湖",
    });
    expect(screen.getByTestId("book-stub").textContent).toBe("/book/2");
  });
});

describe("发现页：三态", () => {
  it("加载中给状态面板，不先铺空网格", () => {
    setRec({ data: null, loading: true });
    renderPage();
    expect(screen.getByTestId("discover-loading")).toBeTruthy();
    expect(screen.queryByTestId("discover-grid")).toBeNull();
    expect(screen.queryByTestId("discover-refresh")).toBeNull();
  });

  it("读不到推荐给错误态，重试按钮重新取数", () => {
    setRec({ data: null, error: { kind: "network" } });
    renderPage();
    expect(screen.getByTestId("discover-error")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(h.rec.reload).toHaveBeenCalled();
  });

  it("库里没有可推荐的书给空态，并指向书架", () => {
    setRec({ data: result([]) });
    renderPage();
    expect(screen.getByTestId("discover-empty")).toBeTruthy();
    expect(screen.getByRole("link", { name: "去书架" }).getAttribute("href")).toBe("/shelf");
  });
});
