import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterBrief, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const chapters = [
  { id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 },
  { id: 11, idx: 1, title: "第二章", volumeId: 1, wordCount: 2100 },
  { id: 12, idx: 2, title: "第三章", volumeId: 1, wordCount: 2200 },
];

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 3,
  wordCount: 6300,
  volumeCount: 1,
  intro: "",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  volumes: [{ id: 1, idx: 0, title: "正文", chapters }],
};

/** 长目录用书：420 章，复刻真实库里的「几百章」形态（书名仍是虚构的）。 */
const longChapters: ChapterBrief[] = Array.from({ length: 420 }, (_, i) => ({
  id: 1000 + i,
  idx: i,
  title: `第${i + 1}章 示例标题${i + 1}`,
  volumeId: 1,
  wordCount: 1000 + i,
}));

const longBook: BookDetail = {
  ...book,
  chapterCount: longChapters.length,
  volumes: [{ id: 1, idx: 0, title: "正文", chapters: longChapters }],
};

/** 当前生效的书：hooks 桩里读它，用例可以临时换成 longBook。 */
let activeBook: BookDetail = book;

function chapterOf(id: number): ChapterDetail {
  const brief = activeBook.volumes.flatMap((v) => v.chapters).find((c) => c.id === id) ?? chapters[0];
  return {
    bookId: 7,
    bookTitle: activeBook.title,
    id: brief.id,
    idx: brief.idx,
    title: brief.title,
    content: "雨从傍晚开始下。\n\n他站在巷口，没有打伞。",
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

/**
 * jsdom 里没有真实滚动容器高度，真实虚拟化会一行都不渲染。
 * 目录用例既要数「筛选后剩几行」，又要确认「定位到当前章」，所以这里换成
 * 全量渲染 + 可观测 scrollToIndex 的桩（与 ChapterList.test.tsx 同一口径）。
 */
const scrollToIndexSpy = vi.hoisted(() => vi.fn());

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (options: { count: number; estimateSize: () => number }) => {
    const items = Array.from({ length: options.count }, (_, index) => ({
      index,
      key: index,
      start: index * options.estimateSize(),
      size: options.estimateSize(),
    }));
    return {
      getTotalSize: () => options.count * options.estimateSize(),
      getVirtualItems: () => items,
      scrollToIndex: scrollToIndexSpy,
    };
  },
}));

const chapterState = vi.fn((_bookId: number, chapterId: number) => ({
  data: Number.isFinite(chapterId) ? chapterOf(chapterId) : null,
  loading: false,
  error: null,
  reload: vi.fn(),
}));


// 阅读器会用 /reading/progress 恢复章内位置：这些用例不关心真实请求，统一给空列表，
// 避免 jsdom 里发真网络请求把测试拖慢（也避免 act 警告）。
vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    // 这些用例只关心排版 / 键盘 / 滚动：进度请求挂起不回流，避免异步 setState 出 act 警告
    api: { ...actual.api, readingProgress: () => new Promise(() => {}) },
  };
});

vi.mock("../../api/hooks", () => ({
  useBook: () => ({ data: activeBook, loading: false, error: null, reload: vi.fn() }),
  useChapter: (bookId: number, chapterId: number) => chapterState(bookId, chapterId),
  useSaveProgress: () => vi.fn(async () => true),
  useRecordReadingSession: () => vi.fn(async () => true),
  useBookmarks: () => ({ data: { bookmarks: [] }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({ theme: "day", setTheme: vi.fn(), themes: ["day", "night", "parchment"] }),
    prefersReducedMotion: () => false,
  };
});

const { showToast } = vi.hoisted(() => ({ showToast: vi.fn() }));
vi.mock("../../lib/toastStore", () => ({ showToast }));

const { ReaderPage } = await import("./ReaderPage");

/** 书架探针：把收到的 location.state 打出来，验证「书架」出口带了 restore 标记。 */
function ShelfProbe() {
  const location = useLocation();
  const state = location.state as { restore?: boolean } | null;
  return <p>书架页 {state?.restore ? "restore" : "none"}</p>;
}

function renderReader(
  tier: "h5" | "pc" = "h5",
  entries: string[] = ["/read/7/10"],
  initialIndex?: number,
) {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={entries} initialIndex={initialIndex}>
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情</p>} />
          <Route path="/shelf" element={<ShelfProbe />} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

function surface() {
  return screen.getByTestId("reader-swipe-area");
}

function tap(target: HTMLElement = surface()) {
  fireEvent.pointerDown(target, { clientX: 120, clientY: 260 });
  fireEvent.pointerUp(target, { clientX: 120, clientY: 260 });
}

function headerHidden(): string | null {
  return screen.getByTestId("reader-top").getAttribute("aria-hidden");
}

function barHidden(): string | null {
  return screen.getByTestId("reader-bar").getAttribute("aria-hidden");
}

describe("阅读器：H5 控制层单击唤出与自动收起", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    chapterState.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("H5 默认完全收起：header 与底栏都不占位、不可见", () => {
    renderReader();

    expect(headerHidden()).toBe("true");
    expect(barHidden()).toBe("true");
    expect(screen.getByTestId("view:reader").className).not.toContain("is-controls");
    // 正文始终在，收起态就能直接读
    expect(screen.getByText("雨从傍晚开始下。")).toBeTruthy();
  });

  it("单击正文唤出控制层，3 秒无操作自动收起", () => {
    renderReader();

    act(() => tap());
    expect(headerHidden()).toBe("false");
    expect(barHidden()).toBe("false");
    expect(screen.getByTestId("view:reader").className).toContain("is-controls");

    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(headerHidden()).toBe("true");
    expect(barHidden()).toBe("true");
  });

  it("唤出后再单击一次立即收起，不等 3 秒", () => {
    renderReader();

    act(() => tap());
    expect(headerHidden()).toBe("false");
    act(() => tap());
    expect(headerHidden()).toBe("true");
  });

  it("滑动不算单击：位移超过阈值不切换控制层", () => {
    renderReader();

    fireEvent.pointerDown(surface(), { clientX: 200, clientY: 300 });
    fireEvent.pointerUp(surface(), { clientX: 100, clientY: 305 });
    expect(headerHidden()).toBe("true");
  });

  it("设置面板打开时暂停自动收起", () => {
    renderReader();

    act(() => tap());
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "阅读设置" }));
    });
    expect(screen.getByRole("button", { name: "阅读设置" }).getAttribute("aria-expanded")).toBe("true");

    act(() => {
      vi.advanceTimersByTime(8000);
    });
    expect(headerHidden()).toBe("false");
  });

  it("C-12：翻章时浮条提示「已翻到第 n 章」", () => {
    showToast.mockClear();
    renderReader();
    act(() => {
      fireEvent.click(screen.getByLabelText("下一章"));
    });
    expect(showToast).toHaveBeenCalledWith("已翻到第 2 章");
  });

  it("翻章保持当前显隐状态，进度数字跟着更新", () => {
    renderReader();

    act(() => tap());
    expect(screen.getByTestId("reader-top").textContent).toContain("第 1 / 3 章");

    act(() => {
      fireEvent.click(screen.getByLabelText("下一章"));
    });

    expect(chapterState).toHaveBeenLastCalledWith(7, 11);
    expect(headerHidden()).toBe("false");
    expect(screen.getByTestId("reader-top").textContent).toContain("第 2 / 3 章");
    // 收起态下进度数字不常驻：收起后一起隐藏
    act(() => tap());
    expect(headerHidden()).toBe("true");
  });

  it("控制层并排显示当前章与字数，翻章后一起更新", () => {
    renderReader();

    act(() => tap());
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 / 3 章 · 2,000 字");

    act(() => {
      fireEvent.click(screen.getByLabelText("下一章"));
    });
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 2 / 3 章 · 2,100 字");
    // 正文底部的进度行同样带字数，口径一致
    expect(screen.getByTestId("reader-word").textContent).toContain("2,100 字");
  });

  it("浏览器没有全屏 API 时按钮照常可点：收起控制层并提示（契约 C-02）", () => {
    renderReader();
    act(() => tap());
    const button = screen.getByLabelText("全屏阅读");
    act(() => {
      fireEvent.click(button);
    });
    expect(screen.getByTestId("view:reader").className).not.toContain("is-controls");
  });

  it("支持全屏时才给按钮（能力探测）", () => {
    Object.defineProperty(document, "fullscreenEnabled", { value: true, configurable: true });
    try {
      renderReader();
      act(() => tap());
      expect(screen.getByLabelText("全屏阅读")).toBeTruthy();
    } finally {
      Object.defineProperty(document, "fullscreenEnabled", { value: undefined, configurable: true });
    }
  });

  it("PC 常驻 header，不渲染 H5 底栏", () => {
    renderReader("pc");

    expect(headerHidden()).toBe("false");
    expect(screen.queryByTestId("reader-bar")).toBeNull();
    expect(screen.getByTestId("view:reader").className).toContain("is-controls");
  });
});

describe("阅读器：书架出口与返回语义", () => {
  beforeEach(() => {
    chapterState.mockClear();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("控制层「书架」出口直达 /shelf 并带 restore 标记", () => {
    renderReader("pc");

    fireEvent.click(screen.getByLabelText("返回书架"));

    expect(screen.getByText("书架页 restore")).toBeTruthy();
  });

  it("返回优先走浏览器历史：从书架进来的阅读器退回书架原位置", () => {
    renderReader("pc", ["/shelf", "/read/7/10"], 1);

    fireEvent.click(screen.getByLabelText("返回目录"));

    // 回退到历史里的 /shelf（不是 push 一条新的），因此不该带 restore 标记
    expect(screen.getByText("书架页 none")).toBeTruthy();
  });

  it("没有历史时返回回落到 /shelf 并带 restore", () => {
    renderReader("pc", ["/read/7/10"]);

    fireEvent.click(screen.getByLabelText("返回目录"));

    expect(screen.getByText("书架页 restore")).toBeTruthy();
  });
});

describe("阅读器：H5 控制层是覆盖层（不占布局）", () => {
  async function readStyles(relative: string): Promise<string> {
    const fs = (await import("node:fs" as string)) as { readFileSync: (file: URL, encoding: string) => string };
    return fs.readFileSync(new URL(relative, import.meta.url), "utf8");
  }

  it("H5 外壳定高 100dvh、header 绝对定位，收起态正文占满", async () => {
    const css = await readStyles("../../styles/globals.css");
    const start = css.indexOf("@media (max-width: 768px)");
    expect(start).toBeGreaterThan(-1);
    const block = css.slice(start);
    expect(block).toContain("height: 100dvh");
    expect(block).toContain("position: absolute");
    expect(block).toContain("overscroll-behavior: none");
    expect(block).toContain("env(safe-area-inset-bottom)");
    // 收起态没有任何 sticky 占位
    expect(block).not.toContain("position: sticky");
  });
});

describe("阅读器：目录抽屉（铺满 / 搜索 / 定位）", () => {
  beforeEach(() => {
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    chapterState.mockClear();
    scrollToIndexSpy.mockClear();
    activeBook = longBook;
  });

  afterEach(() => {
    activeBook = book;
    vi.restoreAllMocks();
  });

  /** 打开目录抽屉（H5 在底栏、PC 在 header，同一个 aria 语义按文案点） */
  function openDrawer() {
    fireEvent.click(screen.getByRole("button", { name: "目录" }));
  }

  function rows() {
    return Array.from(document.querySelectorAll(".cv-row"));
  }

  // 当前章 id=1200 是 420 章里的第 201 章（下标 200）
  const CURRENT = "/read/7/1200";

  it("目录按 60 章分块：首块 60 行，滚动交给抽屉父级（契约 C-29）", () => {
    renderReader("pc", ["/read/7/1000"]);
    openDrawer();

    expect(rows()).toHaveLength(60);
    const scroller = document.querySelector(".drawer-scroll");
    expect(scroller).toBeTruthy();
  });

  it("按章号搜索只剩命中行，清空后恢复全部 420 章", async () => {
    renderReader("pc", [CURRENT]);
    openDrawer();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 60));
    });
    expect(rows()).toHaveLength(201);

    fireEvent.change(screen.getByTestId("toc-search-input"), { target: { value: "419" } });
    expect(rows()).toHaveLength(1);
    expect(rows()[0].textContent).toContain("第419章");
    expect(screen.getByTestId("toc-match").textContent).toContain("匹配 1 章");

    fireEvent.change(screen.getByTestId("toc-search-input"), { target: { value: "" } });
    expect(rows()).toHaveLength(201);
    expect(screen.queryByTestId("toc-match")).toBeNull();
  });

  it("搜索不到时给出空提示，不渲染空列表", () => {
    renderReader("pc", [CURRENT]);
    openDrawer();

    fireEvent.change(screen.getByTestId("toc-search-input"), { target: { value: "不存在的标题" } });

    expect(rows()).toHaveLength(0);
    expect(screen.getByTestId("toc-empty")).toBeTruthy();
    expect(screen.getByTestId("toc-match").textContent).toContain("匹配 0 章");
  });

  it("打开抽屉即定位到当前章：长目录不从第 1 章开始", async () => {
    const scrollIntoView = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", { value: scrollIntoView, configurable: true });
    renderReader("pc", [CURRENT]);
    openDrawer();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 120));
    });

    // 当前章（第 201 章）所在的块被补显出来，并滚进视口
    expect(document.querySelector('[data-current-chapter="true"]')).toBeTruthy();
    expect(rows().length).toBeGreaterThan(200);
  });

  it("点「跳到当前章」把当前章重新滚进视口中间", async () => {
    const scrollIntoView = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", { value: scrollIntoView, configurable: true });
    renderReader("pc", [CURRENT]);
    openDrawer();
    fireEvent.change(screen.getByTestId("toc-search-input"), { target: { value: "419" } });
    expect(rows()).toHaveLength(1);
    scrollIntoView.mockClear();

    fireEvent.click(screen.getByTestId("toc-jump"));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 30));
    });

    expect(document.querySelector('[data-current-chapter="true"]')).toBeTruthy();
    expect(scrollIntoView).toHaveBeenCalled();
    // 跳转同时清掉搜索，回到完整目录（已补显到当前章）
    expect(screen.queryByTestId("toc-match")).toBeNull();
  });
});