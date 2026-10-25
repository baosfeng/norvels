import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { READER_KEYS } from "./readerSettings";

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


// 阅读器会用 /reading/progress 恢复章内位置：这些用例不关心真实请求，统一给空列表，
// 避免 jsdom 里发真网络请求把测试拖慢（也避免 act 警告）。
vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: { ...actual.api, readingProgress: async () => ({ items: [] }) },
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
        </Routes>
      </MemoryRouter>
    </TierProvider>,
  );
}

function openSettings() {
  fireEvent.click(screen.getByRole("button", { name: "设置" }));
}

function autoSwitch() {
  return screen.getByRole("switch", { name: "自动滚动" });
}

function state() {
  return screen.getByTestId("reader-autoscroll-state").textContent;
}

describe("阅读器：自动滚动", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
    // jsdom 没有真实滚动容器：给一份「长正文」度量，避免一开滚就被判定到底
    Object.defineProperty(document.documentElement, "scrollHeight", { configurable: true, value: 6000 });
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 800 });
    Object.defineProperty(window, "scrollY", { configurable: true, value: 0 });
    vi.spyOn(window, "scrollBy").mockImplementation(() => {});
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("默认关闭：没有悬浮条，设置里也不滚动", () => {
    renderReader();
    expect(screen.queryByTestId("reader-autoscroll")).toBeNull();
    openSettings();
    expect(autoSwitch().getAttribute("aria-checked")).toBe("false");
  });

  it("开启后出现悬浮条；设置面板还开着时处于暂停，关掉面板即开始滚", () => {
    renderReader();
    openSettings();

    fireEvent.click(autoSwitch());
    expect(autoSwitch().getAttribute("aria-checked")).toBe("true");
    expect(state()).toBe("已暂停");
    expect(screen.getByTestId("reader-autoscroll-toggle").textContent).toBe("继续");

    // 关掉设置面板：进入自动滚动
    fireEvent.click(screen.getByRole("button", { name: "关闭阅读设置" }));
    expect(state()).toBe("自动滚动中");
    expect(screen.getByTestId("reader-autoscroll-toggle").textContent).toBe("暂停");
  });

  it("手动暂停 / 继续；退出后开关复位、悬浮条消失", () => {
    renderReader();
    openSettings();
    fireEvent.click(autoSwitch());
    fireEvent.click(screen.getByRole("button", { name: "关闭阅读设置" }));

    fireEvent.click(screen.getByTestId("reader-autoscroll-toggle"));
    expect(state()).toBe("已暂停");
    fireEvent.click(screen.getByTestId("reader-autoscroll-toggle"));
    expect(state()).toBe("自动滚动中");

    fireEvent.click(screen.getByRole("button", { name: "退出自动滚动" }));
    expect(screen.queryByTestId("reader-autoscroll")).toBeNull();
    expect(localStorage.getItem(READER_KEYS.autoScroll)).toBe("false");
    // 面板关着时 aria-hidden，开关要重新打开设置才能取到
    openSettings();
    expect(autoSwitch().getAttribute("aria-checked")).toBe("false");
  });

  it("失焦暂停、回焦继续", () => {
    renderReader();
    openSettings();
    fireEvent.click(autoSwitch());
    fireEvent.click(screen.getByRole("button", { name: "关闭阅读设置" }));
    expect(state()).toBe("自动滚动中");

    act(() => {
      window.dispatchEvent(new Event("blur"));
    });
    expect(state()).toBe("已暂停");

    act(() => {
      window.dispatchEvent(new Event("focus"));
    });
    expect(state()).toBe("自动滚动中");
  });

  it("速度 0.5 步进：默认 3 档，加减各半档，到 0.5 后减不动", () => {
    renderReader();
    openSettings();

    expect(screen.getByTestId("reader-speed-value").textContent).toBe("3");
    fireEvent.click(screen.getByRole("button", { name: "加快自动滚动" }));
    expect(screen.getByTestId("reader-speed-value").textContent).toBe("3.5");
    expect(localStorage.getItem(READER_KEYS.autoScrollSpeed)).toBe("3.5");

    fireEvent.click(screen.getByRole("button", { name: "减慢自动滚动" }));
    fireEvent.click(screen.getByRole("button", { name: "减慢自动滚动" }));
    expect(screen.getByTestId("reader-speed-value").textContent).toBe("2.5");

    // 连续减到下限 0.5 后按钮 disabled（0.5–5 共 10 档）
    for (let i = 0; i < 8; i += 1) fireEvent.click(screen.getByRole("button", { name: "减慢自动滚动" }));
    expect(screen.getByTestId("reader-speed-value").textContent).toBe("0.5");
    expect((screen.getByRole("button", { name: "减慢自动滚动" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("reader-speed-hint").textContent).toContain("7 像素/秒");
  });

  it("到底自动下一章默认关；开启后写进 localStorage", () => {
    renderReader();
    openSettings();

    const nextSwitch = screen.getByRole("switch", { name: "到底自动下一章" });
    expect(nextSwitch.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(nextSwitch);
    expect(localStorage.getItem(READER_KEYS.autoScrollNext)).toBe("true");
  });

  it("设置面板里自动滚动与速度成组出现，文案与档位一致", () => {
    renderReader();
    openSettings();
    const panel = screen.getByRole("dialog", { name: "阅读设置" });
    expect(within(panel).getByRole("switch", { name: "自动滚动" })).toBeTruthy();
    expect(within(panel).getByTestId("reader-speed-hint").textContent).toContain("0.5");
  });
});
