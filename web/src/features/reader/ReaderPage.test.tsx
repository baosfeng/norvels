import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterDetail } from "../../api/types";
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

const chapterState = vi.fn((_bookId: number, chapterId: number) => ({
  data: Number.isFinite(chapterId) ? chapterOf(chapterId) : null,
  loading: false,
  error: null,
  reload: vi.fn(),
}));

vi.mock("../../api/hooks", () => ({
  useBook: () => ({ data: book, loading: false, error: null, reload: vi.fn() }),
  useChapter: (bookId: number, chapterId: number) => chapterState(bookId, chapterId),
  useSaveProgress: () => vi.fn(async () => true),
}));

vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({ theme: "day", setTheme: vi.fn(), themes: ["day", "night", "parchment"] }),
    prefersReducedMotion: () => false,
  };
});

const { ReaderPage } = await import("./ReaderPage");

function renderReader(tier: "h5" | "pc" = "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter initialEntries={["/read/7/10"]}>
        <Routes>
          <Route path="/read/:bookId/:chapterId" element={<ReaderPage />} />
          <Route path="/book/:bookId" element={<p>书籍详情</p>} />
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

  it("浏览器没有全屏 API 时不渲染全屏按钮", () => {
    renderReader();

    expect(screen.queryByLabelText("全屏阅读")).toBeNull();
    expect(screen.queryByLabelText("退出全屏")).toBeNull();
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
