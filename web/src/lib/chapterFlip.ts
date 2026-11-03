import { prefersReducedMotion } from "./theme";

/**
 * 显式翻章的翻页转场：旧页滑出、新页滑入。
 *
 * 阅读器的正文是一条连续的章节流（见 useChapterStream），滚动跨章是无缝的；
 * 但读者「滑 / 点 / 按键」翻章时，屏幕内容会整屏换掉——没有过程就成了瞬移。
 * 这里用同文档视图转场（View Transition）把换章前后各拍一帧，两帧之间做滑动：
 * 旧页带着手势位移滑出去，新页从另一侧滑进来，读者看到的是「翻过去」，不是「跳过去」。
 *
 * 四条约定：
 * - 只包显式翻章（flipTo）。滚动跨章不经过这里，连续流原样保留。
 * - 换章与落位发生在 apply 里；落位完成后调用 settleChapterFlip()，浏览器才拍新位置。
 *   落位迟迟不上报（章还没到）时用 FLIP_FALLBACK_MS 兜底，页面不会锁死在快照里。
 * - 每次转场有自己的序号：被浏览器 skip 的旧转场不许删掉新转场的方向标记。
 * - 不支持转场的浏览器 / 系统要求减少动效时直接切换：功能一致，只是没有过程。
 */

/** 翻章方向：决定旧页往哪边滑出。 */
export type FlipDirection = "prev" | "next";

/** 手势层：跟手位移写在这一层——旧快照要带着它，新快照必须从原位开始。 */
const SWIPE_LAYER_SELECTOR = ".reader-swipe-layer";

/** 入场淡入的章节：转场拍照前必须把它们定住，否则一页半透明、转场后还会重播。 */
const ENTERING_SELECTOR = ".reader-article.is-entering";

/** 落位兜底（ms）：超过它就按「已经落位」拍照，动画不会卡住。 */
const FLIP_FALLBACK_MS = 600;

type ViewTransitionLike = { finished: Promise<void>; skipTransition?: () => void };
type DocumentWithViewTransition = Document & {
  startViewTransition?: (update: () => Promise<void>) => ViewTransitionLike;
};

/** 转场序号：只有最新一次翻章能改方向标记。 */
let flipSeq = 0;

/** 正在等落位的那次转场：apply 之后由 settleChapterFlip 放行。 */
let settlePending: (() => void) | null = null;

/** 还在跑的转场：连翻时必须先结束它，否则浏览器会 skip 新转场、旧方向留在屏幕上。 */
let activeTransition: ViewTransitionLike | null = null;

/** 这个浏览器能不能做快照转场（不支持就退回瞬时切换）。 */
export function canFlipWithSnapshot(): boolean {
  if (typeof document === "undefined") return false;
  return typeof (document as DocumentWithViewTransition).startViewTransition === "function";
}

/**
 * 清掉手势层的跟手位移。
 * 先于 apply 执行：旧快照已经拍下「手指拖到哪」的那一刻，新页必须从原位开始，
 * 否则读者松手后整页会歪着落定。
 */
function clearSwipeOffset(): void {
  const layer = document.querySelector<HTMLElement>(SWIPE_LAYER_SELECTOR);
  if (!layer) return;
  layer.style.transition = "none";
  layer.style.transform = "";
}

/**
 * 定住入场淡入。
 *
 * 翻章时若目标章是这次才挂载的，它带着 chapterFade（从 opacity 0 起）：不取消的话
 * 新快照会拍到半透明的一页；而按 class 临时压住动画，转场一结束动画又会从头再播
 * ——读者看到的就是「翻完白一下」。这里直接给已存在的节点写内联 animation: none：
 * 只作用于这一批节点，之后新挂载的章照常淡入。
 */
function freezeEnteringArticles(): void {
  document.querySelectorAll<HTMLElement>(ENTERING_SELECTOR).forEach((el) => {
    el.style.animation = "none";
  });
}

/** 起一次翻章转场：apply 只负责换章与落位，动画交给 CSS（见 styles/motion.css）。 */
export function startChapterFlip(direction: FlipDirection, apply: () => void): void {
  const doc = document as DocumentWithViewTransition;
  if (!canFlipWithSnapshot() || !doc.startViewTransition || prefersReducedMotion()) {
    clearSwipeOffset();
    apply();
    return;
  }
  // 上一次转场还没播完就又翻章：先把它收掉，下一次的快照才按新方向拍
  activeTransition?.skipTransition?.();
  const id = ++flipSeq;
  const root = document.documentElement;
  root.dataset.readerFlip = direction;
  // 方向走自定义属性：view-transition 伪元素只能继承 root 的变量，条件选择器套不上它
  root.style.setProperty("--reader-flip-sign", direction === "prev" ? "-1" : "1");
  const transition = doc.startViewTransition(
    () =>
      new Promise<void>((resolve) => {
        let settled = false;
        let timer = 0;
        const settle = () => {
          if (settled) return;
          settled = true;
          if (flipSeq === id) settlePending = null;
          window.clearTimeout(timer);
          freezeEnteringArticles();
          resolve();
        };
        timer = window.setTimeout(settle, FLIP_FALLBACK_MS);
        settlePending = settle;
        clearSwipeOffset();
        apply();
      }),
  );
  activeTransition = transition;
  void transition.finished.finally(() => {
    if (activeTransition === transition) activeTransition = null;
    // 被 skip 的旧转场也会走到这里：它不许动后来那次翻章的方向
    if (flipSeq !== id) return;
    delete root.dataset.readerFlip;
    root.style.removeProperty("--reader-flip-sign");
  });
}

/** 落位完成：放行这次转场，让浏览器拍下新位置。没在等转场时是空操作。 */
export function settleChapterFlip(): void {
  settlePending?.();
}
