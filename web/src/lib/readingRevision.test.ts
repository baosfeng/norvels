import { afterEach, describe, expect, it, vi } from "vitest";
import { bumpReadingRevision, readingRevision, subscribeReadingRevision } from "./readingRevision";

// jsdom 31 自带 BroadcastChannel（见 node_modules/jsdom）。这里不 new 真通道，
// 而是把它换成可以手工触发 onmessage 的替身：既能断言「广播了什么」，
// 又能模拟「另一个标签页发来了消息」，还不用等真实的消息调度。
interface FakeChannel {
  name: string;
  onmessage: ((event: MessageEvent) => void) | null;
  postMessage: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
}

const opened: FakeChannel[] = [];

function installFakeChannel(): void {
  opened.length = 0;
  class FakeBroadcastChannel implements FakeChannel {
    name: string;
    onmessage: ((event: MessageEvent) => void) | null = null;
    postMessage = vi.fn();
    close = vi.fn();
    constructor(name: string) {
      this.name = name;
      opened.push(this);
    }
  }
  vi.stubGlobal("BroadcastChannel", FakeBroadcastChannel);
}

function currentChannel(): FakeChannel {
  const channel = opened.at(-1);
  if (!channel) throw new Error("模块没有打开 BroadcastChannel");
  return channel;
}

/** 模拟另一个标签页发来的信号（onmessage 是模块建通道时挂上去的）。 */
function remoteSignal(channel: FakeChannel): void {
  channel.onmessage?.({ data: { type: "reading" } } as MessageEvent);
}

afterEach(() => {
  vi.unstubAllGlobals();
});

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

/**
 * 跨标签页：模块在导入时就建好那条通道（常驻，不随订阅数归零而 close），
 * 所以这一组用独立导入的模块实例来断通道本身的行为。
 */
describe("readingRevision 跨标签页同步", () => {
  it("通道常驻：模块加载时按 norvels- 前缀建一次，订阅/退订都不 new 也不 close", async () => {
    installFakeChannel();
    vi.resetModules();
    await import("./readingRevision");
    expect(opened).toHaveLength(1);
    expect(currentChannel().name).toBe("norvels-reading-revision");

    const { subscribeReadingRevision: subscribe } = await import("./readingRevision");
    const off = subscribe(() => {});
    off();
    expect(opened).toHaveLength(1);
    expect(currentChannel().close).not.toHaveBeenCalled();
  });

  it("bump 广播一条只带 type 的信号，不夹带书 / 章节 / 账号", async () => {
    installFakeChannel();
    vi.resetModules();
    const { bumpReadingRevision: bump } = await import("./readingRevision");

    bump();

    expect(currentChannel().postMessage).toHaveBeenCalledTimes(1);
    expect(currentChannel().postMessage).toHaveBeenCalledWith({ type: "reading" });
  });

  it("收到别的标签页的信号：只通知本地订阅者，版本号不变、也不再往外广播", async () => {
    installFakeChannel();
    vi.resetModules();
    const mod = await import("./readingRevision");
    const listener = vi.fn();
    const off = mod.subscribeReadingRevision(listener);
    const before = mod.readingRevision();

    remoteSignal(currentChannel());

    expect(listener).toHaveBeenCalledTimes(1);
    // 不再 bump：否则两个标签页会一个通知一个，来回弹成死循环
    expect(mod.readingRevision()).toBe(before);
    expect(currentChannel().postMessage).not.toHaveBeenCalled();
    off();
  });

  it("拿不到 BroadcastChannel（老的 jsdom / 老浏览器）时退化成纯本地通知", async () => {
    vi.stubGlobal("BroadcastChannel", undefined);
    vi.resetModules();
    const mod = await import("./readingRevision");
    const listener = vi.fn();
    const off = mod.subscribeReadingRevision(listener);
    const before = mod.readingRevision();

    expect(() => mod.bumpReadingRevision()).not.toThrow();
    expect(listener).toHaveBeenCalledTimes(1);
    expect(mod.readingRevision()).toBe(before + 1);
    off();
  });

  it("构造通道抛错（严格隐私模式的 webview）时同样退化成纯本地通知", async () => {
    class ThrowingChannel {
      constructor() {
        throw new Error("blocked by policy");
      }
    }
    vi.stubGlobal("BroadcastChannel", ThrowingChannel);
    vi.resetModules();
    const mod = await import("./readingRevision");
    const listener = vi.fn();
    const off = mod.subscribeReadingRevision(listener);

    expect(() => mod.bumpReadingRevision()).not.toThrow();
    expect(listener).toHaveBeenCalledTimes(1);
    off();
  });
});
