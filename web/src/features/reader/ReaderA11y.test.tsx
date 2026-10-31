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
  return { ...actual, api: { ...actual.api, readingProgress: () => new Promise(() => {}) } };
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

function revealControls() {
  const surface = screen.getByTestId("reader-swipe-area");
  act(() => {
    fireEvent.pointerDown(surface, { clientX: 120, clientY: 260 });
    fireEvent.pointerUp(surface, { clientX: 120, clientY: 260 });
  });
}

function topBarButtons(): HTMLButtonElement[] {
  return Array.from(screen.getByTestId("reader-top").querySelectorAll("button"));
}

function barButtons(): HTMLButtonElement[] {
  return Array.from(screen.getByTestId("reader-bar").querySelectorAll("button"));
}

beforeEach(() => {
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

afterEach(() => {
  vi.restoreAllMocks();
});

/**
 * 键盘可达性口径：
 * - 隐藏用 aria-hidden 只对读屏生效，键盘仍能 Tab 进屏幕外的按钮 → 收起态给容器 inert；
 * - 老浏览器不认 inert 时，退化靠子元素 tabIndex=-1（两条都断言，删掉任一条都能被抓到）。
 * jsdom 不实现 inert 的焦点语义，所以这里只守「属性在不在」，真机按 Tab 走一圈由验收覆盖。
 */
describe("阅读器控制层的键盘可达性", () => {
  it("H5 收起态：顶栏与底栏带 inert，按钮与滑块都 tabIndex=-1", () => {
    renderReader("h5");
    const top = screen.getByTestId("reader-top");
    const bar = screen.getByTestId("reader-bar");
    expect(top.hasAttribute("inert")).toBe(true);
    expect(bar.hasAttribute("inert")).toBe(true);
    expect(top.getAttribute("aria-hidden")).toBe("true");

    for (const button of [...topBarButtons(), ...barButtons()]) {
      expect(button.getAttribute("tabindex")).toBe("-1");
    }
    expect(screen.getByTestId("reader-bar-slider").getAttribute("tabindex")).toBe("-1");
  });

  it("H5 唤出后：inert 移除，按钮恢复可聚焦", () => {
    renderReader("h5");
    revealControls();

    const top = screen.getByTestId("reader-top");
    const bar = screen.getByTestId("reader-bar");
    expect(top.hasAttribute("inert")).toBe(false);
    expect(bar.hasAttribute("inert")).toBe(false);
    for (const button of [...topBarButtons(), ...barButtons()]) {
      expect(button.getAttribute("tabindex")).not.toBe("-1");
    }
    expect(screen.getByTestId("reader-bar-slider").getAttribute("tabindex")).not.toBe("-1");
  });

  it("PC：顶栏常驻，从不加 inert（键盘随时可达）", () => {
    renderReader("pc");
    expect(screen.getByTestId("reader-top").hasAttribute("inert")).toBe(false);
    expect(screen.queryByTestId("reader-bar")).toBeNull();
  });

  it("返回按钮的读屏名是「返回书籍详情」，不再是「返回目录」", () => {
    renderReader("pc");
    const back = screen.getByLabelText("返回书籍详情");
    expect(screen.queryByLabelText("返回目录")).toBeNull();

    fireEvent.click(back);
    expect(screen.getByText("书籍详情")).toBeTruthy();
  });
});
