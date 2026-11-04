import type { PageBox } from "../../../lib/paging/pageGeometry";
import {
  BOUNDARY_REBOUND_MS,
  dragProgress,
  lockDrag,
  rubberOffset,
  settleDuration,
  shouldCommit,
  turnDirection,
  type DragLock,
  type TurnDir,
} from "../../../lib/paging/turnGesture";
import { effectDuration, type TurnEffect } from "../../../lib/paging/turnEffects";
import { createEffect } from "./effects";
import type { EffectFrame, EffectRunner } from "./effects/types";

export interface PageTurnOptions {
  stage: HTMLElement;
  createLayer: (page: number) => HTMLElement;
  setLayerPage: (layer: HTMLElement, page: number) => void;
  box: () => PageBox;
  effect: () => TurnEffect;
  reducedMotion: () => boolean;
  /** 翻到边界之外：调用方决定是提示还是跨章。 */
  onEdge: (dir: TurnDir) => void;
  /** 页码落定：调用方写游标 / 上报进度。 */
  onCommit: (page: number) => void;
}

interface DragState {
  x0: number;
  y0: number;
  lastX: number;
  lastT: number;
  v: number;
  lock: DragLock;
  p: number;
  dir: TurnDir;
}

const CUR_Z = "2";
const NEXT_Z = "1";

/**
 * 翻页引擎：手势 → 逐帧 p → 提交或回弹。
 *
 * 双层模型：当前页（.fx-cur）+ 目标页（.fx-next，只在翻页过程中存在）。
 * 拖动中逐帧写内联样式（跟手）；松手把剩余距离交给效果收尾，收尾**完成之后**才提交页码并
 * 销毁第二层——提前改页码会让页面和 DOM 对不上，出现「翻到一半又跳回去」。
 */
export class PageTurnEngine {
  private readonly opts: PageTurnOptions;
  private cur: HTMLElement | null = null;
  private inc: HTMLElement | null = null;
  private runner: EffectRunner | null = null;
  private cancelSettle: (() => void) | null = null;
  private drag: DragState | null = null;
  private page = 0;
  private total = 1;
  private dir: TurnDir = 1;

  constructor(opts: PageTurnOptions) {
    this.opts = opts;
  }

  get currentPage(): number {
    return this.page;
  }

  get busy(): boolean {
    return this.cancelSettle !== null;
  }

  /** 换章 / 初始挂载：重置成单层。 */
  reset(page: number, total: number): void {
    this.page = clampPage(page, total);
    this.total = total;
    this.clearAll();
    this.cur = this.opts.createLayer(this.page);
    this.cur.style.zIndex = CUR_Z;
  }

  /** 只换页码（重排后同一段落到新页）：改克隆的位移，不重建 DOM。 */
  relocate(page: number, total: number): void {
    this.total = total;
    this.page = clampPage(page, total);
    if (this.cur) this.opts.setLayerPage(this.cur, this.page);
  }

  destroy(): void {
    this.clearAll();
    this.opts.stage.querySelectorAll(".pg-layer").forEach((node) => node.remove());
  }

  pointerDown(x: number, y: number): void {
    if (this.cancelSettle) return;
    this.drag = { x0: x, y0: y, lastX: x, lastT: performance.now(), v: 0, lock: "undecided", p: 0, dir: 1 };
  }

  /** 返回是否已接管（横向锁定）；纵向由调用方交回滚动。 */
  pointerMove(x: number, y: number): boolean {
    const drag = this.drag;
    if (!drag) return false;
    const dx = x - drag.x0;
    if (drag.lock === "undecided") {
      drag.lock = lockDrag(dx, y - drag.y0);
      if (drag.lock === "horizontal") {
        drag.dir = turnDirection(dx);
        this.begin(drag.dir);
      } else if (drag.lock === "vertical") {
        return false;
      }
    }
    if (drag.lock !== "horizontal") return false;
    const now = performance.now();
    const dt = now - drag.lastT;
    if (dt > 0) drag.v = (x - drag.lastX) / dt;
    drag.lastX = x;
    drag.lastT = now;
    drag.p = dragProgress(dx, this.opts.box().viewportW);
    if (this.inc) this.runner?.apply(this.frame(drag.p, drag.dir));
    else if (this.opts.effect() !== "none") this.applyBoundary(dx);
    return true;
  }

  /** 松手：翻页 / 回弹 / 越界三种去向。返回当前页码（提交在收尾结束后才生效）。 */
  pointerUp(x: number): number {
    const drag = this.drag;
    this.drag = null;
    if (!drag || drag.lock !== "horizontal") {
      this.clearPair();
      return this.page;
    }
    const dx = x - drag.x0;
    const commit = shouldCommit(drag.p, drag.v, dx);
    if (!this.inc) {
      // 没有第二层只有两种来由：无动画效果（不跟手，松手直接换页）或越界
      if (this.opts.effect() === "none" && this.canTurn(drag.dir)) {
        if (commit) this.land(this.page + drag.dir);
      } else {
        this.reboundBoundary(dx);
        this.opts.onEdge(drag.dir);
      }
      return this.page;
    }
    this.run(commit ? 1 : 0, drag.dir, drag.p);
    return this.page;
  }

  pointerCancel(): void {
    const from = this.drag?.p ?? 1;
    this.drag = null;
    this.run(0, this.dir, from);
  }

  /** 点按 / 键盘 / 自动翻页：没有手势，跑一次完整收尾。返回是否真的翻了。 */
  turn(dir: TurnDir): boolean {
    if (this.cancelSettle) return false;
    if (!this.canTurn(dir)) {
      this.opts.onEdge(dir);
      return false;
    }
    if (this.opts.effect() === "none") {
      this.land(this.page + dir);
      return true;
    }
    this.dir = dir;
    this.begin(dir);
    if (!this.inc) return false;
    this.run(1, dir, 0);
    return true;
  }

  /** 目标页是否在本章范围内。 */
  canTurn(dir: TurnDir): boolean {
    const target = this.page + dir;
    return target >= 0 && target <= this.total - 1;
  }

  private frame(p: number, dir: TurnDir): EffectFrame {
    return { p, dir, box: this.opts.box(), cur: this.cur as HTMLElement, inc: this.inc };
  }

  private begin(dir: TurnDir) {
    if (this.inc || !this.canTurn(dir) || !this.cur) return;
    // 无动画效果不跟手：拖动过程中不建第二层，松手才换页（省掉一次整页克隆）
    if (this.opts.effect() === "none") return;
    this.dir = dir;
    const target = this.page + dir;
    this.inc = this.opts.createLayer(target);
    this.inc.style.zIndex = NEXT_Z;
    this.opts.setLayerPage(this.inc, target);
    // 只在翻页这几百毫秒里提升为合成层：常驻 will-change 会一直占显存
    this.cur.classList.add("is-turning");
    this.inc.classList.add("is-turning");
    const frameOf = () => this.frame(this.drag?.p ?? 0, this.dir);
    this.runner?.destroy();
    // 效果在「开始这一次翻页」时定下来：设置面板里换效果只影响下一次翻页，
    // 当前这一页不会被重画，也就不会闪。
    this.runner = createEffect(this.opts.effect(), frameOf);
    this.runner.apply(frameOf());
  }

  private run(to: 0 | 1, dir: TurnDir, from: number) {
    const runner = this.runner;
    if (!runner) {
      if (to === 1) this.land(this.page + dir);
      return;
    }
    const duration = settleDuration(effectDuration(this.opts.effect(), this.opts.reducedMotion()), from, to);
    const cancel = runner.settle(from, to, duration, dir, () => {
      this.cancelSettle = null;
      if (to === 1) this.land(this.page + dir);
      else this.clearPair();
    });
    this.cancelSettle = cancel;
  }

  /**
   * 落定新页：销毁双层、重建单层、通知调用方。
   *
   * 换层会重建 DOM：如果焦点原本在舞台里（键盘翻页时就是），节点一换焦点就掉到 body，
   * 读者的下一次按键会打到页面而不是阅读器。这里在重建后把焦点接回来——
   * 只在焦点本来就落在舞台里时补，不抢别人（搜索框 / 设置面板）的焦点。
   */
  private land(page: number) {
    const hadFocus = this.opts.stage.contains(document.activeElement);
    this.clearAll();
    this.page = clampPage(page, this.total);
    this.cur = this.opts.createLayer(this.page);
    this.cur.style.zIndex = CUR_Z;
    if (hadFocus) {
      const host = this.opts.stage.closest<HTMLElement>('[tabindex]');
      host?.focus({ preventScroll: true });
    }
    this.opts.onCommit(this.page);
  }

  private applyBoundary(dx: number) {
    if (!this.cur) return;
    this.cur.style.transform = `translate3d(${rubberOffset(dx, this.opts.box().viewportW).toFixed(2)}px,0,0)`;
  }

  private reboundBoundary(dx: number) {
    const cur = this.cur;
    if (!cur) return;
    const from = rubberOffset(dx, this.opts.box().viewportW);
    const start = performance.now();
    const step = () => {
      const t = Math.min(1, (performance.now() - start) / BOUNDARY_REBOUND_MS);
      const ease = 1 - Math.pow(1 - t, 3);
      cur.style.transform = `translate3d(${(from * (1 - ease)).toFixed(2)}px,0,0)`;
      if (t < 1) requestAnimationFrame(step);
      else cur.style.removeProperty("transform");
    };
    requestAnimationFrame(step);
  }

  private clearPair() {
    this.runner?.destroy();
    this.runner = null;
    this.inc?.remove();
    this.inc = null;
    this.cur?.classList.remove("is-turning");
    this.cur?.style.removeProperty("transform");
  }

  /** 清空双层：旧的页层节点必须一并摘掉，否则留在舞台上的旧层会盖住新层。 */
  private clearAll() {
    this.cancelSettle?.();
    this.cancelSettle = null;
    this.clearPair();
    this.cur = null;
    this.opts.stage.querySelectorAll(`.pg-layer`).forEach((node) => node.remove());
  }
}

function clampPage(page: number, total: number): number {
  const last = Math.max(0, total - 1);
  if (!Number.isFinite(page)) return 0;
  return Math.min(last, Math.max(0, page));
}
