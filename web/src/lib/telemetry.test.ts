import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/**
 * 埋点 SDK：把 api.postEvents 换成桩，用假计时器推进「1s 合并窗口」。
 * 断言只针对**发出去的请求体**与队列长度，不依赖内部实现细节。
 */
const postEvents = vi.hoisted(() => vi.fn());

vi.mock("../api/client", () => ({ api: { postEvents } }));

const {
  FLUSH_DELAY_MS,
  IMPRESSION_DWELL_MS,
  MAX_BATCH,
  TELEMETRY_FINALE_EVENT,
  flush,
  flushOnHide,
  installTelemetry,
  pendingEvents,
  resetTelemetry,
  track,
  useImpression,
} = await import("./telemetry");

beforeEach(() => {
  vi.useFakeTimers();
  resetTelemetry();
  postEvents.mockReset();
  postEvents.mockResolvedValue({ accepted: 1 });
});

afterEach(() => {
  resetTelemetry();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

/** 推进合并窗口并清空微任务（flush 是 async）。 */
async function advanceToFlush() {
  await act(async () => {
    vi.advanceTimersByTime(FLUSH_DELAY_MS);
  });
}

function lastBatch(): { kind: string; bookId?: number }[] {
  return postEvents.mock.calls[postEvents.mock.calls.length - 1][0] as { kind: string; bookId?: number }[];
}

describe("track：内存队列 1s 合并成一次 POST", () => {
  it("1s 内的多条事件合成一批", async () => {
    track("click", { bookId: 1 });
    track("impression", { bookId: 2 });
    expect(postEvents).not.toHaveBeenCalled();

    await advanceToFlush();
    expect(postEvents).toHaveBeenCalledTimes(1);
    expect(lastBatch().map((event) => event.kind)).toEqual(["click", "impression"]);
    expect(lastBatch().map((event) => event.bookId)).toEqual([1, 2]);
    expect(pendingEvents()).toBe(0);
  });

  it("超过一批上限（100 条）时拆成多批，不会把后端 400 掉", async () => {
    for (let bookId = 1; bookId <= MAX_BATCH + 1; bookId += 1) track("impression", { bookId });

    await advanceToFlush();
    expect(postEvents).toHaveBeenCalledTimes(1);
    expect(lastBatch()).toHaveLength(MAX_BATCH);
    expect(pendingEvents()).toBe(1);

    await advanceToFlush();
    expect(postEvents).toHaveBeenCalledTimes(2);
    expect(lastBatch()).toHaveLength(1);
    expect(pendingEvents()).toBe(0);
  });

  it("队列有硬上限：离线再久也不无限涨内存", () => {
    for (let i = 0; i < 700; i += 1) track("impression", { bookId: i });
    expect(pendingEvents()).toBe(500);
  });
});

describe("失败口径：网络类静默重试一次，4xx 直接丢", () => {
  it("网络失败重试一次，第二次成功即清空队列", async () => {
    postEvents.mockRejectedValueOnce(new Error("network down"));
    track("click", { bookId: 7 });

    await advanceToFlush();
    expect(postEvents).toHaveBeenCalledTimes(1);
    expect(pendingEvents()).toBe(1);

    await advanceToFlush();
    expect(postEvents).toHaveBeenCalledTimes(2);
    expect(pendingEvents()).toBe(0);
  });

  it("4xx 是数据不被接受：不重试，直接丢", async () => {
    postEvents.mockRejectedValueOnce(Object.assign(new Error("bad kind"), { status: 400 }));
    track("view", { bookId: 8, value: 3 });

    await advanceToFlush();
    await advanceToFlush();
    await advanceToFlush();
    expect(postEvents).toHaveBeenCalledTimes(1);
    expect(pendingEvents()).toBe(0);
  });

  it("连续失败两次后不再重试（埋点不能变成死循环）", async () => {
    postEvents.mockRejectedValue(new Error("still down"));
    track("click", { bookId: 9 });

    await advanceToFlush();
    await advanceToFlush();
    await advanceToFlush();
    await advanceToFlush();
    expect(postEvents).toHaveBeenCalledTimes(2);
  });

  it("flush 在没有事件时不发请求", async () => {
    expect(await flush()).toBe(0);
    expect(postEvents).not.toHaveBeenCalled();
  });
});

describe("卸载前不丢：pagehide 走 sendBeacon，且收尾事件先入队", () => {
  function stubBeacon(result = true) {
    const beacon = vi.fn<(url: string, body: Blob) => boolean>().mockReturnValue(result);
    Object.defineProperty(navigator, "sendBeacon", { value: beacon, configurable: true, writable: true });
    return beacon;
  }

  /**
   * jsdom 的 Blob 没有 .text()：用 FileReader 读，等价于浏览器里的 await blob.text()。
   * FileReader 的任务排在宏任务里，假计时器下永远不回调，所以这几个用例改用真计时器
   * （flushOnHide 本身是同步的，不依赖 1s 合并窗口）。
   */
  function readBlob(blob: Blob): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.onerror = () => reject(reader.error);
      reader.readAsText(blob);
    });
  }

  async function bodyOf(beacon: ReturnType<typeof stubBeacon>) {
    const blob = beacon.mock.calls[0][1];
    return JSON.parse(await readBlob(blob)) as {
      events: { kind: string; bookId: number; value?: number }[];
    };
  }

  it("pagehide 触发 flushOnHide：队列整批发给 sendBeacon，不再走 fetch", async () => {
    vi.useRealTimers();
    const beacon = stubBeacon();
    installTelemetry();
    track("click", { bookId: 5 });

    act(() => {
      window.dispatchEvent(new Event("pagehide"));
    });

    expect(beacon).toHaveBeenCalledTimes(1);
    expect(beacon.mock.calls[0][0]).toBe("/api/events");
    const body = await bodyOf(beacon);
    expect(body.events).toHaveLength(1);
    expect(body.events[0]).toMatchObject({ kind: "click", bookId: 5 });
    expect(pendingEvents()).toBe(0);
    // beacon 成功就不再补发一次 fetch
    expect(postEvents).not.toHaveBeenCalled();
  });

  it("收尾事件先入队再 beacon：详情页停留秒数不会丢", async () => {
    vi.useRealTimers();
    const beacon = stubBeacon();
    const onFinale = () => track("view", { bookId: 6, value: 42 });
    window.addEventListener(TELEMETRY_FINALE_EVENT, onFinale);
    track("click", { bookId: 6 });

    act(() => {
      flushOnHide();
    });
    window.removeEventListener(TELEMETRY_FINALE_EVENT, onFinale);

    const body = await bodyOf(beacon);
    expect(body.events.map((event) => event.kind)).toEqual(["click", "view"]);
    expect(body.events[1].value).toBe(42);
  });

  it("sendBeacon 不可用时退回普通 flush", async () => {
    vi.useRealTimers();
    Object.defineProperty(navigator, "sendBeacon", { value: undefined, configurable: true, writable: true });
    track("click", { bookId: 11 });
    await act(async () => {
      flushOnHide();
    });
    expect(postEvents).toHaveBeenCalledTimes(1);
    expect(lastBatch()).toHaveLength(1);
  });
});

/** IntersectionObserver 桩：手动决定「可见比例」，不依赖 jsdom 的布局能力。 */
class FakeObserver {
  static all: FakeObserver[] = [];
  readonly callback: IntersectionObserverCallback;
  targets: Element[] = [];
  disconnected = false;

  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    FakeObserver.all.push(this);
  }

  observe(target: Element) {
    this.targets.push(target);
  }

  unobserve() {}

  disconnect() {
    this.disconnected = true;
  }

  takeRecords() {
    return [];
  }

  /** 模拟一次可见性变化。 */
  fire(ratio: number) {
    const entry = {
      isIntersecting: ratio >= 0.5,
      intersectionRatio: ratio,
      target: this.targets[0],
    } as IntersectionObserverEntry;
    this.callback([entry], this as unknown as IntersectionObserver);
  }
}

describe("useImpression：可见 ≥50% 且 ≥500ms 记一次曝光", () => {
  function mount(bookId = 21, extra?: string) {
    const node = document.createElement("div");
    document.body.appendChild(node);
    const ref = { current: node };
    const hook = renderHook(() => useImpression(ref, { bookId, categoryL1: "武侠", extra }));
    return { hook, observer: FakeObserver.all[FakeObserver.all.length - 1] };
  }

  beforeEach(() => {
    FakeObserver.all = [];
    vi.stubGlobal("IntersectionObserver", FakeObserver);
  });

  it("停留够 500ms 才记 impression，参数带 bookId 与分类", async () => {
    const { observer } = mount(21);
    act(() => observer.fire(0.8));
    // 还没到 500ms：不入队
    await act(async () => {
      vi.advanceTimersByTime(IMPRESSION_DWELL_MS - 1);
    });
    expect(pendingEvents()).toBe(0);

    await act(async () => {
      vi.advanceTimersByTime(1);
    });
    expect(pendingEvents()).toBe(1);
    await advanceToFlush();
    expect(lastBatch()[0]).toMatchObject({ kind: "impression", bookId: 21, categoryL1: "武侠" });
  });

  it("曝光带上推荐理由（extra），后端存进 events.extra 才能按理由复盘", async () => {
    const { observer } = mount(25, "因为你常看武侠");
    act(() => observer.fire(0.8));
    await act(async () => {
      vi.advanceTimersByTime(IMPRESSION_DWELL_MS);
    });
    await advanceToFlush();
    expect(lastBatch()[0]).toMatchObject({
      kind: "impression",
      bookId: 25,
      categoryL1: "武侠",
      extra: "因为你常看武侠",
    });
  });

  it("可见比例不足 50% 不算看见", async () => {
    const { observer } = mount(22);
    act(() => observer.fire(0.3));
    await act(async () => {
      vi.advanceTimersByTime(IMPRESSION_DWELL_MS * 4);
    });
    expect(postEvents).not.toHaveBeenCalled();
  });

  it("露一下就滑走：计时器被取消，之后不再补记", async () => {
    const { observer } = mount(23);
    act(() => observer.fire(0.9));
    await act(async () => {
      vi.advanceTimersByTime(IMPRESSION_DWELL_MS - 100);
    });
    act(() => observer.fire(0));
    await act(async () => {
      vi.advanceTimersByTime(IMPRESSION_DWELL_MS * 4);
    });
    expect(postEvents).not.toHaveBeenCalled();
  });

  it("同一张卡只记一次", async () => {
    const { observer } = mount(24);
    act(() => observer.fire(0.9));
    await act(async () => {
      vi.advanceTimersByTime(IMPRESSION_DWELL_MS);
    });
    // 记为曝光后 observer 已断开：再「可见」也不会重复入队
    act(() => observer.fire(0.9));
    await act(async () => {
      vi.advanceTimersByTime(FLUSH_DELAY_MS);
    });
    expect(lastBatch()).toHaveLength(1);
  });
});
