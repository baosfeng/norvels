import { describe, expect, it, vi } from "vitest";
import { bumpReadingRevision, readingRevision, subscribeReadingRevision } from "./readingRevision";

/** 失效通道本身要能在不挂组件的情况下断：bump / subscribe 都是纯函数。 */
describe("readingRevision（阅读记录的跨组件失效通道）", () => {
  it("bump 一次，所有订阅者各收到一次，版本号 +1", () => {
    const a = vi.fn();
    const b = vi.fn();
    const offA = subscribeReadingRevision(a);
    const offB = subscribeReadingRevision(b);
    const before = readingRevision();

    bumpReadingRevision();

    expect(a).toHaveBeenCalledTimes(1);
    expect(b).toHaveBeenCalledTimes(1);
    expect(readingRevision()).toBe(before + 1);
    offA();
    offB();
  });

  it("取消订阅后不再收到，其他订阅者不受影响", () => {
    const off = vi.fn();
    const keep = vi.fn();
    const unsubscribe = subscribeReadingRevision(off);
    const keepOff = subscribeReadingRevision(keep);

    unsubscribe();
    bumpReadingRevision();

    expect(off).not.toHaveBeenCalled();
    expect(keep).toHaveBeenCalledTimes(1);
    keepOff();
  });

  it("与库存量那条通道互不串台：只通知自己的订阅者", async () => {
    const { subscribeStatsRevision } = await import("./statsRevision");
    const reading = vi.fn();
    const stock = vi.fn();
    const offReading = subscribeReadingRevision(reading);
    const offStock = subscribeStatsRevision(stock);

    bumpReadingRevision();

    expect(reading).toHaveBeenCalledTimes(1);
    expect(stock).not.toHaveBeenCalled();
    offReading();
    offStock();
  });
});
