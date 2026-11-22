import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterBrief, ChapterDetail } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { HINT_SEEN_KEY, ZONE_EDGE, ZONE_EDGE_SCROLL, resolveTapZone } from "../../lib/useReaderControls";
import { READER_KEYS } from "./readerSettings";

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

function surface(): HTMLElement {
  return screen.getByTestId("reader-swipe-area");
}

/** jsdom 里 clientWidth 恒为 0：给它一个真实宽度，三区才有意义。 */
function stubWidth(width: number) {
  Object.defineProperty(HTMLElement.prototype, "clientWidth", { value: width, configurable: true });
}

function tapAt(clientX: number) {
  fireEvent.pointerDown(surface(), { clientX, clientY: 300 });
  fireEvent.pointerUp(surface(), { clientX, clientY: 300 });
}

afterEach(() => {
  vi.restoreAllMocks();
  // 还原 clientWidth，避免污染其它用例
  Reflect.deleteProperty(HTMLElement.prototype, "clientWidth");
});

describe("三区手势：分区判定纯函数", () => {
  // 30/40/30 是主流阅读器的默认分区（边缘可调区间 20%–40%）：边太宽会让读者
  // 在以为点的是正文时被翻章，边太窄又点不中。300 宽下边界落在 90 与 210。
  it("边缘 30%、中间 40%（主流默认）：左上一章、中菜单、右下一章", () => {
    expect(ZONE_EDGE).toBe(0.3);
    expect(resolveTapZone(10, 300)).toBe("prev");
    expect(resolveTapZone(89, 300)).toBe("prev");
    expect(resolveTapZone(150, 300)).toBe("menu");
    expect(resolveTapZone(250, 300)).toBe("next");
    expect(resolveTapZone(299, 300)).toBe("next");
  });

  it("边界本身归中区：宁可唤出菜单，也不悄悄翻章", () => {
    expect(resolveTapZone(90, 300)).toBe("menu");
    expect(resolveTapZone(210, 300)).toBe("menu");
    expect(resolveTapZone(89.9, 300)).toBe("prev");
    expect(resolveTapZone(210.1, 300)).toBe("next");
  });

  it("宽度不可用（未布局 / SSR）时一律当中区，绝不悄悄翻章", () => {
    expect(resolveTapZone(10, 0)).toBe("menu");
    expect(resolveTapZone(10, -1)).toBe("menu");
    expect(resolveTapZone(Number.NaN, 300)).toBe("menu");
  });

  /**
   * U12：滚动模式读者会贴着边缘往下捋，落点常在 25%–30% 那一带。
   * 旧实现一律按 30% 判，于是 x=25% 被当成「翻章」，还弹一句 4 秒浮条（改前实测 361 px / 300 宽 = 30% 才进中区）。
   * 分页模式保持 30%：那边的边缘本来就是「上一页 / 下一页」的主控区，不许跟着缩。
   */
  it("U12 滚动模式边缘 22%：x=25% 落中区唤菜单，分页模式仍是 30%", () => {
    expect(ZONE_EDGE).toBe(0.3);
    expect(ZONE_EDGE_SCROLL).toBe(0.22);

    // 300 宽：滚动模式的中区从 66 起，分页模式要到 90 才算中区
    expect(resolveTapZone(75, 300, ZONE_EDGE_SCROLL)).toBe("menu");
    expect(resolveTapZone(75, 300, ZONE_EDGE)).toBe("prev");
    // 两侧对称：右边缘同理
    expect(resolveTapZone(225, 300, ZONE_EDGE_SCROLL)).toBe("menu");
    expect(resolveTapZone(225, 300, ZONE_EDGE)).toBe("next");
    // 真的贴边还是翻章，22% 没把边缘整个吃掉
    expect(resolveTapZone(20, 300, ZONE_EDGE_SCROLL)).toBe("prev");
    expect(resolveTapZone(280, 300, ZONE_EDGE_SCROLL)).toBe("next");
  });
});

describe("三区手势：H5 单击分区", () => {
  beforeEach(() => {
    localStorage.clear();
    // 首次进阅读器的手势提示已在别的用例里守：这里钉成「看过」，免得它挡住点击探针
    localStorage.setItem(HINT_SEEN_KEY, "1");
    // 三区点击在滚动模式下是翻一章；分页模式另有专门用例守「翻一页」
    localStorage.setItem(READER_KEYS.pageMode, "scroll");
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    stubWidth(300);
  });

  it("点中间唤出控制层；再点中间收起", () => {
    renderReader();
    expect(screen.getByTestId("reader-top").getAttribute("aria-hidden")).toBe("true");
    act(() => tapAt(150));
    expect(screen.getByTestId("reader-top").getAttribute("aria-hidden")).toBe("false");
    act(() => tapAt(150));
    expect(screen.getByTestId("reader-top").getAttribute("aria-hidden")).toBe("true");
  });

  it("点右区翻到下一章，并浮条说明翻到了哪", () => {
    renderReader();
    act(() => tapAt(250));
    expect(showToast).toHaveBeenCalledWith("已翻到第 2 章");
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 2 章 / 共 3 章");
  });

  it("点左区翻到上一章；第一章再点只提示不动", () => {
    renderReader(["/read/7/11"]);
    act(() => tapAt(10));
    expect(showToast).toHaveBeenCalledWith("已翻到第 1 章");
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 3 章");

    showToast.mockClear();
    act(() => tapAt(10));
    expect(showToast).toHaveBeenCalledWith("已经是第一章了");
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 3 章");
  });

  it("最后一章点右区只提示已经是最后一章", () => {
    renderReader(["/read/7/12"]);
    act(() => tapAt(280));
    expect(showToast).toHaveBeenCalledWith("已经是最后一章了");
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 3 章 / 共 3 章");
  });

  it("设置面板打开时短按不翻章（只切控制层）", () => {
    renderReader();
    act(() => tapAt(150));
    act(() => fireEvent.click(within(screen.getByTestId("reader-bar")).getByLabelText("阅读设置")));
    expect(screen.getByTestId("reader-settings").getAttribute("aria-hidden")).toBe("false");
    showToast.mockClear();
    act(() => tapAt(280));
    expect(showToast).not.toHaveBeenCalledWith("已翻到第 3 章");
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 3 章");
    // 面板还开着：短按没有被当成「收起控制层」
    expect(screen.getByTestId("reader-settings").getAttribute("aria-hidden")).toBe("false");
  });

  /**
   * U12 的端到端口径：滚动模式（本组已钉 pageMode=scroll）下点 x=25%（300 宽里的 75 px：
   * 改前落在 30% 边缘区 = 翻章 + 4 秒浮条），现在属于中区——章节不变，控制层被唤出。
   */
  it("U12 滚动模式点 x=25%：章节不变、唤出控制层（改前直接翻章并弹浮条）", () => {
    renderReader();
    showToast.mockClear();
    act(() => tapAt(75));
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 3 章");
    expect(showToast).not.toHaveBeenCalled();
    expect(screen.getByTestId("reader-top").getAttribute("aria-hidden")).toBe("false");
  });

  it("滑动不算单击：位移超过阈值既不翻章也不切控制层", () => {
    renderReader();
    fireEvent.pointerDown(surface(), { clientX: 280, clientY: 300 });
    fireEvent.pointerUp(surface(), { clientX: 60, clientY: 305 });
    expect(screen.getByTestId("reader-top").getAttribute("aria-hidden")).toBe("true");
    expect(screen.getByTestId("reader-top-progress").textContent).toContain("第 1 章 / 共 3 章");
  });

  // 沉浸式：纯阅读态正文两侧不留任何装饰（曾经的 24×88 灰胶囊压在字上，实测像没加载出来的占位）。
  // 「左右可滑」的可发现性由翻章浮条与快捷键帮助承担，不靠常驻视觉——主流阅读器也都不放。
  it("纯阅读态正文两侧没有任何装饰层", () => {
    renderReader();
    expect(screen.queryByTestId("reader-edge-hints")).toBeNull();
    expect(document.querySelectorAll(".reader-edge-hint")).toHaveLength(0);
  });
});

/**
 * U18：第一次进阅读器是满屏正文，没人告诉读者「点中间出菜单」。
 * 判据：没有 seenHint 标记时渲染一次性提示（1.5 秒自淡），写完标记后不再渲染。
 */
describe("U18 首次进阅读器的手势提示", () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem(READER_KEYS.pageMode, "scroll");
    stubWidth(300);
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("没有 seenHint 时渲染一行提示；1.5 秒后消失并写下标记", () => {
    vi.useFakeTimers();
    renderReader();
    expect(screen.getByTestId("reader-first-hint").textContent).toContain("单击屏幕中间唤出菜单");

    act(() => {
      vi.advanceTimersByTime(1499);
    });
    expect(screen.queryByTestId("reader-first-hint")).not.toBeNull();

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(screen.queryByTestId("reader-first-hint")).toBeNull();
    expect(localStorage.getItem(HINT_SEEN_KEY)).toBe("1");
  });

  it("已有 seenHint 标记时不再渲染（之后进任何书都不出现）", () => {
    localStorage.setItem(HINT_SEEN_KEY, "1");
    renderReader();
    expect(screen.queryByTestId("reader-first-hint")).toBeNull();
  });
});
