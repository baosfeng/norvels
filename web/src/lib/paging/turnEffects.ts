/**
 * 翻页效果：取值、时长、动效降级、自动翻页的停留时长。纯函数。
 *
 * 「仿真」是折页卷边（clip-path 曲线折痕 + 纸背卷边 + 光影），不是 3D 硬纸板——
 * 3D 透视重采样会把中文字糊掉，原型里已否决（见 /tmp/norvels-flip-lab/README.md 第三节）。
 */
import { SPEED_MAX, SPEED_MIN } from "../autoScroll";

/** 分页模式下可选的翻页效果。 */
export type TurnEffect = "none" | "slide" | "cover" | "fade" | "curl";

/** 设置面板里的顺序：默认项 first（滑动），仿真放最后。 */
export const TURN_EFFECTS: readonly TurnEffect[] = ["slide", "cover", "fade", "curl", "none"];

export const TURN_EFFECT_LABELS: Record<TurnEffect, string> = {
  none: "无动画",
  slide: "滑动",
  cover: "覆盖",
  fade: "淡入淡出",
  curl: "仿真折页",
};

/** 收尾动画时长（ms）：与原型实测一致。 */
export const TURN_DURATION: Record<TurnEffect, number> = {
  none: 0,
  fade: 200,
  slide: 260,
  cover: 280,
  curl: 380,
};

/** 自动翻页：每页停留时长（ms）。5 档 3.5 秒、0.5 档 14 秒，中间按速度线性插值。 */
export const PAGE_HOLD_FAST_MS = 3500;
export const PAGE_HOLD_SLOW_MS = 14000;

/** 是不是合法的效果值（localStorage 里可能是任意脏串）。 */
export function isTurnEffect(value: unknown): value is TurnEffect {
  return typeof value === "string" && (TURN_EFFECTS as readonly string[]).includes(value);
}

/**
 * 实际生效的效果：减少动效时一切退化到 none（时长 0）。
 * 判定放在调用点而不是模块加载时，读者切系统设置后不用重开页面。
 */
export function resolveEffect(effect: TurnEffect, reducedMotion: boolean): TurnEffect {
  return reducedMotion ? "none" : effect;
}

/** 收尾时长：效果自带的时长，减少动效时为 0。 */
export function effectDuration(effect: TurnEffect, reducedMotion: boolean): number {
  return reducedMotion ? 0 : TURN_DURATION[effect] ?? 0;
}

/** 速度档 → 每页停留毫秒数（线性：0.5 → 14000，5 → 3500）。 */
export function pageHoldMs(speed: number): number {
  const value = Number.isFinite(speed) ? Math.min(SPEED_MAX, Math.max(SPEED_MIN, speed)) : 3;
  const ratio = (value - SPEED_MIN) / (SPEED_MAX - SPEED_MIN);
  return Math.round(PAGE_HOLD_SLOW_MS + (PAGE_HOLD_FAST_MS - PAGE_HOLD_SLOW_MS) * ratio);
}
