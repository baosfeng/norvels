import { describe, expect, it, vi } from "vitest";
import { bumpStatsRevision, statsRevision, subscribeStatsRevision } from "./statsRevision";

/** 失效通道本身要能在不挂组件的情况下断：bump / subscribe 都是纯函数。 */
describe("statsRevision（左栏统计的跨组件失效通道）", () => {
  it("bump 一次，所有订阅者各收到一次，版本号 +1", () => {
    const a = vi.fn();
    const b = vi.fn();
    const offA = subscribeStatsRevision(a);
    const offB = subscribeStatsRevision(b);
    const before = statsRevision();

    bumpStatsRevision();

    expect(a).toHaveBeenCalledTimes(1);
    expect(b).toHaveBeenCalledTimes(1);
    expect(statsRevision()).toBe(before + 1);
    offA();
    offB();
  });

  it("取消订阅后不再收到，其他订阅者不受影响", () => {
    const off = vi.fn();
    const keep = vi.fn();
    const unsubscribe = subscribeStatsRevision(off);
    const keepOff = subscribeStatsRevision(keep);

    unsubscribe();
    bumpStatsRevision();

    expect(off).not.toHaveBeenCalled();
    expect(keep).toHaveBeenCalledTimes(1);
    keepOff();
  });

  it("重复 bump 每次都通知（多一次只是多重取一次，不做去抖）", () => {
    const listener = vi.fn();
    const off = subscribeStatsRevision(listener);
    bumpStatsRevision();
    bumpStatsRevision();
    expect(listener).toHaveBeenCalledTimes(2);
    off();
  });
});
