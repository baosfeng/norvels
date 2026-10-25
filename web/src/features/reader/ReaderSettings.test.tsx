import { fireEvent, render, screen, within } from "@testing-library/react";
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

/** 正文用繁体：验证「繁体转简体」开关真的改的是显示文本。 */
const CONTENT = ["燈光下的街道。", "他說明天再來。"].join("\n");

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

const setTheme = vi.fn();
vi.mock("../../lib/theme", async () => {
  const actual = await vi.importActual<typeof import("../../lib/theme")>("../../lib/theme");
  return {
    ...actual,
    useTheme: () => ({ theme: "day", resolvedTheme: "day", setTheme, themes: ["day", "night", "parchment", "system"] }),
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

function shellVar(name: string): string {
  return screen.getByTestId("view:reader").style.getPropertyValue(name);
}

/** PC 档的「设置」按钮在顶栏（H5 底栏那个是 aria-label「阅读设置」）。 */
function settingsTrigger() {
  return screen.getByRole("button", { name: "设置" });
}

function openSettings() {
  fireEvent.click(settingsTrigger());
}

/** 按分组取按钮：「标准」在段距与页宽里各有一个，必须按 role=group 限定。 */
function groupButton(group: string, name: string): HTMLElement {
  return within(screen.getByRole("group", { name: group })).getByRole("button", { name });
}

describe("阅读器：排版设置面板", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("默认值落到外壳变量上：19px / 1.9 行距 / 标准页宽 / 左对齐 / 2em 缩进", () => {
    renderReader();
    expect(shellVar("--reader-font")).toBe("19px");
    expect(shellVar("--reader-leading")).toBe("1.9");
    expect(shellVar("--reader-width")).toBe("42rem");
    expect(shellVar("--reader-align")).toBe("left");
    expect(shellVar("--reader-indent")).toBe("2em");
    expect(shellVar("--reader-para")).toBe("1em");
  });

  it("字体族三档：点「黑体」换成无衬线栈，并写进 localStorage", () => {
    renderReader();
    openSettings();

    fireEvent.click(groupButton("字体", "黑体"));
    expect(shellVar("--reader-family")).toBe("var(--reader-sans, sans-serif)");
    expect(localStorage.getItem(READER_KEYS.fontFamily)).toBe("sans");

    fireEvent.click(groupButton("字体", "衬线"));
    expect(shellVar("--reader-family")).toBe("var(--reader-serif, serif)");
    expect(localStorage.getItem(READER_KEYS.fontFamily)).toBe("serif");

    // 三档都在
    for (const label of ["衬线", "黑体", "系统"]) {
      expect(groupButton("字体", label)).toBeTruthy();
    }
  });

  it("段距四档：点「宽松」变 1.6em，点「紧凑」变 0.4em", () => {
    renderReader();
    openSettings();

    fireEvent.click(groupButton("段距", "宽松"));
    expect(shellVar("--reader-para")).toBe("1.6em");
    expect(localStorage.getItem(READER_KEYS.para)).toBe("1.6");

    fireEvent.click(groupButton("段距", "紧凑"));
    expect(shellVar("--reader-para")).toBe("0.4em");
  });

  it("页宽三档：窄 32rem / 标准 42rem / 宽 52rem", () => {
    renderReader();
    openSettings();

    fireEvent.click(groupButton("页宽", "窄"));
    expect(shellVar("--reader-width")).toBe("32rem");
    fireEvent.click(groupButton("页宽", "宽"));
    expect(shellVar("--reader-width")).toBe("52rem");
    fireEvent.click(groupButton("页宽", "标准"));
    expect(shellVar("--reader-width")).toBe("42rem");
  });

  it("对齐与缩进互不干扰：两端对齐时缩进仍可单独关掉", () => {
    renderReader();
    openSettings();

    fireEvent.click(groupButton("对齐", "两端"));
    expect(shellVar("--reader-align")).toBe("justify");
    expect(shellVar("--reader-indent")).toBe("2em");

    fireEvent.click(screen.getByRole("switch", { name: "首行缩进" }));
    expect(shellVar("--reader-align")).toBe("justify");
    expect(shellVar("--reader-indent")).toBe("0");
    expect(localStorage.getItem(READER_KEYS.indent)).toBe("false");
  });

  it("字号跟随系统：开启后字号改用 rem 折算（默认关闭保持 19px）", () => {
    renderReader();
    openSettings();

    expect(screen.getByRole("switch", { name: "跟随系统字号" }).getAttribute("aria-checked")).toBe("false");
    expect(shellVar("--reader-font")).toBe("19px");

    fireEvent.click(screen.getByRole("switch", { name: "跟随系统字号" }));
    expect(screen.getByRole("switch", { name: "跟随系统字号" }).getAttribute("aria-checked")).toBe("true");
    expect(shellVar("--reader-font")).toBe("1.1875rem");
    expect(localStorage.getItem(READER_KEYS.followSystemFont)).toBe("true");
  });

  it("主题四档：含「跟随系统」，点了交给 setTheme", () => {
    renderReader();
    openSettings();

    expect(groupButton("主题", "跟随系统")).toBeTruthy();
    fireEvent.click(groupButton("主题", "跟随系统"));
    expect(setTheme).toHaveBeenCalledWith("system");
  });

  it("繁体转简体：默认关闭显示原文，打开后正文变简体（段落下标不变）", () => {
    renderReader();
    expect(screen.getByText("燈光下的街道。")).toBeTruthy();

    openSettings();
    fireEvent.click(screen.getByRole("switch", { name: "繁体转简体" }));

    expect(screen.getByText("灯光下的街道。")).toBeTruthy();
    expect(screen.queryByText("燈光下的街道。")).toBeNull();
    expect(localStorage.getItem(READER_KEYS.script)).toBe("true");
    // 段落下标仍是原文那一套（书签 / 进度按它对齐）
    expect(document.getElementById("para-1")?.textContent).toBe("他说明天再来。");
  });

  it("设置面板是 dialog 且 Esc 关闭", () => {
    renderReader();
    openSettings();

    const panel = screen.getByRole("dialog", { name: "阅读设置" });
    expect(panel.getAttribute("aria-hidden")).toBe("false");

    fireEvent.keyDown(document, { key: "Escape" });
    expect(settingsTrigger().getAttribute("aria-expanded")).toBe("false");
  });
});
