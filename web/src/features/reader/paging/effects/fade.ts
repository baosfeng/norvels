import type { EffectFrame, EffectRunner } from "./types";
import { clearInline } from "./types";

/**
 * 淡入淡出：**顺序**淡出 / 淡入，不是交叉淡入。
 *
 * 交叉淡入（cur.opacity = 1-p、inc.opacity = p）在 p≈0.5 时两页中文正文各 50%
 * 叠在一起，字与字互相穿透、整屏发灰——原型实测后改成顺序：
 * 前半程旧页淡出露出纸底，后半程新页淡入。
 * 收尾同样逐帧：两个阶段没法用一条 transition 表达。
 */
const PROPS = ["opacity", "transition"] as const;

export function createFadeEffect(frameOf: () => EffectFrame): EffectRunner {
  const place = (p: number) => {
    const { cur, inc } = frameOf();
    cur.style.opacity = String(1 - Math.min(1, p * 2));
    if (inc) inc.style.opacity = String(Math.max(0, p * 2 - 1));
  };

  return {
    name: "fade",
    apply(frame) {
      place(frame.p);
    },
    settle(from, to, duration, _dir, done) {
      if (!frameOf().inc) {
        done();
        return () => {};
      }
      if (!(duration > 0)) {
        place(to);
        done();
        return () => {};
      }
      const start = performance.now();
      let raf = 0;
      const step = () => {
        const t = Math.min(1, (performance.now() - start) / duration);
        place(from + (to - from) * (1 - Math.pow(1 - t, 3)));
        if (t < 1) raf = requestAnimationFrame(step);
        else done();
      };
      raf = requestAnimationFrame(step);
      return () => cancelAnimationFrame(raf);
    },
    destroy() {
      const { cur, inc } = frameOf();
      clearInline(cur, PROPS);
      clearInline(inc, PROPS);
    },
  };
}
