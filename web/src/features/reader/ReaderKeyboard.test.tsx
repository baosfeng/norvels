import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  chapterCount: 2,
  wordCount: 4100,
  volumeCount: 1,
  intro: "",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  volumes: [{ id: 1, idx: 0, title: "正文", chapters }],
};

function chapterOf(chapterId: number): ChapterDetail {
  const brief = chapters.find((c) => c.id === chapterId) ?? chapters[0];
  return {
    bookId: 7,
    bookTitle: book.title,
    id: brief.id,
    idx: brief.idx,
    title: brief.title,
    content: "雨从傍晚开始下。\n他站在巷口，没有打伞。",
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

vi.mock("../../api/hooks", () => ({
  useBook: () => ({ data: book, loading: false, error: null, reload: vi.fn() }),
  useChapter: (_bookId: number, chapterId: number) => ({
    data: chapterOf(chapterId),
    loading: false,
    error: null,
    reload: vi.fn(),
  }),
  useSaveProgress: () => vi.fn(async () => true),
  useBookmarks: () => ({ data: { bookmarks: [] }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({ theme: "day", resolvedTheme: "day", setTheme: vi.fn(), themes: ["day", "night", "parchment", "system"] }),
    prefersReducedMotion: () => false,
  };
});

const { ReaderPage } = await import("./ReaderPage");

function renderReader() {
  return render(
    <TierProvider value="pc">
      <MemoryRouter initialEntries={["/read/7/10"]}>
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情</p>} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

/** 一屏 = 0.85 视口高（测试里固定 800px → 680px）。 */
const SCREEN_PX = 680;

function press(key: string, options: { shiftKey?: boolean } = {}) {
  act(() => {
    fireEvent.keyDown(window, { key, shiftKey: options.shiftKey ?? false });
  });
}

describe("阅读器：键盘补全", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 800 });
    vi.spyOn(window, "scrollBy").mockImplementation(() => {});
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("空格 / PageDown 向下翻 0.85 屏", () => {
    renderReader();
    press(" ");
    expect(window.scrollBy).toHaveBeenLastCalledWith({ top: SCREEN_PX, behavior: "smooth" });
    press("PageDown");
    expect(window.scrollBy).toHaveBeenLastCalledWith({ top: SCREEN_PX, behavior: "smooth" });
    expect(window.scrollBy).toHaveBeenCalledTimes(2);
  });

  it("PageUp / Shift+空格 向上翻 0.85 屏", () => {
    renderReader();
    press("PageUp");
    expect(window.scrollBy).toHaveBeenLastCalledWith({ top: -SCREEN_PX, behavior: "smooth" });
    press(" ", { shiftKey: true });
    expect(window.scrollBy).toHaveBeenLastCalledWith({ top: -SCREEN_PX, behavior: "smooth" });
    expect(window.scrollBy).toHaveBeenCalledTimes(2);
  });

  it("方向键仍然翻章", () => {
    renderReader();
    press("ArrowRight");
    expect(screen.getByTestId("reader-top").textContent).toContain("第 2 / 2 章");
    press("ArrowLeft");
    expect(screen.getByTestId("reader-top").textContent).toContain("第 1 / 2 章");
  });

  it("? 打开快捷键帮助，列出 PageUp 与 Shift+空格，Esc 关闭", () => {
    renderReader();
    press("?", { shiftKey: true });

    const dialog = screen.getByRole("dialog", { name: "快捷键帮助" });
    const text = dialog.textContent ?? "";
    expect(text).toContain("PageUp");
    expect(text).toContain("PageDown");
    expect(text).toContain("Shift+空格");
    expect(text).toContain("上一章 / 下一章");

    press("Escape");
    expect(screen.queryByRole("dialog", { name: "快捷键帮助" })).toBeNull();

    // 用「点按钮」的方式打开（真实浏览器点击会给按钮焦点），Esc 后焦点回到原按钮
    const trigger = screen.getByRole("button", { name: "快捷键帮助" });
    act(() => trigger.focus());
    fireEvent.click(trigger);
    expect(screen.getByRole("dialog", { name: "快捷键帮助" })).toBeTruthy();
    press("Escape");
    expect(document.activeElement).toBe(trigger);
  });

  it("帮助面板打开时方向键不翻章", () => {
    renderReader();
    fireEvent.click(screen.getByRole("button", { name: "快捷键帮助" }));
    press("ArrowRight");
    expect(screen.getByTestId("reader-top").textContent).toContain("第 1 / 2 章");
  });

  it("正文末尾不再有那行快捷键提示", () => {
    renderReader();
    expect(document.querySelector(".reader-hint")).toBeNull();
    expect(screen.queryByText(/切换章节 · 空格/)).toBeNull();
    expect(screen.queryByText(/左右滑动或点上下章切换/)).toBeNull();
  });

  it("搜索框里打「?」不打开帮助", async () => {
    renderReader();
    fireEvent.click(screen.getByLabelText("搜索本章"));
    const input = screen.getByTestId("reader-search-input");
    input.focus();
    await waitFor(() => expect(document.activeElement).toBe(input));

    fireEvent.keyDown(input, { key: "?" });
    expect(screen.queryByRole("dialog", { name: "快捷键帮助" })).toBeNull();
  });
});
