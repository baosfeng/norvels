import type { TurnEffect } from "../../../../lib/paging/turnEffects";
import { createCoverEffect } from "./cover";
import { createCurlEffect } from "./curl";
import { createFadeEffect } from "./fade";
import { createNoneEffect } from "./none";
import { createSlideEffect } from "./slide";
import type { EffectFrame, EffectRunner } from "./types";

/**
 * 效果注册表：引擎只认这个名字到 runner 的映射，加新效果只动这里与一个模块。
 * 每种效果都吃同一份 EffectFrame，所以换效果只换视觉、手势与时长口径不变。
 */
export function createEffect(name: TurnEffect, frameOf: () => EffectFrame): EffectRunner {
  switch (name) {
    case "slide":
      return createSlideEffect(frameOf);
    case "cover":
      return createCoverEffect(frameOf);
    case "fade":
      return createFadeEffect(frameOf);
    case "curl":
      return createCurlEffect(frameOf);
    case "none":
    default:
      return createNoneEffect();
  }
}
