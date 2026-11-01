import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const track = vi.hoisted(() => vi.fn());
vi.mock("../../lib/telemetry", () => ({
  track,
  TELEMETRY_FINALE_EVENT: "norvels:telemetry-finale",
}));

const { useBookView } = await import("./useBookView");
const FINALE = "norvels:telemetry-finale";

function viewCalls() {
  return track.mock.calls.filter(([kind]) => kind === "view");
}

beforeEach(() => {
  track.mockReset();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("详情页停留埋点", () => {
  it("进页面不发明 start（不在后端 kind 白名单里，整批会 400）", () => {
    renderHook(() => useBookView({ id: 7, authorId: 3, categoryL1: "武侠", categoryL2: "江湖" }));
    expect(track).not.toHaveBeenCalled();
  });

  it("卸载记一次 view，带 bookId / authorId / 分类，value 是可见停留秒数（不是毫秒）", () => {
    const now = vi.spyOn(Date, "now").mockReturnValue(0);
    const { unmount } = renderHook(() =>
      useBookView({ id: 7, authorId: 3, categoryL1: "武侠", categoryL2: "江湖" }),
    );
    now.mockReturnValue(92_000);
    unmount();

    expect(viewCalls()).toHaveLength(1);
    expect(viewCalls()[0][1]).toEqual({
      bookId: 7,
      authorId: 3,
      categoryL1: "武侠",
      categoryL2: "江湖",
      value: 92,
    });
  });

  it("pagehide 收尾事件也报 view，且与卸载合计只有一条", () => {
    const { unmount } = renderHook(() => useBookView({ id: 8, authorId: 1 }));
    act(() => {
      window.dispatchEvent(new Event(FINALE));
    });
    expect(viewCalls()).toHaveLength(1);

    unmount();
    expect(viewCalls()).toHaveLength(1);
  });

  it("关标签前先报停留秒数（pagehide 早于卸载），卸载不再重复", () => {
    const now = vi.spyOn(Date, "now").mockReturnValue(0);
    const { unmount } = renderHook(() => useBookView({ id: 9, authorId: 2 }));
    now.mockReturnValue(30_000);
    act(() => {
      window.dispatchEvent(new Event(FINALE));
    });
    expect(viewCalls()[0][1]).toMatchObject({ bookId: 9, value: 30 });
    unmount();
    expect(viewCalls()).toHaveLength(1);
  });

  it("页面隐藏期间不计时：切走再回来只算可见的那段", () => {
    const now = vi.spyOn(Date, "now").mockReturnValue(0);
    const { unmount } = renderHook(() => useBookView({ id: 10, authorId: 4 }));
    // 读 10 秒后隐藏，再「过了 60 秒」才回来，回来后又读 5 秒
    now.mockReturnValue(10_000);
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });
    now.mockReturnValue(70_000);
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });
    now.mockReturnValue(75_000);
    unmount();

    // 隐藏的 60 秒不算：10 + 5 = 15 秒
    expect(viewCalls()[0][1]).toMatchObject({ bookId: 10, value: 15 });
  });

  it("书还没取回来（null）时什么都不记", () => {
    type Viewed = { id: number; authorId: number } | null;
    const { rerender } = renderHook<ReturnType<typeof useBookView>, Viewed>(
      (props) => useBookView(props),
      { initialProps: null },
    );
    expect(track).not.toHaveBeenCalled();
    rerender({ id: 12, authorId: 5 });
    expect(track).not.toHaveBeenCalled();
  });
});
