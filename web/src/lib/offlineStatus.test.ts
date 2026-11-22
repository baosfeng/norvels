import { describe, expect, it } from "vitest";
import { NEED_SECURE_CONTEXT_HINT } from "./capabilities";
import { OFFLINE_UNAVAILABLE_HINT, offlineBadgeText, offlineState } from "./offlineStatus";

/**
 * 离线徽标的三态：在线不占位置；断网且 Service Worker 已接管才说「离线阅读」；
 * 断网但没有控制器（局域网 http 注册不上 SW）如实说离线不可用并给出访问方式。
 */

describe("offlineState：三种状态由「在线」与「控制器」两个布尔推导", () => {
  it("在线时一律是 online，与控制器有无无关", () => {
    expect(offlineState(true, true)).toBe("online");
    expect(offlineState(true, false)).toBe("online");
  });

  it("断网且已有 Service Worker 接管 → offline-ready", () => {
    expect(offlineState(false, true)).toBe("offline-ready");
  });

  it("断网但没有控制器 → offline-unavailable（这正是局域网 http 的情形）", () => {
    expect(offlineState(false, false)).toBe("offline-unavailable");
  });
});

describe("offlineBadgeText：文案只说做得到的事", () => {
  it("在线返回 null（不渲染、不占位置）", () => {
    expect(offlineBadgeText("online")).toBeNull();
  });

  it("offline-ready 才承诺「离线阅读」", () => {
    expect(offlineBadgeText("offline-ready")).toBe("离线阅读");
  });

  it("offline-unavailable 不承诺离线，改成「离线不可用 + 访问方式」", () => {
    const text = offlineBadgeText("offline-unavailable");
    expect(text).toBe(OFFLINE_UNAVAILABLE_HINT);
    expect(text).toContain("离线不可用");
    expect(text).not.toContain("离线阅读");
  });

  it("不可用的说明复用能力面板那句口径，不另写一句", () => {
    expect(OFFLINE_UNAVAILABLE_HINT).toContain(NEED_SECURE_CONTEXT_HINT);
  });
});
