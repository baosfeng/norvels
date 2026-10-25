import { describe, expect, it } from "vitest";
import { createReadingTimer, MAX_REPORT_SECONDS } from "./readingTimer";

describe("createReadingTimer", () => {
  it("累计可见时长并在 take 时清零", () => {
    const timer = createReadingTimer();
    timer.start(0);
    expect(timer.take(70_000)).toBe(70);
    expect(timer.take(80_000)).toBe(0);
  });

  it("不足最小秒数不消费，够了下一次一起报", () => {
    const timer = createReadingTimer();
    timer.start(0);
    expect(timer.take(30_000)).toBe(0);
    expect(timer.take(65_000)).toBe(65);
  });

  it("暂停期间不计时，恢复后继续累计", () => {
    const timer = createReadingTimer();
    timer.start(0);
    timer.pause(10_000);
    expect(timer.pending(60_000)).toBe(10);
    timer.start(60_000);
    // 10s + 30s：暂停的那 50s 不算；显式传 min=1，避免被 60s 上报阈值吃掉。
    expect(timer.take(90_000, 1)).toBe(40);
  });

  it("单次上报不超过后端上限", () => {
    const timer = createReadingTimer();
    timer.start(0);
    expect(timer.take((MAX_REPORT_SECONDS + 100) * 1000)).toBe(MAX_REPORT_SECONDS);
  });

  it("重复 start 不重置起点", () => {
    const timer = createReadingTimer();
    timer.start(0);
    timer.start(5_000);
    expect(timer.take(60_000)).toBe(60);
  });
});
