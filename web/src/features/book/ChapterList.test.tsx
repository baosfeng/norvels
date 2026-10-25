import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { ChapterBrief } from "../../api/types";
import { ChapterList } from "./ChapterList";

/**
 * jsdom 里没有真实滚动容器高度，useVirtualizer 会一行都不渲染。
 * 这里把虚拟化换成「全部行都渲染」的桩：本用例验的是行内渲染（字数），不是窗口计算。
 */
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
