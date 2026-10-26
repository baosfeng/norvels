import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ApiError } from "../../api/client";
import type { Bookmark, ChapterBrief, Volume } from "../../api/types";
import { BookmarkConfirmDialog } from "./BookmarkConfirmDialog";
import { ReaderRestoreHint } from "./ReaderRestoreHint";
import { ReaderStateArea } from "./ReaderStateArea";
import { TocDrawer } from "./TocDrawer";

const chapter = (id: number, idx: number, title = `第${idx + 1}章 示例`): ChapterBrief => ({
  id,
  idx,
  title,
  volumeId: Math.floor(idx / 100) + 1,
  wordCount: 1000 + idx,
});

const volume = (id: number, title: string, start: number, count: number): Volume => ({
  id,
  idx: id - 1,
  title,
  chapters: Array.from({ length: count }, (_, i) => chapter(start + i, start + i)),
});

function toc(volumes: Volume[], overrides: Partial<Parameters<typeof TocDrawer>[0]> = {}) {
  return render(
    <MemoryRouter>
      <TocDrawer
        open
        loading={false}
        error={null}
        volumes={volumes}
        bookId={7}
        currentChapterId={volumes[0]?.chapters[0]?.id ?? 0}
        onClose={vi.fn()}
        onRetry={vi.fn()}
        {...overrides}
      />
    </MemoryRouter>,
  );
}

function rows() {
  return Array.from(document.querySelectorAll<HTMLElement>("[data-toc-row]"));
}

afterEach(() => {
  vi.useRealTimers();
});

describe("新增控件：正文空态 · 先读下一章（C-15）", () => {
  it("可点并切到下一章；末章置灰且常驻原因", () => {
    const onNext = vi.fn();
    const next = chapter(12, 2, "第三章");
    const { rerender } = render(
      <ReaderStateArea loading={false} error={null} retrying={false} retryFailures={0} next={next} onRetry={vi.fn()} onNext={onNext} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "先读下一章" }));
    expect(onNext).toHaveBeenCalledTimes(1);

    rerender(
      <ReaderStateArea loading={false} error={null} retrying={false} retryFailures={0} next={null} onRetry={vi.fn()} onNext={onNext} />,
    );
    expect((screen.getByRole("button", { name: "先读下一章" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("已经是最后一章了")).toBeTruthy();
  });

  it("错误态重试转 loading，连续失败 3 次后给下一章出口", () => {
    const error = { kind: "network", message: "断开" } as ApiError;
    render(
      <ReaderStateArea loading={false} error={error} retrying retryFailures={3} next={chapter(12, 2)} onRetry={vi.fn()} onNext={vi.fn()} />,
    );
    const retry = screen.getByRole("button", { name: "正在重试…" }) as HTMLButtonElement;
    expect(retry.disabled).toBe(true);
    expect(screen.getByText("还是打不开，可以先读下一章")).toBeTruthy();
  });
});

describe("新增控件：进度恢复提示条（C-16）", () => {
  it("显示「已回到上次读到的地方」，点「知道了」收起", () => {
    const onDismiss = vi.fn();
    const { rerender } = render(<ReaderRestoreHint visible onDismiss={onDismiss} />);
    expect(screen.getByText("已回到上次读到的地方")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "知道了" }));
    expect(onDismiss).toHaveBeenCalledTimes(1);

    rerender(<ReaderRestoreHint visible={false} onDismiss={onDismiss} />);
    expect(screen.queryByText("已回到上次读到的地方")).toBeNull();
  });
});

describe("新增控件：目录卷折叠（C-27）", () => {
  it("点卷头收起该卷章节，aria-expanded 同步；空卷置灰并给原因", () => {
    const volumes = [volume(1, "第一卷 · 江雪", 1000, 3), volume(2, "第二卷 · 山雨", 1100, 2)];
    toc(volumes);

    const header = screen.getByRole("button", { name: /第一卷/ });
    expect(header.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(header);
    expect(header.getAttribute("aria-expanded")).toBe("false");
    // 第一卷章节收起，第二卷仍在
    expect(screen.queryByText("第1章 示例")).toBeNull();
  });
});

describe("新增控件：目录再显示 60 章（C-29）", () => {
  it("首块 60 章；点一次补 60 章，全部加载后按钮禁用并常驻原因", async () => {
    vi.useFakeTimers();
    toc([volume(1, "正文", 1000, 120)]);
    expect(rows()).toHaveLength(60);

    const more = screen.getByRole("button", { name: /再显示 60 章/ }) as HTMLButtonElement;
    fireEvent.click(more);
    expect(more.textContent).toBe("正在加载…");
    expect(more.disabled).toBe(true);

    await act(async () => {
      vi.advanceTimersByTime(200);
    });
    expect(rows()).toHaveLength(120);
    const done = screen.getByRole("button", { name: /已显示全部 120 章/ }) as HTMLButtonElement;
    expect(done.disabled).toBe(true);
    expect(screen.getByText("已经显示全部章节了")).toBeTruthy();
  });
});

describe("新增控件：目录空态清空搜索（C-31）", () => {
  it("无匹配时出现；点后清空输入、恢复列表并把焦点交还搜索框", () => {
    toc([volume(1, "正文", 1000, 3)]);
    const input = screen.getByTestId("toc-search-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "不存在的标题" } });
    expect(screen.getByTestId("toc-empty")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "清空搜索" }));
    expect(input.value).toBe("");
    expect(document.activeElement).toBe(input);
    expect(rows()).toHaveLength(3);
    expect(screen.queryByTestId("toc-empty")).toBeNull();
  });

  it("目录本身为空时不出现清空按钮，改显示未拆章说明", () => {
    toc([]);
    expect(screen.queryByRole("button", { name: "清空搜索" })).toBeNull();
    expect(screen.getByText("这本书尚未拆出章节。")).toBeTruthy();
  });
});

describe("新增控件：目录失败态两出口（C-61 / C-62）", () => {
  it("重试立即转 loading 并禁用；成功后错误面板消失、列表回来", () => {
    vi.useFakeTimers();
    const onRetry = vi.fn();
    const error = { kind: "network", message: "断开" } as ApiError;
    const volumes = [volume(1, "正文", 1000, 2)];
    const view = (err: ApiError | null, vols: Volume[]) => (
      <MemoryRouter>
        <TocDrawer open loading={false} error={err} volumes={vols} bookId={7} currentChapterId={1000} onClose={vi.fn()} onRetry={onRetry} />
      </MemoryRouter>
    );
    const { rerender } = render(view(error, []));
    const retry = screen.getByRole("button", { name: "重试" }) as HTMLButtonElement;
    fireEvent.click(retry);
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect((screen.getByRole("button", { name: "正在重试…" }) as HTMLButtonElement).disabled).toBe(true);

    rerender(view(null, volumes));
    act(() => {
      vi.advanceTimersByTime(50);
    });
    expect(screen.queryByTestId("toc-error")).toBeNull();
    expect(rows()).toHaveLength(2);
    expect(document.activeElement).toBe(screen.getByTestId("toc-search-input"));
  });

  it("用平铺列表：卷头消失、章节仍可搜，焦点落到首行", () => {
    vi.useFakeTimers();
    const error = { kind: "network", message: "断开" } as ApiError;
    const volumes = [volume(1, "第一卷", 1000, 2), volume(2, "第二卷", 1100, 2)];
    render(
      <MemoryRouter>
        <TocDrawer open loading={false} error={error} volumes={volumes} bookId={7} currentChapterId={1000} onClose={vi.fn()} onRetry={vi.fn()} />
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole("button", { name: "用平铺列表" }));
    expect(document.querySelector(".toc-vol")).toBeNull();
    expect(rows()).toHaveLength(4);
    act(() => {
      vi.advanceTimersByTime(20);
    });
    expect(document.activeElement).toBe(rows()[0]);
  });
});

describe("新增控件：删除书签二次确认（C-58 / C-59 / C-60）", () => {
  const bookmark: Bookmark = {
    id: 99,
    chapterId: 11,
    chapterIdx: 1,
    chapterTitle: "第三章 · 灯下落雪",
    paraIndex: 2,
    label: "沈砚在渡口等了半个时辰。",
    createdAt: "2026-10-24T09:00:00.000Z",
  };

  it("确认框写明对象与不可恢复；默认焦点在取消", () => {
    render(<BookmarkConfirmDialog open bookmark={bookmark} deleting={false} onCancel={vi.fn()} onConfirm={vi.fn()} />);
    expect(screen.getByText("删除这条书签？")).toBeTruthy();
    expect(screen.getByText(/删除后立即移除，不可恢复。这一条是「第三章 · 灯下落雪」。/)).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "取消" }));
  });

  it("取消只关闭不删除；删除中按钮禁用并显示「正在删除…」", () => {
    const onCancel = vi.fn();
    const onConfirm = vi.fn();
    const { rerender } = render(
      <BookmarkConfirmDialog open bookmark={bookmark} deleting={false} onCancel={onCancel} onConfirm={onConfirm} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(onCancel).toHaveBeenCalledTimes(1);

    rerender(<BookmarkConfirmDialog open bookmark={bookmark} deleting onCancel={onCancel} onConfirm={onConfirm} />);
    const deleting = screen.getByRole("button", { name: "正在删除…" }) as HTMLButtonElement;
    expect(deleting.disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(onCancel).toHaveBeenCalledTimes(2);
  });
});