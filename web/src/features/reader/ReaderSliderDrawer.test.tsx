import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, Bookmark, ChapterBrief, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const chapters: ChapterBrief[] = [
  { id: 10, idx: 0, title: "第一章 起风", volumeId: 1, wordCount: 2000 },
  { id: 11, idx: 1, title: "第二章 落雨", volumeId: 1, wordCount: 2100 },
  { id: 12, idx: 2, title: "第三章 归途", volumeId: 1, wordCount: 2200 },
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

function chapterOf(id: number): ChapterDetail {
  const brief = chapters.find((c) => c.id === id) ?? chapters[0];
  return {
    bookId: 7,
    bookTitle: book.title,
    id: brief.id,
    idx: brief.idx,
    title: brief.title,
    // 下一章用另一段正文：连续流同屏会渲染两章，相同文本会让断言撞车
    content:
      brief.idx === 0
        ? "雨从傍晚开始下。\n\n他站在巷口，没有打伞。"
        : `第${brief.idx + 1}章的正文。\n\n灯在巷尾亮着。`,
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

let store: Bookmark[] = [];

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      readingProgress: () => new Promise(() => {}),
      bookmarks: async () => ({ bookmarks: [...store] }),
    },
  };
});

vi.mock("../../api/hooks", () => ({
  useBook: () => ({ data: book, loading: false, error: null, reload: vi.fn() }),
  useChapter: (_bookId: number, chapterId: number) => ({
    data: chapterOf(chapterId),
    loading: false,
    error: null,
    reload: vi.fn(),
  }),
  useSaveProgress: () => vi.fn(async () => true),
  useRecordReadingSession: () => vi.fn(async () => true),
  useBookmarks: () => ({ data: { bookmarks: [...store] }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({
      theme: "day",
      resolvedTheme: "day",
      setTheme: vi.fn(),
      themes: ["day", "night", "parchment", "green", "black", "system"],
    }),
    prefersReducedMotion: () => false,
  };
});

const { showToast } = vi.hoisted(() => ({ showToast: vi.fn() }));
vi.mock("../../lib/toastStore", () => ({ showToast }));

const { ReaderPage } = await import("./ReaderPage");

function ShelfProbe() {
  const location = useLocation();
  return <p>书架页 {(location.state as { restore?: boolean } | null)?.restore ? "restore" : "none"}</p>;
}

function renderReader(entries: string[] = ["/read/7/10"], tier: "h5" | "pc" = "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={entries}>
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情</p>} />
          <Route path="/shelf" element={<ShelfProbe />} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

function slider(): HTMLInputElement {
  return screen.getByTestId("chapter-slider") as HTMLInputElement;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("进度滑块：拖动跳章", () => {
  beforeEach(() => {
    store = [];
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  it("滑块按章节下标取值：3 章时范围 0–2，当前章即当前值", () => {
    renderReader(["/read/7/11"]);
    const input = slider();
    expect(input.type).toBe("range");
    expect(input.min).toBe("0");
    expect(input.max).toBe("2");
    expect(input.value).toBe("1");
    expect(input.getAttribute("aria-valuetext")).toBe("第 2 章，共 3 章");
  });

  it("拖动时浮出「第 N 章 章节名」气泡，松手才跳章", () => {
    renderReader(["/read/7/10"]);
    const input = slider();

    // 真实浏览器里 pointerdown 与后续的 input/change 是两次独立事件，中间有一次重渲染
    act(() => {
      fireEvent.pointerDown(input, { clientX: 10, clientY: 800 });
    });
    act(() => {
      fireEvent.change(input, { target: { value: "2" } });
    });

    const bubble = screen.getByTestId("chapter-slider-bubble");
    expect(bubble.textContent).toContain("第 3 章");
    expect(bubble.textContent).toContain("第三章 归途");
    // 还没松手：仍在第 1 章
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 3 章");

    act(() => {
      fireEvent.pointerUp(input, { clientX: 10, clientY: 800 });
    });
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 3 章 / 共 3 章");
    expect(screen.queryByTestId("chapter-slider-bubble")).toBeNull();
  });

  it("松手停回原章不当跳章，也不弹浮条", () => {
    renderReader(["/read/7/11"]);
    showToast.mockClear();
    const input = slider();

    act(() => {
      fireEvent.pointerDown(input, { clientX: 10, clientY: 800 });
      fireEvent.change(input, { target: { value: "1" } });
      fireEvent.pointerUp(input, { clientX: 10, clientY: 800 });
    });

    expect(showToast).not.toHaveBeenCalled();
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 2 章 / 共 3 章");
  });

  it("键盘可达：改值即跳章，Home / End 跳首末章", () => {
    renderReader(["/read/7/11"]);
    const input = slider();
    // 非拖动状态下改值（键盘 ← / → 走的就是 change）
    fireEvent.change(input, { target: { value: "2" } });
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 3 章 / 共 3 章");
  });

  it("Home / End 直接跳首章 / 末章", () => {
    renderReader(["/read/7/11"]);
    fireEvent.keyDown(slider(), { key: "End" });
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 3 章 / 共 3 章");
    fireEvent.keyDown(slider(), { key: "Home" });
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 3 章");
  });

  it("H5 底栏顶部也有一个进度滑块，和正文底部那个共用跳章口径", () => {
    renderReader(["/read/7/10"]);
    act(() => {
      fireEvent.pointerDown(screen.getByTestId("reader-swipe-area"), { clientX: 150, clientY: 300 });
      fireEvent.pointerUp(screen.getByTestId("reader-swipe-area"), { clientX: 150, clientY: 300 });
    });
    const bar = within(screen.getByTestId("reader-bar")).getByTestId("reader-bar-slider") as HTMLInputElement;
    expect(bar.type).toBe("range");
    fireEvent.change(bar, { target: { value: "1" } });
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 2 章 / 共 3 章");
  });
});

describe("目录 / 书签：合并成一个 tab 抽屉", () => {
  beforeEach(() => {
    store = [];
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  function drawer(): HTMLElement {
    return screen.getByTestId("reader-drawer");
  }

  it("点目录开在目录页签；抽屉里两个页签都在", () => {
    renderReader(["/read/7/10"], "pc");
    fireEvent.click(screen.getByRole("button", { name: "目录" }));
    expect(drawer().getAttribute("aria-hidden")).toBe("false");
    expect(screen.getByTestId("drawer-tab-toc").getAttribute("aria-selected")).toBe("true");
    expect(screen.getByTestId("drawer-tab-bookmarks").getAttribute("aria-selected")).toBe("false");
    expect(screen.getByTestId("drawer-meta").textContent).toContain("3 章");
  });

  it("切到书签页签不关抽屉，目录内容留在原地", () => {
    renderReader(["/read/7/10"], "pc");
    fireEvent.click(screen.getByRole("button", { name: "目录" }));
    fireEvent.click(screen.getByTestId("toc-search-input"));
    fireEvent.change(screen.getByTestId("toc-search-input"), { target: { value: "第三章" } });
    expect(screen.getByTestId("toc-match").textContent).toContain("匹配 1 章");

    fireEvent.click(screen.getByTestId("drawer-tab-bookmarks"));
    expect(drawer().getAttribute("aria-hidden")).toBe("false");
    expect(screen.getByTestId("drawer-tab-bookmarks").getAttribute("aria-selected")).toBe("true");
    expect(screen.getByTestId("bookmark-empty")).toBeTruthy();

    // 切回来：搜索框里的关键词还在（页签切换没有重置浏览状态）
    fireEvent.click(screen.getByTestId("drawer-tab-toc"));
    expect((screen.getByTestId("toc-search-input") as HTMLInputElement).value).toBe("第三章");
  });

  it("点书签直接开在书签页签；加书签后计数出现在页签上", () => {
    renderReader(["/read/7/10"]);
    act(() => {
      fireEvent.pointerDown(screen.getByTestId("reader-swipe-area"), { clientX: 150, clientY: 300 });
      fireEvent.pointerUp(screen.getByTestId("reader-swipe-area"), { clientX: 150, clientY: 300 });
    });
    fireEvent.click(within(screen.getByTestId("reader-bar")).getByLabelText("打开书签"));
    expect(screen.getByTestId("drawer-tab-bookmarks").getAttribute("aria-selected")).toBe("true");
    expect(screen.getByTestId("bookmark-empty")).toBeTruthy();
  });

  it("页签支持 ← / → 键切换", () => {
    renderReader(["/read/7/10"], "pc");
    fireEvent.click(screen.getByRole("button", { name: "目录" }));
    fireEvent.keyDown(screen.getByTestId("drawer-tab-toc"), { key: "ArrowRight" });
    expect(screen.getByTestId("drawer-tab-bookmarks").getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(screen.getByTestId("drawer-tab-bookmarks"), { key: "ArrowLeft" });
    expect(screen.getByTestId("drawer-tab-toc").getAttribute("aria-selected")).toBe("true");
  });

  it("关闭抽屉后页签与浏览状态重置（下次打开是干净的一页）", () => {
    renderReader(["/read/7/10"], "pc");
    fireEvent.click(screen.getByRole("button", { name: "打开书签" }));
    fireEvent.click(screen.getByTestId("drawer-tab-toc"));
    fireEvent.change(screen.getByTestId("toc-search-input"), { target: { value: "第三章" } });
    fireEvent.click(within(drawer()).getByRole("button", { name: "关闭目录与书签" }));
    expect(drawer().getAttribute("aria-hidden")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "目录" }));
    expect((screen.getByTestId("toc-search-input") as HTMLInputElement).value).toBe("");
  });
});

describe("章末衔接：没有按钮卡片，下一章正文直接接在下面", () => {
  beforeEach(() => {
    store = [];
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  it("非末章：章末没有任何翻章按钮，下一章的标题与正文已经接在下面", () => {
    renderReader(["/read/7/10"]);
    // 章末只剩一行轻分隔，不再是「下一章」大按钮卡片
    expect(screen.queryByRole("button", { name: /下一章/ })).toBeNull();
    expect(screen.getByTestId("reader-stream-mark").textContent).toContain("本章完");
    // 第二章的正文已经在同一棵 DOM 里：读者滚过章界线就是它，不用点
    const next = screen.getByTestId("reader-segment-11");
    expect(next.textContent).toContain("第二章 落雨");
    expect(next.textContent).toContain("第2章的正文。");
  });

  it("末章：给「全书读完」+ 回详情页 / 回书架两个出口", () => {
    renderReader(["/read/7/12"]);
    const end = screen.getByTestId("reader-pager-end");
    expect(end.textContent).toContain("全书读完");
    expect(within(end).getByRole("link", { name: "回详情页" })).toBeTruthy();
    fireEvent.click(within(end).getByRole("button", { name: "回书架" }));
    expect(screen.getByText("书架页 restore")).toBeTruthy();
  });

  it("首章：上一章按钮禁用并写明原因，按钮本身还在（位置不跳）", () => {
    renderReader(["/read/7/10"]);
    const prev = screen.getByRole("button", { name: "← 上一章" });
    expect(prev.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("已经是第一章了")).toBeTruthy();
    // 翻到第二章后同一颗按钮换到可点态——说明它没被隐藏，只是禁用
    act(() => {
      fireEvent.keyDown(window, { key: "ArrowRight" });
    });
    expect(screen.getByRole("button", { name: "← 上一章" }).hasAttribute("disabled")).toBe(false);
  });
});

describe("阅读进度常驻顶栏（章末不再重复统计）", () => {
  beforeEach(() => {
    store = [];
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  it("章号与字数由顶栏承担，章末分隔只剩「本章完」", () => {
    renderReader(["/read/7/10"], "pc");
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 3 章 · 2,000 字");
    act(() => {
      fireEvent.keyDown(window, { key: "ArrowRight" });
    });
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 2 章 / 共 3 章 · 2,100 字");
    // 章末不再印字数 / 时长 / 进度：这些都是常驻状态
    expect(screen.queryByTestId("reader-word")).toBeNull();
    expect(screen.queryByTestId("reader-chapter-time")).toBeNull();
    expect(screen.getByTestId("reader-stream-mark").textContent).toBe("本章完");
  });

  it("全书剩余时长按目录剩余章字数求和，翻章后跟着变小（PC 顶栏）", () => {
    renderReader(["/read/7/10"], "pc");
    // 第 1 章时剩第 2 + 3 章 = 2100 + 2200 = 4300 字 → 9 分钟
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("还剩约 9 分钟");
    act(() => {
      fireEvent.keyDown(window, { key: "ArrowRight" });
    });
    // 第 2 章时只剩第 3 章 = 2200 字 → 4 分钟
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("还剩约 4 分钟");
  });

  it("末章没有剩余章节：顶栏不编「还剩约」，流末尾给「全书读完」出口", () => {
    renderReader(["/read/7/12"], "pc");
    const progress = screen.getByTestId("reader-top-progress").textContent ?? "";
    expect(progress).toContain("第 3 章 / 共 3 章");
    expect(progress).not.toContain("还剩约");
    expect(screen.getByTestId("reader-pager-end").textContent).toContain("全书读完");
  });

  it("H5 顶栏把「全书 % · 还剩约」换到第二行小字，数据与 PC 同一份", () => {
    renderReader(["/read/7/10"], "h5");
    const main = screen.getByTestId("reader-top-progress").textContent ?? "";
    expect(main).toContain("第 1 章 / 共 3 章 · 2,000 字");
    // 主进度行只放章号与字数，全书/剩余换到第二行，避免挤掉书名
    expect(main).not.toContain("全书");

    const sub = screen.getByTestId("reader-top-progress-sub").textContent ?? "";
    expect(sub).toContain("全书 33%");
    expect(sub).toContain("还剩约 9 分钟");
  });

  it("H5 末章：第二行只有「全书 %」，不编「还剩约」", () => {
    renderReader(["/read/7/12"], "h5");
    const sub = screen.getByTestId("reader-top-progress-sub").textContent ?? "";
    expect(sub).toContain("全书 100%");
    expect(sub).not.toContain("还剩约");
  });
});
