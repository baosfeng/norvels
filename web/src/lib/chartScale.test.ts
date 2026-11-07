import { describe, expect, it } from "vitest";
import {
  barHeight,
  bandScale,
  barWidth,
  clampNumber,
  clipText,
  domainTopOf,
  estimateTextWidth,
  labelStride,
  labelVisible,
  LABEL_MIN_GAP,
  linearScale,
  niceTicks,
  stackSlices,
  tickStep,
  ticksWithin,
  ZERO_BAR_PX,
} from "./chartScale";

/**
 * 图表几何内核的单元测试。
 *
 * 这里锁住的是「驾驶舱 13 张图共用哪套口径」：柱宽收敛、0 值基线、刻度取 1/2/5、
 * 标签最小间距、堆叠条按真实占比。改这些数等于改所有图的样子，所以逐条钉在这里，
 * 而不是散在各张图的 DOM 断言里。
 */

describe("clampNumber", () => {
  it("收敛到区间内；NaN / 无穷当 lo", () => {
    expect(clampNumber(5, 0, 10)).toBe(5);
    expect(clampNumber(-3, 0, 10)).toBe(0);
    expect(clampNumber(30, 0, 10)).toBe(10);
    expect(clampNumber(Number.NaN, 2, 10)).toBe(2);
    // 无穷也不当有限值用：一律按 lo 处理（调用方不需要再各自判一次）
    expect(clampNumber(Number.POSITIVE_INFINITY, 0, 10)).toBe(0);
  });
});

describe("bandScale（分列）", () => {
  it("14 天 350px：柱宽是列宽的 62%（约 15px），不是整列 25px", () => {
    const band = bandScale(14, 350);
    expect(band.step).toBeCloseTo(25, 6);
    expect(band.bandwidth).toBeCloseTo(25 * 0.62, 6);
    // 相邻柱不粘连：柱子中心间距 = 列宽
    expect(band.center(1) - band.center(0)).toBeCloseTo(band.step, 6);
    // 首列与末列都落在 [0, 350] 内
    expect(band.start(0)).toBeGreaterThanOrEqual(0);
    expect(band.start(13) + band.bandwidth).toBeLessThanOrEqual(350);
  });

  it("count / width 为 0 或非法时退化成全 0，不产生 NaN 坐标", () => {
    for (const band of [bandScale(0, 100), bandScale(5, 0), bandScale(Number.NaN, Number.NaN)]) {
      expect(band.step).toBe(0);
      expect(band.bandwidth).toBe(0);
      expect(band.start(3)).toBe(0);
      expect(band.center(3)).toBe(0);
    }
  });

  it("padding 被钳在 0–0.9：负数不会让柱子超出列宽", () => {
    expect(bandScale(4, 100, -1).bandwidth).toBeCloseTo(25, 6);
    expect(bandScale(4, 100, 0.95).bandwidth).toBeCloseTo(25 * 0.1, 6);
  });
});

describe("linearScale（值 → 像素）", () => {
  it("域 [0, 100] → 像素 [100(底), 0(顶)]：0 在底、100 在顶", () => {
    const scale = linearScale(0, 100, 100, 0);
    expect(scale.zero).toBe(100);
    expect(scale.scale(0)).toBe(100);
    expect(scale.scale(50)).toBe(50);
    expect(scale.scale(100)).toBe(0);
    expect(scale.invert(50)).toBe(50);
  });

  it("超出域的值被钳住：柱子不会画到图体外面", () => {
    const scale = linearScale(0, 100, 100, 0);
    expect(scale.scale(400)).toBe(0);
    expect(scale.scale(-5)).toBe(100);
  });

  it("域退化成 0 宽时给 1 的跨度，不除零", () => {
    const scale = linearScale(0, 0, 100, 0);
    expect(scale.domainMax).toBe(1);
    expect(Number.isFinite(scale.scale(0.5))).toBe(true);
  });
});

describe("刻度：1/2/5 × 10ⁿ", () => {
  it("domainTopOf 只留 8% 的空气：最高一根柱占绘图区 92% 以上", () => {
    const top = domainTopOf(838);
    expect(top).toBeCloseTo(905.04, 2);
    // 838 / 905 = 92.6%：把顶刻度当域时只有 83.8%，图体填充率就掉到 80% 以下
    expect(838 / top).toBeGreaterThan(0.92);
  });

  it("0 / 负数 / NaN 的域给 1，不至于算出 0 高度", () => {
    expect(domainTopOf(0)).toBe(1);
    expect(domainTopOf(-5)).toBe(1);
    expect(domainTopOf(Number.NaN)).toBe(1);
  });

  it("tickStep 只取 1/2/5 × 10ⁿ", () => {
    expect(tickStep(905, 5)).toBe(200);
    expect(tickStep(324, 5)).toBe(100);
    expect(tickStep(3.2, 3)).toBe(2);
    expect(tickStep(0, 3)).toBe(1);
    for (const [max, count] of [[905, 5], [324, 3], [61, 4], [9999, 5]] as const) {
      const step = tickStep(max, count);
      const norm = step / 10 ** Math.floor(Math.log10(step));
      expect([1, 2, 5, 10]).toContain(Math.round(norm));
    }
  });

  it("ticksWithin 只返回域内的刻度，且含 0", () => {
    expect(ticksWithin(200, 905)).toEqual([0, 200, 400, 600, 800]);
    expect(ticksWithin(500, 905)).toEqual([0, 500]);
    expect(ticksWithin(1, 3.2)).toEqual([0, 1, 2, 3]);
    expect(ticksWithin(0, 100)).toEqual([0]);
  });

  it("niceTicks 覆盖最大值（旧口径保留：上界是刻度的整数倍）", () => {
    const ticks = niceTicks(838, 3);
    expect(ticks[0]).toBe(0);
    expect(ticks[ticks.length - 1]).toBeGreaterThanOrEqual(838);
    expect(niceTicks(0)).toEqual([0]);
  });
});

describe("标签抽稀：最小间距 44px、首尾必标", () => {
  it("14 列 350px：隔一列标一次，相邻标签中心距 ≥ 44px", () => {
    const band = bandScale(14, 350, 0);
    const stride = labelStride(14, band.step);
    expect(stride).toBe(2);
    const visible = labelVisible(14, stride);
    expect(visible[0]).toBe(true);
    expect(visible[13]).toBe(true);
    const shown = visible.map((flag, index) => (flag ? index : -1)).filter((index) => index >= 0);
    for (let i = 1; i < shown.length; i += 1) {
      expect((shown[i] - shown[i - 1]) * band.step).toBeGreaterThanOrEqual(LABEL_MIN_GAP);
    }
  });

  it("末尾那根不会被抽稀点挤住：12 号位让给 13 号位", () => {
    const visible = labelVisible(14, 2);
    expect(visible[12]).toBe(false);
    expect(visible[13]).toBe(true);
    // 让位之后最后一个抽稀点是 10 号，与 13 号隔了 3 列
    expect(visible[10]).toBe(true);
  });

  it("列数够宽时每列都标；单列 / 非法输入不崩", () => {
    expect(labelStride(6, 60)).toBe(1);
    expect(labelVisible(6, 1).every(Boolean)).toBe(true);
    expect(labelVisible(1, 1)).toEqual([true]);
    expect(labelVisible(0, 3)).toEqual([]);
    expect(labelStride(0, 40)).toBe(1);
  });
});

describe("柱宽 / 柱高", () => {
  it("柱宽收敛在 4–28px：14 天每根 15px，6 个桶给满 28px", () => {
    expect(barWidth(bandScale(14, 350).bandwidth, 1)).toBeCloseTo(15.5, 0);
    expect(barWidth(bandScale(6, 350).bandwidth, 1)).toBe(28);
    expect(barWidth(0, 1)).toBe(0);
  });

  it("0 值留 1px 基线：既不是「没有柱」也不是看得见的一截", () => {
    const scale = linearScale(0, 905, 100, 0);
    expect(ZERO_BAR_PX).toBe(1);
    expect(barHeight(0, scale)).toBe(1);
    expect(barHeight(-3, scale)).toBe(1);
    expect(barHeight(Number.NaN, scale)).toBe(1);
    // 有值的柱高按线性刻度算，最小也不低于基线
    expect(barHeight(838, scale)).toBeCloseTo(100 * (838 / 905), 4);
  });
});

describe("堆叠条切点：真实占比", () => {
  it("段宽 = 真实占比（不放大最小段），三段之和正好等于条宽", () => {
    const slices = stackSlices([96.94, 3.06, 0.04], 700);
    expect(slices[0].start).toBe(0);
    expect(slices[0].end - slices[0].start).toBeCloseTo(700 * (96.94 / 100.04), 4);
    // 0.04% 的那段不到一个像素：按真实比例画就是看不见，由文案写「<1%」解释
    const tail = slices[2];
    expect(tail.end - tail.start).toBeLessThan(1);
    expect(slices[2].end).toBeCloseTo(700, 6);
  });

  it("全 0 / 负数 / 非法输入都给 0 长度，不产生 NaN", () => {
    for (const slice of stackSlices([0, 0], 100)) {
      expect(slice.start).toBe(0);
      expect(slice.end).toBe(0);
    }
    const mixed = stackSlices([10, -5, Number.NaN, 30], 100);
    expect(mixed[0].end - mixed[0].start).toBeCloseTo(25, 6);
    expect(mixed[1].end - mixed[1].start).toBe(0);
    expect(mixed[3].end).toBeCloseTo(100, 6);
  });
});

describe("文本宽度估算与截断", () => {
  it("中文按 1em、数字按约 0.58em 估宽", () => {
    expect(estimateTextWidth("武侠", 10)).toBeCloseTo(20, 6);
    expect(estimateTextWidth("123", 10)).toBeCloseTo(17.4, 6);
    // 非法字号退回 11px，不会算出 0 宽
    expect(estimateTextWidth("武侠", Number.NaN)).toBeCloseTo(22, 6);
  });

  it("放不下就截断补「…」，放得下原样返回", () => {
    expect(clipText("演示作者己", 200, 10)).toBe("演示作者己");
    const clipped = clipText("演示作者己很长很长的名字", 40, 10);
    expect(clipped.endsWith("…")).toBe(true);
    expect(estimateTextWidth(clipped, 10)).toBeLessThanOrEqual(40);
  });
});
