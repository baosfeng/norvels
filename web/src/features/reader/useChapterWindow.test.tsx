import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ApiError } from "../../api/client";
import type { ChapterBrief, ChapterDetail } from "../../api/types";

vi.mock("../../api/hooks", () => ({ useChapter: vi.fn() }));

const { useChapter } = await import("../../api/hooks");
const { useChapterWindow } = await import("./useChapterWindow");

const chapters: ChapterBrief[] = [1, 2, 3, 4, 5].map((id) => ({
  id,
  idx: id - 1,
  title: `第 ${id} 章`,
  volumeId: 0,
  wordCount: 1200,
}));

function detail(id: number): ChapterDetail {
  return {
    bookId: 7,
    bookTitle: "示例书",
    id,
    idx: id - 1,
    title: `第 ${id} 章`,
    content: "第一段\n\n第二段",
    wordCount: 1200,
    prevId: id - 1,
    nextId: id + 1,
  };
}

type Slot = { data: ChapterDetail | null; loading: boolean; error: ApiError | null };

function loaded(id: number): Slot {
  return { data: detail(id), loading: false, error: null };
}

/** 按章节 id 给假数据：同一个 useChapter 桩覆盖窗口里的所有槽位。 */
function mount(slots: Map<number, Slot>, activeId: number) {
  vi.mocked(useChapter).mockImplementation(((
    _bookId: number | null,
    chapterId: number | null,
  ) => {
    const slot = slots.get(chapterId ?? -1);
    return {
      data: slot?.data ?? null,
      loading: slot?.loading ?? false,
      error: slot?.error ?? null,
      reload: () => undefined,
    };
  }) as unknown as typeof useChapter);
  return renderHook(
    ({ active }: { active: number }) =>
      useChapterWindow({ bookId: 7, activeId: active, chapters, index: active - 1 }),
    { initialProps: { active: activeId } },
  );
}

describe("章节窗口：正文没到货时不把流清空", () => {
  it("可见章正文加载中：沿用上一批段落，流的高度不塌", () => {
    const slots = new Map<number, Slot>([
      [1, loaded(1)],
      [2, loaded(2)],
      [3, loaded(3)],
    ]);
    const { result, rerender } = mount(slots, 2);
    expect(result.current.segments.map((segment) => segment.id)).toEqual([1, 2, 3]);

    // 翻到第 4 章：它的正文还在路上（loading，无 data）
    slots.set(4, { data: null, loading: true, error: null });
    rerender({ active: 4 });
    expect(result.current.segments.map((segment) => segment.id)).toEqual([1, 2, 3]);
    expect(result.current.segments.length).toBeGreaterThan(0);
  });

  it("可见章真的打不开时交回空表，让页面走错误态并放行未落位的翻章", () => {
    const slots = new Map<number, Slot>([
      [1, loaded(1)],
      [2, loaded(2)],
      [3, loaded(3)],
    ]);
    const { result, rerender } = mount(slots, 2);
    expect(result.current.segments).toHaveLength(3);

    slots.set(4, { data: null, loading: false, error: new Error("boom") as unknown as ApiError });
    rerender({ active: 4 });
    expect(result.current.segments).toEqual([]);
    expect(result.current.error).toBeTruthy();
  });

  it("正文到货后照常换到新窗口，段落来自 cache", () => {
    const slots = new Map<number, Slot>([
      [2, loaded(2)],
      [3, loaded(3)],
    ]);
    const { result, rerender } = mount(slots, 3);
    expect(result.current.segments.map((segment) => segment.id)).toEqual([2, 3]);

    slots.set(4, loaded(4));
    rerender({ active: 4 });
    expect(result.current.segments.map((segment) => segment.id)).toEqual([3, 4]);
  });
});
