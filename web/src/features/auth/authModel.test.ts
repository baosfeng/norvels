import { describe, expect, it } from "vitest";
import {
  absoluteSeenTitle,
  authToastText,
  isCurrentUser,
  isSwitchMode,
  relativeSeenText,
  seenLine,
  shouldLeaveLoginPage,
  switchAccountPath,
} from "./authModel";

/** 固定"现在"：2026-11-01 12:00 本地时间，避免用例随真实时钟漂移。 */
const NOW = new Date(2026, 10, 1, 12, 0, 0).getTime();

function iso(y: number, mo: number, d: number, h = 12, mi = 0): string {
  return new Date(y, mo - 1, d, h, mi).toISOString();
}

describe("账号列表的「上次活跃」文案", () => {
  it("一分钟内（含时钟偏差造成的未来时间）说「刚刚」", () => {
    expect(relativeSeenText(new Date(NOW - 30_000).toISOString(), NOW)).toBe("刚刚");
    expect(relativeSeenText(new Date(NOW + 5_000).toISOString(), NOW)).toBe("刚刚");
  });

  it("一小时内按分钟计", () => {
    expect(relativeSeenText(new Date(NOW - 5 * 60_000).toISOString(), NOW)).toBe("5 分钟前");
    expect(relativeSeenText(new Date(NOW - 59 * 60_000).toISOString(), NOW)).toBe("59 分钟前");
  });

  it("当天按小时计", () => {
    expect(relativeSeenText(iso(2026, 11, 1, 9), NOW)).toBe("3 小时前");
  });

  it("昨天给出「昨天 HH:mm」，再往前给日期", () => {
    expect(relativeSeenText(iso(2026, 10, 31, 14, 3), NOW)).toBe("昨天 14:03");
    expect(relativeSeenText(iso(2026, 10, 20, 8, 0), NOW)).toBe("10月20日");
  });

  it("跨年保留年份", () => {
    expect(relativeSeenText(iso(2025, 12, 31, 23, 59), NOW)).toBe("2025年12月31日");
  });

  it("没有时间（空串 / null / 解析不了）就是「从未登录」", () => {
    expect(relativeSeenText(null, NOW)).toBe("从未登录");
    expect(relativeSeenText("", NOW)).toBe("从未登录");
    expect(relativeSeenText("不是时间", NOW)).toBe("从未登录");
    expect(seenLine(null)).toBe("从未登录");
    expect(seenLine(iso(2026, 11, 1, 9), NOW)).toBe("上次活跃 3 小时前");
  });

  it("title 里放绝对时间，悬停可查", () => {
    expect(absoluteSeenTitle(iso(2026, 10, 31, 14, 3))).toBe("2026年10月31日 14:03");
    expect(absoluteSeenTitle(null)).toBe("建号后还没有登录过");
  });
});

describe("切换账号开关（?switch=1）", () => {
  it("只认 switch=1", () => {
    expect(isSwitchMode(new URLSearchParams("switch=1"))).toBe(true);
    expect(isSwitchMode(new URLSearchParams("switch=0"))).toBe(false);
    expect(isSwitchMode(new URLSearchParams(""))).toBe(false);
  });

  it("已登录默认离开登录页，带 ?switch=1 才留下", () => {
    expect(shouldLeaveLoginPage(true, false)).toBe(true);
    expect(shouldLeaveLoginPage(true, true)).toBe(false);
    expect(shouldLeaveLoginPage(false, false)).toBe(false);
  });

  it("切换入口带回跳地址与开关", () => {
    expect(switchAccountPath("/shelf")).toBe("/login?next=%2Fshelf&switch=1");
    expect(switchAccountPath("/book/7?tab=1")).toBe("/login?next=%2Fbook%2F7%3Ftab%3D1&switch=1");
  });
});

describe("成功提示与「当前」标记", () => {
  it("登录 / 注册说清是「进来了」还是「新建了」", () => {
    expect(authToastText("login", "夜读的人")).toBe("已登录为 夜读的人");
    expect(authToastText("register", "夜读的人")).toBe("已创建并登录为 夜读的人");
  });

  it("「当前」按 id 比，不按名字", () => {
    expect(isCurrentUser({ id: 2 }, { id: 2 })).toBe(true);
    expect(isCurrentUser({ id: 2 }, { id: 3 })).toBe(false);
    expect(isCurrentUser({ id: 2 }, null)).toBe(false);
  });
});
