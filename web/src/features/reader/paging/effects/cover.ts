import type { EffectFrame, EffectRunner } from "./types";
import { clearInline, transition } from "./types";

/**
 * 覆盖：新页从右侧压上来，旧页同时往后退一小段，新页左缘带一条渐变投影。
 *
 * 旧页只退 14% 而不是跟着走满整宽：读者看到的是「新的一页盖在旧页上」，
 * 旧页退得太远就变成两页并排滑动，观感与 slide 没区别了。
 * 倒退（上一页）时角色互换：被盖住的那页滑回、阴影跟手，观感是「把盖住的页掀开」。
 */
const PROPS = ["transform", "opacity", "transition"] as const;
/** 旧页后退比例：只让一点点，保住「被盖住」的语义。 */
const RECEDE = 0.14;
/** 新页左缘投影宽度（px）。 */
const SHADOW_PX = 44;

export function createCoverEffect(frameOf: () => EffectFrame): EffectRunner {
  let shadow: HTMLElement | null = null;

  const ensureShadow = (): HTMLElement | null => {
    if (shadow?.isConnected) return shadow;
    const stage = frameOf().cur.parentElement;
    if (!stage) return null;
    shadow = document.createElement("div");
    shadow.className = "fx-cover-shadow";
    shadow.setAttribute("aria-hidden", "true");
    shadow.style.width = `${SHADOW_PX}px`;
    stage.appendChild(shadow);
    return shadow;
  };

  const place = (p: number, dir: number) => {
    const { cur, inc, box } = frameOf();
    const width = box.viewportW;
    const band = ensureShadow();
    if (dir > 0) {
      // 下一页：新页从右压入，旧页后退
      if (inc) inc.style.transform = `translate3d(${(1 - p) * width}px,0,0)`;
      cur.style.transform = `translate3d(${-p * width * RECEDE}px,0,0)`;
    } else {
      // 上一页：当前页（被掀开的那页）滑回右侧，底下的页不动
      cur.style.transform = `translate3d(${p * width}px,0,0)`;
      if (inc) inc.style.transform = "translate3d(0,0,0)";
    }
    if (!band) return;
    // 投影跟着「正在移动的那一页」的左缘走
    band.style.transform = `translate3d(${(dir > 0 ? (1 - p) * width : p * width).toFixed(1)}px,0,0)`;
    band.style.opacity = String(Math.min(1, (dir > 0 ? p : 1 - p) * 3));
  };

  return {
    name: "cover",
    apply(frame) {
      place(frame.p, frame.dir);
    },
    settle(_from, to, duration, dir, done) {
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
      shadow?.remove();
      shadow = null;
    },
  };
}
