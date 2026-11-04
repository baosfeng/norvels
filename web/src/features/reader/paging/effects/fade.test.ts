import { describe, expect, it } from "vitest";
import { computePageBox } from "../../../../lib/paging/pageGeometry";
import { createFadeEffect } from "./fade";
import type { EffectFrame } from "./types";

const box = computePageBox({ stageW: 390, stageH: 748, padX: 18, lineH: 38 });

function setup() {
  const stage = document.createElement("div");
  const cur = document.createElement("div");
  const inc = document.createElement("div");
  stage.append(cur, inc);
  const frame: EffectFrame = { p: 0, dir: 1, box, cur, inc };
  return { stage, cur, inc, frame };
}

describe("淡入淡出：顺序淡出 / 淡入", () => {
  it("前半程只淡出旧页，新页还不出来（不叠字）", () => {
    const { cur, inc, frame } = setup();
    const effect = createFadeEffect(() => frame);
    effect.apply({ ...frame, p: 0.25, dir: 1 });
    expect(Number(cur.style.opacity)).toBeCloseTo(0.5, 5);
    expect(Number(inc.style.opacity)).toBe(0);
  });

  it("后半程旧页已隐去，新页开始淡入", () => {
    const { cur, inc, frame } = setup();
    const effect = createFadeEffect(() => frame);
    effect.apply({ ...frame, p: 0.75, dir: 1 });
    expect(Number(cur.style.opacity)).toBe(0);
    expect(Number(inc.style.opacity)).toBeCloseTo(0.5, 5);
  });

  it("中段两页不同时可见——交叉淡入会让中文正文互相穿透", () => {
    const { cur, inc, frame } = setup();
    const effect = createFadeEffect(() => frame);
    for (const p of [0, 0.2, 0.5, 0.8, 1]) {
      effect.apply({ ...frame, p, dir: 1 });
      const both = Number(cur.style.opacity) > 0.01 && Number(inc.style.opacity) > 0.01;
      expect(both).toBe(false);
    }
  });

  it("销毁时清掉不透明度", () => {
    const { cur, inc, frame } = setup();
    const effect = createFadeEffect(() => frame);
    effect.apply({ ...frame, p: 0.5, dir: 1 });
    effect.destroy();
    expect(cur.style.opacity).toBe("");
    expect(inc.style.opacity).toBe("");
  });
});
