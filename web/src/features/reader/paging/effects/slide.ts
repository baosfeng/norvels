import type { EffectFrame, EffectRunner } from "./types";
import { clearInline, transition } from "./types";

/**
 * 滑动：当前页左移、下一页从右侧同步跟进。
 *
 * 位移基准是**可视宽**（stage 整宽）不是版心宽：版心两侧还有留白，
 * 按版心宽算两页之间会露出缝，翻到一半能看见底下那页的边缘。
 * 拖动时逐帧写 transform；松手交给 CSS transition 跑完（纯位移，合成器最省）。
 */
const PROPS = ["transform", "transition"] as const;

export function createSlideEffect(frameOf: () => EffectFrame): EffectRunner {
  const place = (p: number, dir: number) => {
    const { cur, inc, box } = frameOf();
    const width = box.viewportW;
    cur.style.transform = `translate3d(${(dir > 0 ? -p : p) * width}px,0,0)`;
    if (inc) inc.style.transform = `translate3d(${(dir > 0 ? 1 - p : p - 1) * width}px,0,0)`;
  };

  return {
    name: "slide",
    apply(frame) {
      place(frame.p, frame.dir);
    },
    settle(_from, to, duration, dir, done) {
      // 只有一层（越界 / 页还没建）时不跑过渡，直接收尾
      if (!frameOf().inc) {
        done();
        return () => {};
      }
      const cancel = transition(frameOf().cur, "transform", duration, () => place(to, dir), done);
      return () => {
        cancel();
        done();
      };
    },
    destroy() {
      const { cur, inc } = frameOf();
      clearInline(cur, PROPS);
      clearInline(inc, PROPS);
    },
  };
}
