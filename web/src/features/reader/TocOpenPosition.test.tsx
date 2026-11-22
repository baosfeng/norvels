import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChapterBrief, Volume } from "../../api/types";
import { TocPanel } from "./TocPanel";
import { centerOffset, TOC_CENTER_TOLERANCE } from "./tocScroll";

/**
 * U11：打开目录时当前章必须在**第一帧**就在可视区中间。
 *
 * 改前是 setTimeout(0) + 再 setTimeout(20) + scrollIntoView：读者先看到列表停在第 1 章，
 * 隔一拍才跳到当前章。改后定位发生在布局阶段，渲染结束时的 scrollTop 就已经到位。
 *
 * jsdom 不做布局：这里给目录容器与每一行手工造出「真实浏览器会算出的数」
 * （行高 44、可视高 694），断言组件**同步渲染完成后**的 scrollTop，
 * 而不是「等定时器跑完才滚过去」。
 */

const ROW_H = 44;
const CLIENT_H = 694;

const chapter = (id: number, idx: number): ChapterBrief => ({
  id,
  idx,
  title: `第${idx + 1}章 示例`,
  volumeId: 1,
  wordCount: 1000,
});

const volume = (count: number): Volume => ({
  id: 1,
  idx: 0,
  title: "正文",
  chapters: Array.from({ length: count }, (_, i) => chapter(1000 + i, i)),
});

/** 装一套「浏览器布局」：行顶按 [data-toc-row] 的文档顺序排，容器可视高 694。 */
function installLayout() {
  const rows = () => Array.from(document.querySelectorAll<HTMLElement>("[data-toc-row]"));
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const at = rows().indexOf(this);
    const top = at < 0 ? 0 : at * ROW_H;
    const height = at < 0 ? CLIENT_H : ROW_H;
    return {
      top,
      bottom: top + height,
      left: 0,
      right: 0,
      width: 0,
      height,
      x: 0,
      y: top,
      toJSON: () => ({}),
    } as DOMRect;
  });
  Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.tocRow !== undefined ? ROW_H : 0;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.tocScroll !== undefined ? CLIENT_H : 0;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
    configurable: true,
    get(this: HTMLElement) {
      return this.dataset.tocScroll !== undefined ? rows().length * ROW_H : 0;
    },
  });
}

function renderToc(count: number, currentId: number) {
  installLayout();
  render(
    <MemoryRouter>
      <TocPanel
        active
        isH5={false}
        loading={false}
        error={null}
        volumes={[volume(count)]}
        bookId={12}
        currentChapterId={currentId}
        onNavigate={vi.fn()}
        onRetry={vi.fn()}
      />
    </MemoryRouter>,
  );
  const list = document.querySelector<HTMLElement>("[data-toc-scroll]");
  if (!list) throw new Error("缺少目录滚动容器 [data-toc-scroll]");
  return list;
}

/** 当前章行在内容里的行顶（下标 × 行高）。 */
function rowTopOf(index: number) {
  return index * ROW_H;
}

afterEach(() => {
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

describe("打开目录首帧定位（U11）", () => {
  it("当前章在第 1201 章：渲染完成时它已经在可视区中间（偏差 ≤ 40px）", () => {
    const index = 1200;
    const list = renderToc(1500, 1000 + index);
    const offset = centerOffset({
      rowTop: rowTopOf(index),
      rowHeight: ROW_H,
      clientHeight: CLIENT_H,
      scrollTop: list.scrollTop,
    });
    expect(list.scrollTop).toBeGreaterThan(0);
    expect(offset).toBeLessThanOrEqual(TOC_CENTER_TOLERANCE);
  });

  it("当前章在第一批 60 章之外：同一帧里补显 + 定位，不会先停在第 1 章", () => {
    const index = 60;
    const list = renderToc(1500, 1000 + index);
    expect(document.querySelector<HTMLElement>("[data-current-chapter='true']")).not.toBeNull();
    const offset = centerOffset({
      rowTop: rowTopOf(index),
      rowHeight: ROW_H,
      clientHeight: CLIENT_H,
      scrollTop: list.scrollTop,
    });
    expect(offset).toBeLessThanOrEqual(TOC_CENTER_TOLERANCE);
  });

  it("定位不依赖 scrollIntoView（改前就是靠它的异步一拍）", () => {
    const spy = vi.fn();
    HTMLElement.prototype.scrollIntoView = spy;
    const list = renderToc(1500, 1000 + 300);
    expect(list.scrollTop).toBeGreaterThan(0);
    // 打开目录这一下不该调用 scrollIntoView：位置在布局阶段就算好了
    expect(spy).not.toHaveBeenCalled();
  });

  it("当前章是首章：不滚（scrollTop 保持 0）", () => {
    const list = renderToc(1500, 1000);
    expect(list.scrollTop).toBe(0);
  });

  it("当前章不在目录里：不定位、不抛错", () => {
    const list = renderToc(1500, 999999);
    expect(list.scrollTop).toBe(0);
  });
});
