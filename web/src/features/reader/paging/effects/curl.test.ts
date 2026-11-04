import { beforeAll, describe, expect, it, vi } from "vitest";
import { computePageBox } from "../../../../lib/paging/pageGeometry";
import { createCurlEffect } from "./curl";
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

// jsdom 不实现 canvas：这里给个最小 2D 上下文桩，让「只画光影」的路径真的跑一遍
// （真实环境里 getContext 拿不到上下文时，效果会自己跳过绘制，不影响翻页本身）
beforeAll(() => {
  const ctx = {
    setTransform: vi.fn(),
    clearRect: vi.fn(),
    fillRect: vi.fn(),
    createLinearGradient: () => ({ addColorStop: vi.fn() }),
    set fillStyle(_v: string) {},
  };
  Object.defineProperty(HTMLCanvasElement.prototype, "getContext", {
    configurable: true,
    value: () => ctx,
  });
});

describe("仿真折页：裁剪而不是 3D", () => {
  it("用 clip-path 切出 C 形折痕（文字是矢量，不会被重采样）", () => {
    const { cur, frame } = setup();
    const effect = createCurlEffect(() => frame);
    effect.apply({ ...frame, p: 0.35, dir: 1 });
    expect(cur.style.clipPath).toContain("path(");
    expect(cur.style.clipPath).toContain("C ");
    // 不做 3D：不写 perspective / rotateY 那套
    expect(cur.style.transform).toBe("translate3d(0,0,0)");
  });

  it("折痕位置随进度左移：p 越大，露出的当前页越窄", () => {
    const { cur, frame } = setup();
    const effect = createCurlEffect(() => frame);
    effect.apply({ ...frame, p: 0.2, dir: 1 });
    const near = Number(/H([\d.]+) C/.exec(cur.style.clipPath)?.[1] ?? "0");
    effect.apply({ ...frame, p: 0.7, dir: 1 });
    const far = Number(/H([\d.]+) C/.exec(cur.style.clipPath)?.[1] ?? "0");
    expect(far).toBeLessThan(near);
  });

  it("纸背卷边与光影画布都在，且只画光影不贴字", () => {
    const { stage, frame } = setup();
    const effect = createCurlEffect(() => frame);
    effect.apply({ ...frame, p: 0.4, dir: 1 });
    const flap = stage.querySelector<HTMLElement>(".fx-flap");
    const canvas = stage.querySelector<HTMLCanvasElement>(".fx-canvas");
    expect(flap).not.toBeNull();
    expect(canvas).not.toBeNull();
    // 画布尺寸跟着页高走（铺满正文区），文字不在画布里
    expect(canvas?.style.height).toBe(`${box.pageH}px`);
    expect(canvas?.textContent).toBe("");
  });

  it("完全翻走（p=1）时折痕贴到版心左缘", () => {
    const { cur, frame } = setup();
    const effect = createCurlEffect(() => frame);
    effect.apply({ ...frame, p: 1, dir: 1 });
    const crease = Number(/H([\d.]+) C/.exec(cur.style.clipPath)?.[1] ?? "-1");
    expect(crease).toBeCloseTo(box.padX, 1);
  });

  it("销毁时摘掉纸背与画布、清掉裁剪", () => {
    const { stage, cur, frame } = setup();
    const effect = createCurlEffect(() => frame);
    effect.apply({ ...frame, p: 0.4, dir: 1 });
    effect.destroy();
    expect(stage.querySelector(".fx-flap")).toBeNull();
    expect(stage.querySelector(".fx-canvas")).toBeNull();
    expect(cur.style.clipPath).toBe("");
  });
});
