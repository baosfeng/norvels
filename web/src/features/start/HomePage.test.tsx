import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastHost } from "../../lib/toast";
import { clearToast } from "../../lib/toastStore";

/** 取数替身：同一份 hook 返回值按用例改 state，避免每个用例一份 mock 工厂。 */
const h = vi.hoisted(() => ({
  books: { data: null as null | { items: { id: number; title: string; authorName: string; chapterCount: number }[]; total: number; page: number; size: number }, loading: false, error: null as null | { kind: string; status?: number }, reload: () => {} },
  progress: { data: { items: [] as { bookId: number; bookTitle: string; chapterId: number; chapterTitle: string; percent: number }[] }, loading: false, error: null, reload: () => {} },
  stats: { data: null as null | { totalSeconds: number; todaySeconds: number; weekSeconds: number; streak: number; days: { date: string; seconds: number }[] }, loading: false, error: null, reload: () => {} },
}));

vi.mock("../../api/hooks", () => ({
  useBooks: () => h.books,
  useReadingProgress: () => h.progress,
  useReadingStats: () => h.stats,
  // 首页的推荐位（SceneRecommendBlock）与本次断言无关：桩成空态 → 整块不渲染。
  useRecommendations: () => ({ data: null, loading: false, error: null, reload: vi.fn() }),
}));

const { HomePage } = await import("./HomePage");

/** 每次用例前回到「空库 + 无阅读记录」：不共享上一个用例留下的行。 */
function resetState() {
  h.books = { data: { items: [], total: 0, page: 1, size: 5 }, loading: false, error: null, reload: () => {} };
  h.progress = { data: { items: [] }, loading: false, error: null, reload: () => {} };
  h.stats = { data: null, loading: false, error: null, reload: () => {} };
}

function renderHome() {
  return render(
    <MemoryRouter initialEntries={["/home"]}>
      <HomePage />
      <ToastHost />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  clearToast();
  resetState();
});

describe("H5 首页：错档落点提示（C-05 边界）", () => {
  it("从 PC 专属页被送回首页时，浮出「在窄屏不提供」提示", async () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: "/home", state: { from: "/dashboard" } }]}>
        <HomePage />
        <ToastHost />
      </MemoryRouter>,
    );
    await waitFor(() =>
      expect(screen.getByTestId("toast").textContent).toContain("「驾驶舱」在窄屏不提供，已回到首页。"),
    );
  });

  // 回归：系列与阅读历史已对 H5 放开，不再被 TierRoute 送回首页，也就不该有提示
  it("从已对窄屏放开的页面进来不弹提示", () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: "/home", state: { from: "/series" } }]}>
        <HomePage />
        <ToastHost />
      </MemoryRouter>,
    );
    expect(screen.queryByTestId("toast")).toBeNull();
  });

  it("普通进入首页不弹提示", () => {
    render(
      <MemoryRouter initialEntries={["/home"]}>
        <HomePage />
        <ToastHost />
      </MemoryRouter>,
    );
    expect(screen.queryByTestId("toast")).toBeNull();
  });
});

/**
 * 复现缺陷：首页两栏原来都取 books?sort=updated，390 走查时「最近在读」与「最近入库」
 * 前 3 本完全相同 —— 同一批书占两次屏。这里只看 bookId 交集，书名用中性占位。
 */
describe("H5 首页：最近入库排除在读过的书", () => {
  function seedOverlap() {
    h.progress = {
      data: {
        items: [
          { bookId: 1, bookTitle: "甲书", chapterId: 1, chapterTitle: "第1章 开篇", percent: 0.01 },
          { bookId: 12, bookTitle: "乙书", chapterId: 1040, chapterTitle: "第46章 灯下", percent: 0.9 },
          { bookId: 4, bookTitle: "丙书", chapterId: 211, chapterTitle: "第11章 长夜", percent: 0.2 },
        ],
      },
      loading: false,
      error: null,
      reload: () => {},
    };
    h.books = {
      data: {
        items: [
          { id: 12, title: "乙书", authorName: "作者A", chapterCount: 150 },
          { id: 4, title: "丙书", authorName: "作者B", chapterCount: 90 },
          { id: 7, title: "丁书", authorName: "作者C", chapterCount: 42 },
        ],
        total: 3,
        page: 1,
        size: 5,
      },
      loading: false,
      error: null,
      reload: () => {},
    };
  }

  it("在读过的书不再出现在最近入库里，两栏 bookId 无交集", () => {
    seedOverlap();
    renderHome();
    const reading = [...screen.getByTestId("home:reading").querySelectorAll("a")].map((a) => a.getAttribute("href"));
    const recent = [...screen.getByTestId("home:recent-books").querySelectorAll("a")].map((a) => a.getAttribute("href"));
    expect(recent).toEqual(["/book/7"]);
    expect(reading.filter((href) => recent.includes(href))).toEqual([]);
  });

  it("排除后一本不剩就整区隐藏，不留一个空标题", () => {
    h.progress = {
      data: { items: [{ bookId: 12, bookTitle: "乙书", chapterId: 10, chapterTitle: "第11章", percent: 0.5 }] },
      loading: false,
      error: null,
      reload: () => {},
    };
    h.books = {
      data: { items: [{ id: 12, title: "乙书", authorName: "作者A", chapterCount: 150 }], total: 1, page: 1, size: 5 },
      loading: false,
      error: null,
      reload: () => {},
    };
    renderHome();
    expect(screen.queryByTestId("home:recent-section")).toBeNull();
  });
});

describe("H5 首页：最近入库的四态", () => {
  it("加载态是行骨架，不是转圈面板", () => {
    h.books = { data: null, loading: true, error: null, reload: () => {} };
    renderHome();
    expect(screen.getByTestId("home:recent-skeleton")).toBeTruthy();
    expect(screen.queryByTestId("state:loading")).toBeNull();
  });

  it("错误态用通用 ErrorPanel，重试走同一条 reload", () => {
    const reload = vi.fn();
    h.books = { data: null, loading: false, error: { kind: "http", status: 503 }, reload };
    renderHome();
    const panel = screen.getByTestId("home:recent-error");
    // 503 不再一律说成「数据库不可用」：没有 code 时只给最小口径，不指向整库故障
    expect(panel.textContent).toContain("服务暂时不可用");
    expect(panel.textContent).not.toContain("数据库");
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe("H5 首页：今天已读多久", () => {
  it("读数达到 1 分钟时压在标题下一行给出反馈", () => {
    h.stats = {
      data: { totalSeconds: 3832, todaySeconds: 1312, weekSeconds: 3832, streak: 4, days: [] },
      loading: false,
      error: null,
      reload: () => {},
    };
    renderHome();
    expect(screen.getByTestId("home:today").textContent).toContain("今天已读 22 分钟");
    expect(screen.getByTestId("home:today").textContent).toContain("连续 4 天");
  });

  it("不足 1 分钟 / 没取到数时整行不出现", () => {
    h.stats = {
      data: { totalSeconds: 20, todaySeconds: 20, weekSeconds: 20, streak: 0, days: [] },
      loading: false,
      error: null,
      reload: () => {},
    };
    renderHome();
    expect(screen.queryByTestId("home:today")).toBeNull();
  });
});
