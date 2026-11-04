import type { EffectRunner } from "./types";

/**
 * 无动画：松手即换页，拖动时不跟手。
 * 三种用途：读者主动选、prefers-reduced-motion 的兜底、量其它效果额外成本的对照基准。
 */
export function createNoneEffect(): EffectRunner {
  return {
    name: "none",
    apply() {
      /* 无过渡：拖动过程中页面不动 */
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
