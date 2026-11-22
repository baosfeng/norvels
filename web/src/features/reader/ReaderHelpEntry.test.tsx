import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterBrief, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { READER_KEYS } from "./readerSettings";
import { READER_TOUCH_HINTS } from "./readerConstants";

/**
 * 手机上的帮助入口（U15/U16）。
 *
 * 曾经只有 PC 顶栏有「?」，手机 5 颗图标里没有它——读者想确认「点左边缘会发生什么」
 * 只能猜。这里守两件事：H5 顶栏有这个入口且能打开现有帮助面板；帮助文案不再写
 * 页边渐变提示（DOM 里从来没有那个元素，见 ReaderGestures 的「正文两侧没有装饰层」）。
 */

const chapters: ChapterBrief[] = [
  { id: 10, idx: 0, title: "第一章 起风", volumeId: 1, wordCount: 2000 },
  { id: 11, idx: 1, title: "第二章 落雨", volumeId: 1, wordCount: 2100 },
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

function chapterOf(id: number): ChapterDetail {
  const brief = chapters.find((c) => c.id === id) ?? chapters[0];
  return {
    bookId: 7,
    bookTitle: book.title,
    id: brief.id,
    idx: brief.idx,
    title: brief.title,
    content: "雨从傍晚开始下。\n\n他站在巷口，没有打伞。",
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: { ...actual.api, readingProgress: () => new Promise(() => {}) },
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
  useBookmarks: () => ({ data: { bookmarks: [] }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({ theme: "day", resolvedTheme: "day", setTheme: vi.fn(), themes: ["day", "system"] }),
    prefersReducedMotion: () => false,
  };
});

const { ReaderPage } = await import("./ReaderPage");

function renderH5() {
  return render(
    <TierProvider value="h5">
      <MemoryRouter initialEntries={["/read/7/10"]}>
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情</p>} />
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

function revealControls() {
  const surface = screen.getByTestId("reader-swipe-area");
  act(() => {
    fireEvent.pointerDown(surface, { clientX: 150, clientY: 300 });
    fireEvent.pointerUp(surface, { clientX: 150, clientY: 300 });
  });
}

describe("H5 帮助入口", () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem(READER_KEYS.pageMode, "scroll");
    Object.defineProperty(HTMLElement.prototype, "clientWidth", { value: 300, configurable: true });
  });

  it("唤出控制层后顶栏有「快捷键帮助」，点了打开现有面板", () => {
    renderH5();
    revealControls();

    const trigger = screen.getByRole("button", { name: "快捷键帮助" });
    expect(trigger.getAttribute("aria-hidden")).toBeNull();

    fireEvent.click(trigger);
    const dialog = screen.getByRole("dialog", { name: "快捷键帮助" });
    expect(dialog.textContent).toContain("触摸设备");
    // 面板里列的是真实存在的手势
    expect(dialog.textContent).toContain("单击中间");
  });

  it("再次点击收起；收起态整条顶栏对读屏与 Tab 都不可达（无障碍不回退）", () => {
    renderH5();
    revealControls();
    const trigger = screen.getByRole("button", { name: "快捷键帮助" });
    fireEvent.click(trigger);
    expect(screen.queryByRole("dialog", { name: "快捷键帮助" })).toBeTruthy();

    fireEvent.click(trigger);
    expect(screen.queryByRole("dialog", { name: "快捷键帮助" })).toBeNull();
  });

  it("控制层收起时帮助入口不在焦点顺序里（tabIndex=-1，整条顶栏 aria-hidden）", () => {
    renderH5();
    // 未唤出控制层：顶栏对读屏隐藏，按钮也 Tab 不进去——无障碍不回退
    expect(screen.getByTestId("reader-top").getAttribute("aria-hidden")).toBe("true");
    expect(screen.getByRole("button", { name: "快捷键帮助", hidden: true }).tabIndex).toBe(-1);
  });

  it("帮助文案不再提「页边有淡淡的渐变提示」（DOM 里没有该元素）", () => {
    for (const hint of READER_TOUCH_HINTS) {
      expect(hint.desc).not.toContain("渐变");
    }
    renderH5();
    revealControls();
    fireEvent.click(screen.getByRole("button", { name: "快捷键帮助" }));
    expect(screen.getByRole("dialog", { name: "快捷键帮助" }).textContent).not.toContain("渐变");
  });
});
