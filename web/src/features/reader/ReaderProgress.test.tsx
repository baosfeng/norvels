import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, Bookmark, ChapterDetail, ReadingProgress } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { pickStoredParaIndex } from "./readerProgress";

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

/** 3 段正文：段落下标 0 / 1 / 2。 */
const CONTENT = ["雨从傍晚开始下。", "他站在巷口。", "远处有人在收摊。"].join("\n");

function chapterOf(chapterId: number): ChapterDetail {
  const brief = chapters.find((c) => c.id === chapterId) ?? chapters[0];
  return {
    bookId: 7,
    bookTitle: book.title,
    id: brief.id,
    idx: brief.idx,
    title: brief.title,
    content: CONTENT,
    wordCount: brief.wordCount,
    prevId: 0,
    nextId: 0,
  };
}

const spies = vi.hoisted(() => ({
  saveProgress: vi.fn(async () => true),
  readingProgress: vi.fn(async () => ({ items: [] as ReadingProgress[] })),
  createBookmark: vi.fn(async () => ({}) as Bookmark),
  bookmarkList: vi.fn(async () => ({ bookmarks: [] as Bookmark[] })),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      readingProgress: spies.readingProgress,
      createBookmark: spies.createBookmark,
      bookmarks: spies.bookmarkList,
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
  useSaveProgress: () => spies.saveProgress,
  useRecordReadingSession: () => vi.fn(async () => true),
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

/** 手摇 IntersectionObserver：用例自己决定「哪一段进了视口」。 */
class FakeObserver {
  static instances: FakeObserver[] = [];
  callback: IntersectionObserverCallback;
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    FakeObserver.instances.push(this);
  }
  observe() {}
  unobserve() {}
  disconnect() {
    FakeObserver.instances = FakeObserver.instances.filter((item) => item !== this);
  }
  takeRecords() {
    return [];
  }
  static emit(paraIndex: number) {
    const node = document.getElementById(`para-${paraIndex}`);
    if (!node) throw new Error(`没有第 ${paraIndex} 段`);
    for (const observer of FakeObserver.instances) {
      observer.callback(
        [{ target: node, isIntersecting: true } as unknown as IntersectionObserverEntry],
        observer as unknown as IntersectionObserver,
      );
    }
  }
  static reset() {
    FakeObserver.instances = [];
  }
}

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

/** 记录「哪个 id 被 scrollIntoView 了」：位置恢复必须落在目标段上。 */
const scrolled: string[] = [];

function progressRow(paraIndex: number, chapterId = 10): ReadingProgress {
  return {
    bookId: 7,
    bookTitle: book.title,
    chapterId,
    chapterIdx: 0,
    chapterTitle: "第一章",
    percent: 0.5,
    paraIndex,
    updatedAt: "2026-10-25T00:00:00.000Z",
  };
}

beforeEach(() => {
  localStorage.clear();
  scrolled.length = 0;
  spies.saveProgress.mockClear();
  spies.readingProgress.mockReset();
  spies.readingProgress.mockImplementation(async () => ({ items: [] }));
  vi.stubGlobal("IntersectionObserver", FakeObserver);
  FakeObserver.reset();
  (HTMLElement.prototype as unknown as { scrollIntoView: () => void }).scrollIntoView = function scrollIntoView(this: HTMLElement) {
    scrolled.push(this.id);
  };
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  vi.spyOn(window, "scrollBy").mockImplementation(() => {});
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  delete (HTMLElement.prototype as unknown as { scrollIntoView?: () => void }).scrollIntoView;
});

describe("章内位置记忆：纯函数", () => {
  it("只有「本书 + 本章」对得上才返回 paraIndex", () => {
    const items = [progressRow(3)];
    expect(pickStoredParaIndex(items, 7, 10)).toBe(3);
    // 章节对不上（进度停在别的章）：返回 0，不要拿别的章的下标乱滚
    expect(pickStoredParaIndex(items, 7, 11)).toBe(0);
    // 书对不上
    expect(pickStoredParaIndex(items, 8, 10)).toBe(0);
    // 没有进度 / 脏值
    expect(pickStoredParaIndex([], 7, 10)).toBe(0);
    expect(pickStoredParaIndex([{ ...progressRow(0), paraIndex: -2 }], 7, 10)).toBe(0);
    expect(pickStoredParaIndex([{ ...progressRow(0), paraIndex: undefined }], 7, 10)).toBe(0);
  });
});

describe("章内位置记忆：进章节恢复 + 上报时机", () => {
  it("进入章节按 paraIndex 滚回原段，并把首屏位置一起上报", async () => {
    spies.readingProgress.mockImplementation(async () => ({ items: [progressRow(2)] }));
    renderReader();

    await waitFor(() => expect(scrolled).toContain("para-2"));
    await waitFor(() =>
      expect(spies.saveProgress).toHaveBeenCalledWith(expect.objectContaining({ bookId: 7, chapterId: 10, paraIndex: 2 })),
    );
  });

  it("没有进度时停在章首，不上报非 0 的 paraIndex", async () => {
    renderReader();
    await waitFor(() => expect(spies.saveProgress).toHaveBeenCalled());
    expect(scrolled).toHaveLength(0);
    expect(spies.saveProgress).toHaveBeenCalledWith(expect.objectContaining({ chapterId: 10, paraIndex: 0 }));
  });

  it("滚动停下 1 秒才上报 paraIndex（不是每滚一次就发）", async () => {
    renderReader();
    await waitFor(() => expect(spies.saveProgress).toHaveBeenCalledTimes(1));
    spies.saveProgress.mockClear();

    vi.useFakeTimers();
    act(() => FakeObserver.emit(1));
    act(() => {
      vi.advanceTimersByTime(600);
    });
    expect(spies.saveProgress).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(spies.saveProgress).toHaveBeenCalledWith(expect.objectContaining({ chapterId: 10, paraIndex: 1 }));
    expect(spies.saveProgress).toHaveBeenCalledTimes(1);
  });

  it("切章时立即补发，不等防抖", async () => {
    renderReader();
    await waitFor(() => expect(spies.saveProgress).toHaveBeenCalled());
    spies.saveProgress.mockClear();

    act(() => FakeObserver.emit(1));
    // 还没到 1 秒就切章
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: /下一章/ }));
    });
    expect(spies.saveProgress).toHaveBeenCalledWith(expect.objectContaining({ chapterId: 10, paraIndex: 1 }));
  });

  it("页面隐藏时立即补发", async () => {
    renderReader();
    await waitFor(() => expect(spies.saveProgress).toHaveBeenCalled());
    spies.saveProgress.mockClear();

    act(() => FakeObserver.emit(2));
    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" });
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });
    expect(spies.saveProgress).toHaveBeenCalledWith(expect.objectContaining({ chapterId: 10, paraIndex: 2 }));
  });
});

describe("书签抽屉：正在保存用真实状态", () => {
  it("点「在这里加书签」后按钮变「正在保存…」并禁用，保存完恢复", async () => {
    let release: () => void = () => {};
    spies.createBookmark.mockImplementation(
      () =>
        new Promise((resolve) => {
          release = () => resolve({} as Bookmark);
        }),
    );
    renderReader();

    fireEvent.click(screen.getByLabelText("打开书签"));
    const addButton = screen.getByTestId("bookmark-add") as HTMLButtonElement;
    expect(addButton.textContent).toBe("在这里加书签");

    fireEvent.click(addButton);
    await waitFor(() => expect(addButton.textContent).toBe("正在保存…"));
    expect(addButton.disabled).toBe(true);

    await act(async () => {
      release();
      await Promise.resolve();
    });
    await waitFor(() => expect(addButton.textContent).toBe("在这里加书签"));
    expect(addButton.disabled).toBe(false);
  });
});
