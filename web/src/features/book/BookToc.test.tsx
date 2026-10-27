import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { BookDetail, ChapterBrief } from "../../api/types";
import { initialShown, moreChaptersLabel, searchToc, TOC_CHUNK } from "./tocView";
import { BookToc } from "./BookToc";

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

function renderToc(book: BookDetail, columns = 1) {
  return render(
    <MemoryRouter>
      <BookToc book={book} columns={columns} dense onNavigate={() => {}} />
    </MemoryRouter>,
  );
}

describe("tocView 纯函数（C-11 / C-16 口径）", () => {
  it("搜索按章节名与章号命中，命中超过 60 条只先给 60 条", () => {
    const list = chapters(130);
    expect(searchToc(list, "雨停").total).toBe(1);
    expect(searchToc(list, "夜行").total).toBe(129);
    expect(searchToc(list, "夜行").chapters).toHaveLength(TOC_CHUNK);
    // 章号命中：标题一律不含数字时，只有第 3 章算命中
    const numbered = chapters(5).map((c) => ({ ...c, title: "夜行" }));
    expect(searchToc(numbered, "3").total).toBe(1);
  });

  it("卷内按钮文案随剩余数减少，铺满后不再给按钮", () => {
    expect(initialShown(130)).toBe(60);
    expect(moreChaptersLabel(70)).toBe("在本卷再显示 60 章（本卷还有 70 章）");
    expect(moreChaptersLabel(10)).toBe("在本卷再显示 10 章（本卷还有 10 章）");
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
    await waitFor(() => expect(document.activeElement).toBe(input));
    expect(screen.getByTestId("book-toc-count").textContent).toBe("130 章");
  });
});

describe("卷内再显示 60 章（C-16 新增）", () => {
  it("一次最多补 60 行，按钮文案剩余数同步减少，铺满后按钮消失", async () => {
    renderToc(bookOf([130]));
    const rows = () => document.querySelectorAll(".cv-row").length;
    expect(rows()).toBe(60);
    const more = screen.getByTestId("book-more-0");
    expect(more.textContent).toBe("在本卷再显示 60 章（本卷还有 70 章）");

    fireEvent.click(more);
    expect(rows()).toBe(120);
    expect(screen.getByTestId("book-more-0").textContent).toBe("在本卷再显示 10 章（本卷还有 10 章）");

    fireEvent.click(screen.getByTestId("book-more-0"));
    expect(rows()).toBe(130);
    expect(screen.queryByTestId("book-more-0")).toBeNull();
    // 按钮消失后焦点落到本卷新增的最后一行
    await waitFor(() => {
      const all = document.querySelectorAll<HTMLElement>(".cv-row");
      expect(document.activeElement).toBe(all[all.length - 1]);
      // 虚拟滚动下只查 DOM 会拿到视口内最后一行（曾落到第 96 章）；这里盯住「第 130 章」
      expect(document.activeElement?.textContent).toContain("第 130 章");
    });
  });

  it("本卷不足 60 章时按钮不渲染（照旧整卷铺开）", () => {
    renderToc(bookOf([2, 3]));
    expect(screen.queryByTestId("book-more-0")).toBeNull();
  });
});
