import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { ChapterBrief } from "../../api/types";
import { ChapterList } from "./ChapterList";

/**
 * jsdom 里没有真实滚动容器高度，useVirtualizer 会一行都不渲染。
 * 这里把虚拟化换成「全部行都渲染」的桩：本用例验的是行内渲染（字数），不是窗口计算。
 */
const scrollToIndexSpy = vi.hoisted(() => vi.fn());

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
      scrollToIndex: scrollToIndexSpy,
    };
  },
}));

const chapters: ChapterBrief[] = [
  { id: 10, idx: 0, title: "第一章 开端", volumeId: 1, wordCount: 1234 },
  { id: 11, idx: 1, title: "第二章 承接", volumeId: 1, wordCount: 6177 },
];

describe("目录：每章显示字数", () => {
  it("目录行右侧给出千分位字数，数据来自 chapters 响应里已有的 wordCount", () => {
    render(
      <MemoryRouter>
        <ChapterList bookId={7} chapters={chapters} ariaLabel="章节目录" />
      </MemoryRouter>,
    );

    const words = screen.getAllByTestId("chapter-word").map((node) => node.textContent?.trim());
    expect(words).toContain("1,234 字");
    expect(words).toContain("6,177 字");
    expect(screen.getByText("第一章 开端")).toBeTruthy();
  });
});
const LONG_TITLE =
  "第一章 长夜将尽时他站在巷口看着第九次告别与最后一盏灯下那封没读完的信以及窗台上渐渐凉透的茶和第二天清晨仍未寄出的回信";

describe("目录：长标题 + 4 位字数的两列排版", () => {
  function renderList(layout?: "detail" | "drawer") {
    return render(
      <MemoryRouter>
        <ChapterList
          bookId={7}
          chapters={[
            { id: 10, idx: 0, title: LONG_TITLE, volumeId: 1, wordCount: 5065 },
            { id: 11, idx: 1, title: "第二章 短标题", volumeId: 1, wordCount: 5818 },
          ]}
          ariaLabel="章节目录"
          layout={layout}
        />
      </MemoryRouter>,
    );
  }

  it("标题 60 字、字数 5,065：行是标题区 + 固定宽度字数区，字数不换行", () => {
    renderList();

    const rows = screen.getAllByTestId("chapter-word").map((node) => node.parentElement as HTMLElement);
    expect(rows).toHaveLength(2);
    for (const row of rows) {
      expect(row.className).toContain("cv-row");
      // 详情目录单行省略，行高恒定 44px（与 H5 触屏 44px 底子对齐，虚拟滚动步进才不叠行）
      expect(row.style.height).toBe("44px");
      const title = row.querySelector(".cv-title") as HTMLElement;
      const word = row.querySelector(".cv-word") as HTMLElement;
      expect(title.textContent).toBeTruthy();
      expect(word.textContent?.trim()).toMatch(/^\d{1,3}(,\d{3})* 字$/);
    }
    expect(screen.getAllByTestId("chapter-word").map((n) => n.textContent?.trim())).toEqual([
      "5,065 字",
      "5,818 字",
    ]);
  });

  it("抽屉里最多两行，行高抬到 44px，仍不把字数挤到第二行", () => {
    renderList("drawer");

    const row = screen.getAllByTestId("chapter-word")[0].parentElement as HTMLElement;
    expect(row.className).toContain("is-drawer");
    expect(row.style.height).toBe("44px");
  });

  it("样式契约：标题 1fr + min-width:0 省略，字数固定宽度、右对齐、tabular-nums、间距 12px", async () => {
    const fs = (await import("node:fs" as string)) as { readFileSync: (file: string, encoding: string) => string };
    const css = fs.readFileSync("src/styles/prototype.css", "utf8");

    expect(css).toContain(".cv-row{display:flex;align-items:center;gap:12px");
    expect(css).toContain(".cv-row .cv-title{flex:1 1 auto;min-width:0");
    expect(css).toContain("text-overflow:ellipsis;white-space:nowrap");
    expect(css).toContain(".cv-row .cv-word{flex:0 0 60px;width:60px;text-align:right;white-space:nowrap");
    expect(css).toContain("font-variant-numeric:tabular-nums");
    expect(css).toContain(".cv-row.is-drawer .cv-title{white-space:normal;display:-webkit-box;-webkit-line-clamp:2");
    // 旧原型选择器（DOM 里已不存在）不残留
    expect(css).not.toContain(".chapter-win");
  });
});

describe("目录：多列与撑满", () => {
  const many: ChapterBrief[] = Array.from({ length: 7 }, (_, i) => ({
    id: 100 + i,
    idx: i,
    title: `第${i + 1}章 标题`,
    volumeId: 1,
    wordCount: 1000 + i,
  }));

  it("columns=3 时一行放 3 章，总行数 = ceil(章数 / 3)", () => {
    render(
      <MemoryRouter>
        <ChapterList bookId={7} chapters={many} columns={3} ariaLabel="章节目录" />
      </MemoryRouter>,
    );
    const scroller = screen.getByTestId("chapter-scroll");
    expect(scroller.getAttribute("data-columns")).toBe("3");
    const rows = scroller.querySelectorAll(".cv-grid");
    expect(rows).toHaveLength(3); // 7 章 = 3 + 3 + 1
    expect(rows[0].querySelectorAll(".cv-row")).toHaveLength(3);
    expect(rows[2].querySelectorAll(".cv-row")).toHaveLength(1);
  });

  it("fill 撑满父容器，不再由组件自己限制高度", () => {
    render(
      <MemoryRouter>
        <ChapterList bookId={7} chapters={many} fill maxHeight={360} ariaLabel="章节目录" />
      </MemoryRouter>,
    );
    const scroller = screen.getByTestId("chapter-scroll");
    expect(scroller.className).toContain("is-fill");
    expect(scroller.style.maxHeight).toBe("");
  });

  it("scrollToCurrent 会把当前章所在行滚到中间", () => {
    scrollToIndexSpy.mockReset();
    render(
      <MemoryRouter>
        <ChapterList bookId={7} chapters={many} columns={3} currentChapterId={104} scrollToCurrent ariaLabel="章节目录" />
      </MemoryRouter>,
    );
    // 第 5 章（下标 4）在 3 列布局里属于第 1 行（0 起）
    expect(scrollToIndexSpy).toHaveBeenCalledWith(1, { align: "center" });
  });

  it("行数超过 800 时退回 70vh 限高（否则虚拟化不裁剪，几千章会全渲染）", () => {
    const huge: ChapterBrief[] = Array.from({ length: 900 }, (_, i) => ({
      id: 1000 + i,
      idx: i,
      title: `第${i + 1}章 标题`,
      volumeId: 1,
      wordCount: 1000,
    }));
    render(
      <MemoryRouter>
        <ChapterList bookId={7} chapters={huge} ariaLabel="章节目录" />
      </MemoryRouter>,
    );
    expect(screen.getByTestId("chapter-scroll").style.maxHeight).toBe("70vh");
  });

  it("dense 用 32px 行高（PC 鼠标密度）", () => {
    render(
      <MemoryRouter>
        <ChapterList bookId={7} chapters={many} dense ariaLabel="章节目录" />
      </MemoryRouter>,
    );
    const row = screen.getAllByTestId("chapter-word")[0].parentElement as HTMLElement;
    expect(row.style.height).toBe("32px");
  });
});
