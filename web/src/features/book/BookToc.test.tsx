import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterBrief } from "../../api/types";
import { BookToc } from "./BookToc";
import { firstBatch, searchToc, TOC_CHUNK, TOC_FIRST_BATCH } from "./tocView";

/** jsdom 没有滚动容器高度：把虚拟化换成「全部行都渲染」的桩。 */
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

function chapters(count: number, volumeId = 1): ChapterBrief[] {
  return Array.from({ length: count }, (_, i) => ({
    id: 100 + i,
    idx: i,
    title: i === 0 ? "第一章 雨停之前" : `第 ${i + 1} 章 夜行`,
    volumeId,
    wordCount: 2000,
  }));
}

function bookOf(volumeChapters: number[]): BookDetail {
  return {
    id: 7,
    title: "示例长篇",
    authorId: 1,
    authorName: "示例作者",
    authors: ["示例作者"],
    chapterCount: volumeChapters.reduce((a, b) => a + b, 0),
    wordCount: 1000,
    volumeCount: volumeChapters.length,
    intro: "",
    status: "parsed",
    importedAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    volumes: volumeChapters.map((count, idx) => ({
      id: idx + 1,
      idx,
      title: `第 ${idx + 1} 卷`,
      chapters: chapters(count, idx + 1).map((c) => ({ ...c, id: c.id + idx * 1000, idx: c.idx })),
    })),
  };
}

function renderToc(
  book: BookDetail,
  options: { columns?: number; isH5?: boolean; resumeChapterId?: number; onNavigate?: (chapter: ChapterBrief) => void } = {},
) {
  const { columns = 1, isH5 = false, resumeChapterId, onNavigate = () => {} } = options;
  return render(
    <MemoryRouter>
      <BookToc
        book={book}
        columns={columns}
        dense
        isH5={isH5}
        resumeChapterId={resumeChapterId}
        onNavigate={onNavigate}
      />
    </MemoryRouter>,
  );
}

/** 目录里的章行：虚拟化被换成全渲染，所以 DOM 行数 = 已铺出来的章数。 */
const rowCount = () => document.querySelectorAll(".cv-row").length;
const moreButtonCount = () => document.querySelectorAll('[data-testid^="book-more-"]').length;

/** 卷内列表底在页面里的位置（px）：测试用它模拟「滚到底」与「补完长高」。 */
const listBottoms = new Map<string, number>();
/** jsdom 的 window.innerHeight。 */
const VIEWPORT = 768;

function setListBottom(bottom: number, volumeIdx = 0) {
  listBottoms.set(String(volumeIdx), bottom);
}

beforeEach(() => {
  listBottoms.clear();
  // 默认：列表底远在视口下方（没滚到底）
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const volumeIdx = this.closest("[data-volume]")?.getAttribute("data-volume") ?? "0";
    const bottom = listBottoms.get(volumeIdx) ?? 4000;
    return {
      top: 0,
      bottom,
      left: 0,
      right: 0,
      width: 100,
      height: bottom,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect;
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("tocView 纯函数（搜索口径 + H5 首屏批次）", () => {
  it("搜索按章节名与章号命中，命中超过 60 条只先给 60 条", () => {
    const list = chapters(130);
    expect(searchToc(list, "雨停").total).toBe(1);
    expect(searchToc(list, "夜行").total).toBe(129);
    expect(searchToc(list, "夜行").chapters).toHaveLength(TOC_CHUNK);
    // 章号命中：标题一律不含数字时，只有第 3 章算命中
    const numbered = chapters(5).map((c) => ({ ...c, title: "夜行" }));
    expect(searchToc(numbered, "3").total).toBe(1);
  });

  it("H5 首屏批次远小于每次续载的批量，章节不足时全给", () => {
    expect(TOC_FIRST_BATCH).toBeLessThan(TOC_CHUNK);
    expect(firstBatch(150)).toBe(TOC_FIRST_BATCH);
    expect(firstBatch(3)).toBe(3);
    expect(firstBatch(0)).toBe(0);
  });
});

describe("目录搜索（C-11 新增）", () => {
  it("输入即筛：只留命中章节，计数变「n 章匹配」，并说明找到几章", () => {
    renderToc(bookOf([130]));
    expect(screen.getByTestId("book-toc-count").textContent).toBe("130 章");
    fireEvent.change(screen.getByTestId("book-toc-search"), { target: { value: "雨停" } });
    expect(screen.getByTestId("book-toc-count").textContent).toBe("1 章匹配");
    expect(screen.getByTestId("book-toc-summary").textContent).toBe("找到 1 章");
    expect(screen.getAllByTestId("chapter-word")).toHaveLength(1);
  });

  it("Esc 清空关键词并恢复全量目录，焦点留在搜索框", () => {
    renderToc(bookOf([130]));
    const input = screen.getByTestId("book-toc-search") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "雨停" } });
    expect(screen.queryByTestId("book-toc-results")).toBeTruthy();
    fireEvent.keyDown(input, { key: "Escape" });
    expect(input.value).toBe("");
    expect(screen.queryByTestId("book-toc-results")).toBeNull();
    expect(screen.getByTestId("book-toc-count").textContent).toBe("130 章");
    expect(document.activeElement).toBe(input);
  });

  it("Enter 不提交、不跳转（只防止表单行为）", () => {
    renderToc(bookOf([130]));
    const input = screen.getByTestId("book-toc-search") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "夜行" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByTestId("book-toc-count").textContent).toBe("129 章匹配");
  });

  it("命中 0 条给空态与「清空搜索」，点完回到全量目录且焦点回搜索框（C-12）", async () => {
    renderToc(bookOf([130]));
    const input = screen.getByTestId("book-toc-search") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "不存在的词" } });
    const empty = screen.getByTestId("book-toc-no-match");
    expect(empty.textContent).toContain("没有找到章节");
    fireEvent.click(screen.getByRole("button", { name: "清空搜索" }));
    expect(screen.queryByTestId("book-toc-no-match")).toBeNull();
    expect(input.value).toBe("");
    expect(document.activeElement).toBe(input);
    expect(screen.getByTestId("book-toc-count").textContent).toBe("130 章");
  });
});

describe("目录：PC 全部展开（不懒加载）", () => {
  it("单卷 150 章一次铺全，页面上没有「查看全部」「再显示」按钮", () => {
    renderToc(bookOf([150]));
    expect(rowCount()).toBe(150);
    expect(screen.queryByTestId("book-toc-expand")).toBeNull();
    expect(moreButtonCount()).toBe(0);
    // PC 不需要「继续下滑」的提示：本来就没有隐藏的章节
    expect(screen.queryByTestId("book-toc-autoload-hint")).toBeNull();
  });

  it("多卷：卷头照旧可折叠，展开的那一卷同样一次铺全", () => {
    renderToc(bookOf([130, 70]), { columns: 3 });
    expect(rowCount()).toBe(130);
    const heads = document.querySelectorAll(".vol-head");
    expect(heads).toHaveLength(2);
    fireEvent.click(heads[1]);
    expect(rowCount()).toBe(200);
    expect(moreButtonCount()).toBe(0);
  });

  it("倒序仍可用：切到倒序后末章排到最前", () => {
    renderToc(bookOf([5]));
    expect(document.querySelector(".cv-row")?.textContent).toContain("雨停之前");
    fireEvent.click(screen.getByTestId("book-toc-order"));
    expect(document.querySelector(".cv-row")?.textContent).toContain("第 5 章");
  });

  it("续读定位条是指向那一章的链接，点击仍触发清角标回调", () => {
    const onNavigate = vi.fn();
    renderToc(bookOf([5]), { resumeChapterId: 104, onNavigate });
    const resume = screen.getByTestId("book-toc-resume") as HTMLAnchorElement;
    expect(resume.textContent).toContain("第 5 章");
    // 跳章是导航：按钮只挂回调、不会离开详情页（实测点击后 URL 仍停在 /book/7）
    expect(resume.getAttribute("href")).toBe("/read/7/104");
    fireEvent.click(resume);
    expect(onNavigate).toHaveBeenCalledWith(expect.objectContaining({ id: 104 }));
  });
});

describe("目录：H5 滚到底自动续载", () => {
  it("首屏只铺一小批（远少于 60 章），没有「查看全部」「再显示」按钮", () => {
    renderToc(bookOf([150]), { isH5: true });
    expect(rowCount()).toBe(TOC_FIRST_BATCH);
    expect(rowCount()).toBeLessThan(TOC_CHUNK);
    expect(screen.queryByTestId("book-toc-expand")).toBeNull();
    expect(moreButtonCount()).toBe(0);
    expect(screen.getByTestId("book-toc-autoload-hint")).toBeTruthy();
  });

  it("没滚到底不补；滚到列表底自动补一批（不用点任何按钮）", () => {
    renderToc(bookOf([150]), { isH5: true });
    setListBottom(4000);
    fireEvent.scroll(window);
    expect(rowCount()).toBe(TOC_FIRST_BATCH);
    setListBottom(VIEWPORT - 40);
    fireEvent.scroll(window);
    expect(rowCount()).toBe(TOC_FIRST_BATCH + TOC_CHUNK);
  });

  it("同一手势连发滚动事件只补一批（一次性守卫）", () => {
    renderToc(bookOf([300]), { isH5: true });
    setListBottom(VIEWPORT - 40);
    fireEvent.scroll(window);
    fireEvent.scroll(window);
    fireEvent.scroll(window);
    expect(rowCount()).toBe(TOC_FIRST_BATCH + TOC_CHUNK);
  });

  it("连续下滑一直补到最后一章；铺完后提示消失、不留哨兵", () => {
    renderToc(bookOf([150]), { isH5: true });
    // 20 → 80 → 140 → 150：每批补完列表长高，下一次触底再补下一批
    for (const expected of [80, 140, 150]) {
      setListBottom(4000); // 长高后离底还远：重新上膛
      fireEvent.scroll(window);
      setListBottom(VIEWPORT - 40); // 再次触底
      fireEvent.scroll(window);
      expect(rowCount()).toBe(expected);
    }
    expect(screen.queryByTestId("book-toc-autoload-hint")).toBeNull();
    expect(screen.queryByTestId("book-toc-expand")).toBeNull();
    expect(moreButtonCount()).toBe(0);
  });

  it("PC 看全后切到 H5 档位：不把已经铺出来的章节又藏回去", () => {
    const book = bookOf([150]);
    const ui = (isH5: boolean) => (
      <MemoryRouter>
        <BookToc book={book} columns={1} dense isH5={isH5} onNavigate={() => {}} />
      </MemoryRouter>
    );
    const { rerender } = render(ui(false));
    expect(rowCount()).toBe(150);
    rerender(ui(true));
    expect(rowCount()).toBe(150);
    expect(screen.queryByTestId("book-toc-autoload-hint")).toBeNull();
  });

  it("多卷：滚到哪一卷的底就续哪一卷，折叠的卷不参与", () => {
    renderToc(bookOf([100, 100]), { isH5: true });
    expect(rowCount()).toBe(TOC_FIRST_BATCH);
    fireEvent.click(document.querySelectorAll(".vol-head")[1]);
    expect(rowCount()).toBe(TOC_FIRST_BATCH * 2);
    setListBottom(4000, 0); // 第一卷离底还远
    setListBottom(VIEWPORT - 40, 1); // 第二卷触底
    fireEvent.scroll(window);
    expect(rowCount()).toBe(TOC_FIRST_BATCH * 2 + TOC_CHUNK);
  });

  it("搜索态不受影响：命中直接列出（最多 60 条），滚动不触发续载", () => {
    renderToc(bookOf([150]), { isH5: true });
    fireEvent.change(screen.getByTestId("book-toc-search"), { target: { value: "夜行" } });
    expect(rowCount()).toBe(TOC_CHUNK);
    expect(screen.queryByTestId("book-toc-autoload-hint")).toBeNull();
    setListBottom(VIEWPORT - 40);
    fireEvent.scroll(window);
    expect(rowCount()).toBe(TOC_CHUNK);
  });
});
