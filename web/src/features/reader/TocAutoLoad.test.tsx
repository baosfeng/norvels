import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChapterBrief, Volume } from "../../api/types";
import { TocPanel } from "./TocPanel";

/** 滚到底自动续载（task-2）：目录分块渲染，滑到接近底部自动补下一批，不用点「再显示」。 */

const chapter = (id: number, idx: number): ChapterBrief => ({
  id,
  idx,
  title: `第${idx + 1}章 示例`,
  volumeId: 1,
  wordCount: 1000 + idx,
});

const singleVolume = (count: number): Volume => ({
  id: 1,
  idx: 0,
  title: "正文",
  chapters: Array.from({ length: count }, (_, i) => chapter(1000 + i, i)),
});

function mount(count = 150) {
  return render(
    <MemoryRouter>
      <TocPanel
        active
        isH5={false}
        loading={false}
        error={null}
        volumes={[singleVolume(count)]}
        bookId={12}
        currentChapterId={1000}
        onNavigate={vi.fn()}
        onRetry={vi.fn()}
      />
    </MemoryRouter>,
  );
}

function rows(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>("[data-toc-row]"));
}

function scroller(): HTMLElement {
  const el = document.querySelector<HTMLElement>("[data-toc-scroll]");
  if (!el) throw new Error("缺少目录滚动容器 [data-toc-scroll]");
  return el;
}

/** jsdom 不做排版：手动给出滚动量，模拟真实浏览器里布局算出来的三个数。 */
function mockScroll(el: HTMLElement, m: { scrollTop: number; clientHeight: number; scrollHeight: number }) {
  Object.defineProperty(el, "scrollTop", { value: m.scrollTop, configurable: true, writable: true });
  Object.defineProperty(el, "clientHeight", { value: m.clientHeight, configurable: true });
  Object.defineProperty(el, "scrollHeight", { value: m.scrollHeight, configurable: true });
}

/** 打开目录的定位 effect 用的是 0 / 20ms 定时器，先让它们走完再断言。 */
async function settleOpen() {
  await act(async () => {
    vi.advanceTimersByTime(60);
  });
}

async function scrollTo(el: HTMLElement, m: { scrollTop: number; clientHeight: number; scrollHeight: number }) {
  mockScroll(el, m);
  fireEvent.scroll(el);
  await act(async () => {
    vi.advanceTimersByTime(200);
  });
}

afterEach(() => {
  vi.useRealTimers();
});

describe("目录滚到底自动续载（task-2）", () => {
  it("滑到接近底部就自动补 60 章，不需要点按钮", async () => {
    vi.useFakeTimers();
    mount(150);
    await settleOpen();
    expect(rows()).toHaveLength(60);

    // 60 章 ≈ 2780px，抽屉可视 694px：滚到距底部 240px 以内
    await scrollTo(scroller(), { scrollTop: 2086, clientHeight: 694, scrollHeight: 2780 });
    expect(rows()).toHaveLength(120);
  });

  it("连续下滑能一路到第 150 章，末章可见且按钮转「已显示全部」", async () => {
    vi.useFakeTimers();
    mount(150);
    await settleOpen();

    const el = scroller();
    await scrollTo(el, { scrollTop: 2086, clientHeight: 694, scrollHeight: 2780 });
    expect(rows()).toHaveLength(120);

    // 补完之后内容变高，继续滑到底 → 再补最后 30 章
    await scrollTo(el, { scrollTop: 4846, clientHeight: 694, scrollHeight: 5540 });
    expect(rows()).toHaveLength(150);
    expect(screen.getByText("第150章 示例")).toBeTruthy();

    const done = screen.getByRole("button", { name: "已显示全部 150 章" }) as HTMLButtonElement;
    expect(done.disabled).toBe(true);
    expect(screen.getByText("已经显示全部章节了")).toBeTruthy();
  });

  it("离底部还远时不自动补，避免一边浏览一边把 150 章全铺出来", async () => {
    vi.useFakeTimers();
    mount(150);
    await settleOpen();

    await scrollTo(scroller(), { scrollTop: 300, clientHeight: 694, scrollHeight: 2780 });
    expect(rows()).toHaveLength(60);
  });

  it("自动续载时按钮回退仍在，提示文案说明不用点", async () => {
    vi.useFakeTimers();
    mount(150);
    await settleOpen();

    expect(screen.getByRole("button", { name: /再显示 60 章/ })).toBeTruthy();
    expect(screen.getByTestId("toc-auto-hint").textContent).toBe("继续下滑，自动加载下一批");
  });
});
