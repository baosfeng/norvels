import { describe, expect, it } from "vitest";
import { centerOffset, centerScrollTop, TOC_CENTER_TOLERANCE } from "./tocScroll";

/** U11：打开目录时当前章在第一帧就在可视区中间，不是隔一拍才滚过去。 */

describe("目录首帧定位（U11）", () => {
  it("把第 1200 章那一行放到中间：scrollTop 一次算对，不需要 scrollIntoView", () => {
    // 目录 1500 行 × 44px = 66000px，抽屉可视 694px；当前章行顶 = 1200 × 44
    const rowTop = 1200 * 44;
    const scrollTop = centerScrollTop({
      rowTop,
      rowHeight: 44,
      clientHeight: 694,
      scrollHeight: 1500 * 44,
    });
    expect(scrollTop).toBe(Math.round(rowTop + 22 - 347));
    // 定位后这一行正好在中间：偏差远小于 40px 的验收线
    const offset = centerOffset({ rowTop, rowHeight: 44, clientHeight: 694, scrollTop });
    expect(offset).toBeLessThanOrEqual(1);
    expect(offset).toBeLessThanOrEqual(TOC_CENTER_TOLERANCE);
  });

  it("当前章在首章时不滚（不会被算成负数）", () => {
    const scrollTop = centerScrollTop({ rowTop: 0, rowHeight: 44, clientHeight: 694, scrollHeight: 66000 });
    expect(scrollTop).toBe(0);
  });

  it("当前章在末章时钳在可滚范围最大值，不会滚出内容", () => {
    const scrollHeight = 1500 * 44;
    const scrollTop = centerScrollTop({
      rowTop: 1499 * 44,
      rowHeight: 44,
      clientHeight: 694,
      scrollHeight,
    });
    expect(scrollTop).toBe(scrollHeight - 694);
    expect(scrollTop).toBeLessThanOrEqual(scrollHeight - 694);
  });

  it("极端数值（还没排版：高宽都是 0）退成 0，不抛也不产生 NaN", () => {
    expect(
      centerScrollTop({ rowTop: Number.NaN, rowHeight: 44, clientHeight: 0, scrollHeight: 0 }),
    ).toBe(0);
    expect(centerScrollTop({ rowTop: 0, rowHeight: 0, clientHeight: 0, scrollHeight: 0 })).toBe(0);
  });

  it("误差口径与审计同源：中心偏差按「行中心 vs 可视区中心」算", () => {
    expect(centerOffset({ rowTop: 1000, rowHeight: 44, clientHeight: 694, scrollTop: 675 })).toBe(0);
    expect(centerOffset({ rowTop: 1000, rowHeight: 44, clientHeight: 694, scrollTop: 615 })).toBe(60);
  });
});
