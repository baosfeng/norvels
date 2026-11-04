import { describe, expect, it } from "vitest";
import {
  PAGE_HOLD_FAST_MS,
  PAGE_HOLD_SLOW_MS,
  TURN_DURATION,
  TURN_EFFECTS,
  isTurnEffect,
  effectDuration,
  pageHoldMs,
  resolveEffect,
} from "./turnEffects";

describe("翻页效果：取值与时长", () => {
  it("五种效果都在，默认第一个是滑动", () => {
    expect(TURN_EFFECTS).toEqual(["slide", "cover", "fade", "curl", "none"]);
  });

  it("时长与原型实测一致：none 0 / fade 200 / slide 260 / cover 280 / curl 380", () => {
    expect(TURN_DURATION).toEqual({ none: 0, fade: 200, slide: 260, cover: 280, curl: 380 });
  });

  it("减少动效时一切退化到 none、时长为 0", () => {
    expect(resolveEffect("curl", true)).toBe("none");
    expect(resolveEffect("curl", false)).toBe("curl");
    expect(effectDuration("curl", true)).toBe(0);
    expect(effectDuration("curl", false)).toBe(380);
  });

  it("脏值（localStorage 里手改过的串）不算合法效果", () => {
    expect(isTurnEffect("slide")).toBe(true);
    expect(isTurnEffect("curl2")).toBe(false);
    expect(isTurnEffect(null)).toBe(false);
  });
});

describe("自动翻页：速度档 → 每页停留", () => {
  it("0.5 档 14 秒、5 档 3.5 秒，中间线性插值", () => {
    expect(pageHoldMs(0.5)).toBe(PAGE_HOLD_SLOW_MS);
    expect(pageHoldMs(5)).toBe(PAGE_HOLD_FAST_MS);
    expect(pageHoldMs(3)).toBeGreaterThan(PAGE_HOLD_FAST_MS);
    expect(pageHoldMs(3)).toBeLessThan(PAGE_HOLD_SLOW_MS);
  });

  it("越界档位与脏值都被夹住", () => {
    expect(pageHoldMs(0)).toBe(PAGE_HOLD_SLOW_MS);
    expect(pageHoldMs(99)).toBe(PAGE_HOLD_FAST_MS);
    expect(pageHoldMs(Number.NaN)).toBe(pageHoldMs(3));
  });
});
