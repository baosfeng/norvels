import { describe, expect, it } from "vitest";
import { computePageBox } from "../../../../lib/paging/pageGeometry";
import { createCoverEffect } from "./cover";
import type { EffectFrame } from "./types";

const box = computePageBox({ stageW: 390, stageH: 748, padX: 18, lineH: 38 });

/** jsdom 没有布局：造两个裸 div 当页层，只验样式写入。 */
function setup() {
  const stage = document.createElement("div");
  const cur = document.createElement("div");
  const inc = document.createElement("div");
  stage.append(cur, inc);
  const frame: EffectFrame = { p: 0, dir: 1, box, cur, inc };
  return { stage, cur, inc, frame };
}

/** 从 translate3d(x,y,z) 里取第一个参数（x）。 */
function xOf(transform: string): number {
  const match = /translate3d\((-?[\d.]+)px/.exec(transform);
  return match ? Number.parseFloat(match[1]) : Number.NaN;
}

describe("覆盖效果：两层位移与投影", () => {
  it("下一页：新页从右压入，旧页只后退 14%", () => {
    const { cur, inc, frame } = setup();
    const effect = createCoverEffect(() => frame);
    effect.apply({ ...frame, p: 0.5, dir: 1 });
    // 新页位移 (1-p)·可视宽 = 195
    expect(inc.style.transform).toBe("translate3d(195px,0,0)");
    // 旧页后退 14% × 195 = 27.3
    expect(xOf(cur.style.transform)).toBeCloseTo(-27.3, 1);
  });

  it("左缘投影跟着新页走，不透明度按 min(1, 3p) 起亮", () => {
    const { stage, frame } = setup();
    const effect = createCoverEffect(() => frame);
    effect.apply({ ...frame, p: 0.2, dir: 1 });
    const shadow = stage.querySelector<HTMLElement>(".fx-cover-shadow");
    expect(shadow).not.toBeNull();
    // 新页左缘 = (1-p)·可视宽 = 0.8 × 390（取 translate3d 的第一个参数）
    expect(xOf(shadow?.style.transform ?? "")).toBeCloseTo(312, 1);
    expect(Number(shadow?.style.opacity)).toBeCloseTo(0.6, 5);
  });

  it("上一页：被盖住的那页滑回，底页不动", () => {
    const { cur, inc, frame } = setup();
    const effect = createCoverEffect(() => frame);
    effect.apply({ ...frame, p: 0.4, dir: -1 });
    expect(cur.style.transform).toBe("translate3d(156px,0,0)");
    expect(inc.style.transform).toBe("translate3d(0,0,0)");
  });

  it("销毁时清掉投影层与内联位移，回到静态页", () => {
    const { stage, cur, inc, frame } = setup();
    const effect = createCoverEffect(() => frame);
    effect.apply({ ...frame, p: 0.5, dir: 1 });
    effect.destroy();
    expect(stage.querySelector(".fx-cover-shadow")).toBeNull();
    expect(cur.style.transform).toBe("");
    expect(inc.style.transform).toBe("");
  });
});
