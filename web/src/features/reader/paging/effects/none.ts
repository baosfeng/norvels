import type { EffectRunner } from "./types";

/** 无动画下拖动时的跟手比例：页面跟着手指走一点，松手仍然是瞬时换页。 */
export const NONE_FOLLOW_RATIO = 0.15;

/**
 * 无动画的跟手位移（px）：方向与手指一致、幅度 15%，并封顶在可视宽的 15%。
 *
 * 「无动画」要的是松手零等待，不是拖动零反馈——完全不动会让读者以为手势没被识别，
 * 于是继续往同方向拖（或干脆放弃翻页）。给一层阻尼跟手两头都占：手感有回应，翻页时长不变。
 */
export function noneFollowOffset(dx: number, viewportW: number): number {
  const cap = (Number.isFinite(viewportW) ? Math.max(0, viewportW) : 0) * NONE_FOLLOW_RATIO;
  const pull = Number.isFinite(dx) ? dx * NONE_FOLLOW_RATIO : 0;
  return Math.max(-cap, Math.min(cap, pull));
}

/**
 * 无动画：松手即换页，拖动时只给 15% 阻尼跟手（见 noneFollowOffset）。
 * 三种用途：读者主动选、prefers-reduced-motion 的兜底、量其它效果额外成本的对照基准。
 */
export function createNoneEffect(): EffectRunner {
  return {
    name: "none",
    apply() {
      /* 无过渡：跟手位移由引擎直接写在页层上（这个效果不建第二层） */
    },
    settle(_from, _to, _duration, _dir, done) {
      done();
      return () => {};
    },
    destroy() {
      /* 没有自建 DOM */
    },
  };
}
