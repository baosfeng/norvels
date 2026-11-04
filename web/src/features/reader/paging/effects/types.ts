import type { PageBox } from "../../../../lib/paging/pageGeometry";
import type { TurnDir } from "../../../../lib/paging/turnGesture";
import type { TurnEffect } from "../../../../lib/paging/turnEffects";

/** 一帧的上下文：两层 + 尺寸 + 进度。效果模块只读它，不碰引擎状态。 */
export interface EffectFrame {
  /** 0 = 当前页完整，1 = 完全翻走。 */
  p: number;
  dir: TurnDir;
  box: PageBox;
  /** 当前页（正在被翻走的那一页）的层。 */
  cur: HTMLElement;
  /** 目标页的层；越界时不存在。 */
  inc: HTMLElement | null;
}

/** 效果 runner：拖拽时逐帧写样式，松手后自己把剩余距离走完。 */
export interface EffectRunner {
  readonly name: TurnEffect;
  apply(frame: EffectFrame): void;
  /**
   * 收尾：把 p 从 from 推到 to，跑完调用 done。
   * 走逐帧还是 CSS transition 由效果自己定——fade 的顺序淡出、curl 的 clip-path
   * 都没法用单条 transition 表达；slide / cover 的纯位移交给合成器更省。
   */
  /** 返回取消函数：引擎在换章 / 卸载时用它把没跑完的收尾掐掉，避免旧回调把页码写回去。 */
  settle(from: number, to: 0 | 1, duration: number, dir: TurnDir, done: () => void): () => void;
  destroy(): void;
}

/** 建效果时拿到的宿主。 */
export interface EffectHost {
  stage: HTMLElement;
  /** 当前帧的来源函数：效果在收尾过程中也要读最新的一帧。 */
  frame: () => EffectFrame;
  /** 建一个额外层（挂在 stage 上，销毁由效果自己负责）。 */
  addLayer(className: string, tag?: "div" | "canvas"): HTMLElement;
}

/** 逐帧走完一段 0..1 的推进；duration 为 0 时直接到终态。返回取消函数。 */
export function animate(
  duration: number,
  onFrame: (eased: number) => void,
  done: () => void,
): () => void {
  if (!(duration > 0)) {
    onFrame(1);
    done();
    return () => {};
  }
  let raf = 0;
  const start = performance.now();
  const step = () => {
    const t = Math.min(1, (performance.now() - start) / duration);
    onFrame(1 - Math.pow(1 - t, 3));
    if (t < 1) raf = requestAnimationFrame(step);
    else done();
  };
  raf = requestAnimationFrame(step);
  return () => cancelAnimationFrame(raf);
}

/**
 * 给元素的指定属性加过渡并等它跑完。
 * transitionend 可能永远不来（值没变 / 被打断 / 元素被移除），所以带超时兜底。
 */
export function transition(
  el: HTMLElement,
  props: string,
  duration: number,
  apply: () => void,
  done: () => void,
): () => void {
  if (!(duration > 0)) {
    apply();
    done();
    return () => {};
  }
  let settled = false;
  const finish = () => {
    if (settled) return;
    settled = true;
    el.removeEventListener("transitionend", onEnd);
    el.style.removeProperty("transition");
    done();
  };
  const onEnd = (event: TransitionEvent) => {
    if (event.target === el) finish();
  };
  el.addEventListener("transitionend", onEnd);
  el.style.transition = `${props} ${duration}ms cubic-bezier(0.2, 0, 0, 1)`;
  apply();
  return finish;
}

/** 清掉逐帧写的内联值，免得与 transition 抢同一组属性。 */
export function clearInline(el: HTMLElement | null, props: readonly string[]): void {
  if (!el) return;
  for (const prop of props) el.style.removeProperty(prop);
}
