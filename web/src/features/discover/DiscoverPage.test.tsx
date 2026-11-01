import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RecommendItem, RecommendResult, RecommendTab } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { ToastHost } from "../../lib/toast";
import { clearToast } from "../../lib/toastStore";

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

/** 取数桩的形状：与 useResource 的返回值同构，页面只读这四个字段。 */
interface Feed {
  data: RecommendResult | null;
  loading: boolean;
  error: unknown;
  reload: () => void;
}

const h = vi.hoisted(() => ({
  /** 每个 tab 的取数结果；没配的 tab 落到 fallback */
  byTab: {} as Record<string, Feed>,
  fallback: { data: null, loading: false, error: null, reload: vi.fn() } as Feed,
  limit: 0,
  tab: "",
}));

vi.mock("../../api/hooks", () => ({
  useRecommendations: (limit: number, tab: string) => {
    h.limit = limit;
    h.tab = tab;
    return h.byTab[tab] ?? h.fallback;
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

/** 取数桩：data 用 RecommendResult，断言只关心渲染结果。 */
function feed(items: RecommendItem[] | null, over: Partial<Feed> = {}): Feed {
  return {
    data: items === null ? null : result(items),
    loading: over.loading ?? false,
    error: over.error ?? null,
    reload: over.reload ?? vi.fn(),
  };
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
        <ToastHost />
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  track.mockReset();
  impression.mockReset();
  clearToast();
  h.limit = 0;
  h.tab = "";
  h.byTab = {};
  h.fallback = feed([item(1), item(2), item(3)]);
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

  it("请求条数用契约内的默认值（1–50），首个 tab 是 for_you", () => {
    renderPage();
    expect(RECOMMEND_LIMIT).toBeGreaterThanOrEqual(1);
    expect(RECOMMEND_LIMIT).toBeLessThanOrEqual(50);
    expect(h.limit).toBe(RECOMMEND_LIMIT);
    expect(h.tab).toBe("for_you");
  });

  it("每张卡挂一次曝光观察，payload 带上这条推荐理由（后端存进 extra）", () => {
    renderPage();
    expect(impression).toHaveBeenCalledTimes(3);
    expect(impression.mock.calls.map(([payload]) => (payload as { bookId: number }).bookId)).toEqual([1, 2, 3]);
    expect((impression.mock.calls[0][0] as { extra?: string }).extra).toBe("因为你常看武侠");
  });

  it("H5 与 PC 同一套 DOM：只在封面占位文案上分档", () => {
    renderPage("h5");
    expect(screen.getByTestId("discover-grid").children).toHaveLength(3);
  });
});

describe("发现页：三个来源的 tab", () => {
  it("三颗 tab 都在，默认选中「为你推荐」，点另一颗就会换 tab 取数", () => {
    h.byTab = { for_you: feed([item(1), item(2)]), resume: feed([item(7)]) };
    renderPage();

    const forYou = screen.getByTestId("discover-tab-for_you");
    const resume = screen.getByTestId("discover-tab-resume");
    const random = screen.getByTestId("discover-tab-random");
    expect([forYou.textContent, resume.textContent, random.textContent]).toEqual([
      "为你推荐",
      "继续阅读",
      "随机发现",
    ]);
    // 选中态走 aria-pressed（按钮是「换数据源」的开关组，不是分页 tab）
    expect(forYou.getAttribute("aria-pressed")).toBe("true");
    expect(resume.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(resume);
    expect(h.tab).toBe("resume");
    expect(screen.getByTestId("discover-tab-resume").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId("discover-grid").children).toHaveLength(1);
    expect(screen.getByTestId("rec-7")).toBeTruthy();

    fireEvent.click(random);
    expect(h.tab).toBe("random");
  });

  it("tab 的 id 就是请求参数（for_you / resume / random）", () => {
    renderPage();
    for (const tab of ["for_you", "resume", "random"] as RecommendTab[]) {
      fireEvent.click(screen.getByTestId(`discover-tab-${tab}`));
      expect(h.tab).toBe(tab);
    }
  });

  it("空态文案按 tab 区分：继续阅读空 = 还没有读到一半的书", () => {
    h.byTab = { resume: feed([]) };
    renderPage();
    fireEvent.click(screen.getByTestId("discover-tab-resume"));

    const empty = screen.getByTestId("discover-empty");
    expect(empty.textContent).toContain("还没有读到一半的书");
    expect(empty.textContent).not.toContain("还没有可以推荐的书");
  });

  it("随机发现的空态说明「书库还没有可以翻的书」", () => {
    h.byTab = { random: feed([]) };
    renderPage();
    fireEvent.click(screen.getByTestId("discover-tab-random"));
    expect(screen.getByTestId("discover-empty").textContent).toContain("书库还没有可以翻的书");
  });
});

describe("发现页：不感兴趣", () => {
  it("点「不感兴趣」：记一条 dislike（带理由）、把这张卡摘掉、弹提示，且不整页重取", () => {
    renderPage();
    expect(screen.getByTestId("discover-grid").children).toHaveLength(3);

    fireEvent.click(screen.getByTestId("dislike-2"));

    expect(track).toHaveBeenCalledWith("dislike", { bookId: 2, extra: "因为你常看武侠" });
    expect(screen.queryByTestId("rec-2")).toBeNull();
    expect(screen.getByTestId("discover-grid").children).toHaveLength(2);
    // 其他卡不受影响
    expect(screen.getByTestId("rec-1")).toBeTruthy();
    // 提示「不会再推荐这本」；没有触发任何重新取数
    expect(screen.getByTestId("toast").textContent).toContain("不会再推荐这本");
    expect(h.fallback.reload).not.toHaveBeenCalled();
  });

  it("按钮是卡片链接的兄弟节点：点它不会跳进详情页", () => {
    renderPage();
    const card = screen.getByTestId("rec-2");
    const button = screen.getByTestId("dislike-2");
    expect(button.closest("a")).toBeNull();
    expect(card.contains(button)).toBe(true);

    fireEvent.click(button);
    expect(screen.queryByTestId("book-stub")).toBeNull();
  });

  it("连点几本都只影响本地列表，直到换一批才清空已移除的集合", () => {
    const { rerender } = renderPage();
    fireEvent.click(screen.getByTestId("dislike-1"));
    fireEvent.click(screen.getByTestId("dislike-2"));
    // 还剩第 3 本：网格继续在，只是少了两张卡
    expect(screen.getByTestId("discover-grid").children).toHaveLength(1);
    fireEvent.click(screen.getByTestId("dislike-3"));
    expect(screen.queryByTestId("discover-grid")).toBeNull();
    expect(screen.getByTestId("discover-empty")).toBeTruthy();
    expect(h.fallback.reload).not.toHaveBeenCalled();

    // 换一批：新数据到手，已移除集合清空
    h.fallback = feed([item(1), item(2), item(3)]);
    rerender(
      <TierProvider value="pc">
        <MemoryRouter initialEntries={["/discover"]}>
          <Routes>
            <Route path="/discover" element={<DiscoverPage />} />
          </Routes>
          <ToastHost />
        </MemoryRouter>
      </TierProvider>,
    );
    expect(screen.getByTestId("discover-grid").children).toHaveLength(3);
  });

  it("「不感兴趣」按钮有无障碍名（读屏能说清是哪一本）", () => {
    renderPage();
    expect(screen.getByTestId("dislike-2").getAttribute("aria-label")).toBe("不再推荐《样书2》");
  });
});

describe("发现页：换一批", () => {
  it("点「换一批」重新取数，取回期间按钮禁用且文案改为进行中", () => {
    const { rerender } = renderPage();
    const button = screen.getByTestId("discover-refresh") as HTMLButtonElement;
    expect(button.textContent).toBe("换一批");

    fireEvent.click(button);
    expect(h.fallback.reload).toHaveBeenCalledTimes(1);

    // 重取未完成：列表保持原样（不闪骨架），按钮禁用
    rerender(
      <TierProvider value="pc">
        <MemoryRouter initialEntries={["/discover"]}>
          <Routes>
            <Route path="/discover" element={<DiscoverPage />} />
          </Routes>
          <ToastHost />
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

    h.fallback = feed([item(9)]);
    rerender(
      <TierProvider value="pc">
        <MemoryRouter initialEntries={["/discover"]}>
          <Routes>
            <Route path="/discover" element={<DiscoverPage />} />
          </Routes>
          <ToastHost />
        </MemoryRouter>
      </TierProvider>,
    );
    expect((screen.getByTestId("discover-refresh") as HTMLButtonElement).disabled).toBe(false);
    expect(screen.getByTestId("discover-grid").children).toHaveLength(1);
  });
});

describe("发现页：点击埋点与跳转", () => {
  it("点卡片先记 click（带 bookId / 分类 / 推荐理由）再进详情", () => {
    renderPage();
    const link = screen.getByTestId("rec-2").querySelector("a");
    expect(link).not.toBeNull();

    fireEvent.click(link as HTMLAnchorElement);
    expect(track).toHaveBeenCalledTimes(1);
    expect(track).toHaveBeenCalledWith("click", {
      bookId: 2,
      categoryL1: "武侠",
      categoryL2: "江湖",
      extra: "因为你常看武侠",
    });
    expect(screen.getByTestId("book-stub").textContent).toBe("/book/2");
  });
});

describe("发现页：三态", () => {
  it("加载中给状态面板，不先铺空网格", () => {
    h.fallback = feed(null, { loading: true });
    renderPage();
    expect(screen.getByTestId("discover-loading")).toBeTruthy();
    expect(screen.queryByTestId("discover-grid")).toBeNull();
    expect(screen.queryByTestId("discover-refresh")).toBeNull();
  });

  it("读不到推荐给错误态，重试按钮重新取数", () => {
    const reload = vi.fn();
    h.fallback = feed(null, { error: { kind: "network" }, reload });
    renderPage();
    expect(screen.getByTestId("discover-error")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(reload).toHaveBeenCalled();
  });

  it("库里没有可推荐的书给空态，并指向书架", () => {
    h.fallback = feed([]);
    renderPage();
    expect(screen.getByTestId("discover-empty")).toBeTruthy();
    expect(screen.getByRole("link", { name: "去书架" }).getAttribute("href")).toBe("/shelf");
  });
});
