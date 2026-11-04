import type { TurnEffect } from "../../../../lib/paging/turnEffects";
import { createNoneEffect } from "./none";
import { createSlideEffect } from "./slide";
import type { EffectFrame, EffectRunner } from "./types";

/**
 * 效果注册表：引擎只认这个名字到 runner 的映射。
 * 尚未接线的效果先落到 none——翻页本身照常工作，只是没有动画。
 */
export function createEffect(name: TurnEffect, frameOf: () => EffectFrame): EffectRunner {
  switch (name) {
    case "slide":
      return createSlideEffect(frameOf);
    case "none":
    default:
      return createNoneEffect();
  }
}
