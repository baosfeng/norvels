import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { showToast } = vi.hoisted(() => ({ showToast: vi.fn() }));
vi.mock("../../lib/toastStore", () => ({ showToast }));

const { useReaderProgressSync } = await import("./useReaderProgressSync");

const chapters = [
  { id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 },
  { id: 11, idx: 1, title: "第二章", volumeId: 1, wordCount: 2100 },
];

function renderSync(saveProgress: () => Promise<boolean>) {
  return renderHook(
    (props: { anchor: { chapterId: number; para: number } }) =>
      useReaderProgressSync({
        bookId: 7,
        chapterId: 10,
        chapters,
        index: 0,
        chapterDataId: 10,
        storedParaIndex: 0,
        anchor: props.anchor,
        saveProgress,
      }),
    { initialProps: { anchor: { chapterId: 10, para: 0 } } },
  );
}

beforeEach(() => {
  showToast.mockReset();
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-31T10:00:00Z"));
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

/**
 * 口径：进度写失败不能静默（读者以为存上了），也不能每章弹一次（后端正挂着时刷屏）。
 * 一次 error toast + 60 秒节流；窗口过了再失败会再提示。
 */
describe("阅读进度写入失败提示", () => {
  it("失败提示一次；60 秒内的后续失败不再弹；过了窗口再弹", async () => {
    const saveProgress = vi.fn(async () => false);
    const { rerender } = renderSync(saveProgress);

    // 挂载即上报一次：失败 → 弹一次
    await act(async () => {});
    expect(showToast).toHaveBeenCalledTimes(1);
    expect(showToast.mock.calls[0][1]).toBe("error");
    expect(String(showToast.mock.calls[0][0])).toContain("阅读进度没能保存");

    // 换段落触发第二次上报（仍在 60 秒窗口内）：不重复弹
    rerender({ anchor: { chapterId: 10, para: 5 } });
    await act(async () => {
      vi.advanceTimersByTime(1100);
    });
    expect(saveProgress).toHaveBeenCalledTimes(2);
    expect(showToast).toHaveBeenCalledTimes(1);
  });

  it("过了节流窗口再失败会再提示一次", async () => {
    const saveProgress = vi.fn(async () => false);
    const { rerender } = renderSync(saveProgress);
    await act(async () => {});
    expect(showToast).toHaveBeenCalledTimes(1);

    vi.setSystemTime(new Date("2026-10-31T10:02:00Z"));
    rerender({ anchor: { chapterId: 10, para: 9 } });
    await act(async () => {
      vi.advanceTimersByTime(1100);
    });
    expect(showToast).toHaveBeenCalledTimes(2);
  });

  it("写入成功不弹提示，并广播一次进度失效", async () => {
    const { readingRevision } = await import("../../lib/readingRevision");
    const before = readingRevision();
    const saveProgress = vi.fn(async () => true);
    renderSync(saveProgress);
    await act(async () => {});
    expect(showToast).not.toHaveBeenCalled();
    expect(readingRevision()).toBe(before + 1);
  });
});
