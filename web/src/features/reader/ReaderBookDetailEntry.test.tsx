import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const chapters = [
  { id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 },
  { id: 11, idx: 1, title: "第二章", volumeId: 1, wordCount: 2100 },
];

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: chapters.length,
  wordCount: 4100,
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
    content: brief.idx === 0 ? "雨从傍晚开始下。" : "灯在巷尾亮着。",
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

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
      scrollToIndex: vi.fn(),
    };
  },
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    // 进度请求挂起：这些用例只关心顶栏出口，不需要真网络
    api: { ...actual.api, readingProgress: () => new Promise(() => {}) },
  };
});

vi.mock("../../api/hooks", () => ({
  useBook: () => ({ data: book, loading: false, error: null, reload: vi.fn() }),
  useChapter: (_bookId: number, chapterId: number) => ({
    data: Number.isFinite(chapterId) ? chapterOf(chapterId) : null,
    loading: false,
    error: null,
    reload: vi.fn(),
  }),
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

vi.mock("../../lib/toastStore", () => ({ showToast: vi.fn() }));

const { ReaderPage } = await import("./ReaderPage");

function renderReader(tier: "h5" | "pc") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={["/read/7/10"]}>
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情</p>} />
          <Route path="/shelf" element={<p>书架页</p>} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

/** H5 的控制层默认收起：先单击正文唤出来，才有点击顶栏的现实路径。 */
function revealControls() {
  const surface = screen.getByTestId("reader-swipe-area");
  act(() => {
    fireEvent.pointerDown(surface, { clientX: 120, clientY: 260 });
    fireEvent.pointerUp(surface, { clientX: 120, clientY: 260 });
  });
}

beforeEach(() => {
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

afterEach(() => {
  vi.restoreAllMocks();
});

/**
 * 「看书中能回详情页」两档各一个入口：
 * PC 是顶栏的「详情」按钮（与书架同规格），H5 是顶栏可点的书名（手机上没有横向空间再放按钮）。
 * 跳走再返回能回到原章节/滚动位置由阅读器既有的进度恢复负责（真机验证，jsdom 量不到滚动）。
 */
describe("阅读器回书籍详情", () => {
  it("PC：顶栏有「详情」按钮，点它进 /book/7", () => {
    renderReader("pc");
    const entry = screen.getByTestId("reader-top-detail");
    expect(entry.getAttribute("aria-label")).toBe("查看书籍详情");
    expect(entry.textContent).toBe("详情");

    fireEvent.click(entry);
    expect(screen.getByText("书籍详情")).toBeTruthy();
  });

  it("PC：书名处不是第二个入口（不重复渲染可点标题）", () => {
    renderReader("pc");
    expect(screen.getAllByTestId("reader-top-detail")).toHaveLength(1);
    expect(document.querySelector(".reader-top-title-btn")).toBeNull();
  });

  it("PC：「详情」与既有按钮同一规格（btn / 40px 高）", () => {
    renderReader("pc");
    const entry = screen.getByTestId("reader-top-detail");
    expect(entry.className).toContain("btn");
    expect(entry.tagName).toBe("BUTTON");
  });

  it("H5：顶栏书名可点，点它进 /book/7，进度行照旧显示", () => {
    renderReader("h5");
    revealControls();

    const title = screen.getByTestId("reader-top-detail");
    expect(title.tagName).toBe("BUTTON");
    expect(title.getAttribute("aria-label")).toBe("查看书籍详情");
    expect(title.className).toContain("reader-top-title-btn");
    // 书名与进度两行都还在（可点不等于改排版）
    expect(title.textContent).toContain("示例书简");
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 2 章");

    fireEvent.click(title);
    expect(screen.getByText("书籍详情")).toBeTruthy();
  });

  it("H5：不额外塞一颗「详情」文字按钮（入口就是书名）", () => {
    renderReader("h5");
    revealControls();
    const actions = document.querySelector(".reader-top-actions");
    expect(actions?.textContent).not.toContain("详情");
    expect(screen.getAllByTestId("reader-top-detail")).toHaveLength(1);
  });
});
