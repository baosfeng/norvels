import { useEffect, useRef } from "react";
import type { ReaderSettings } from "./readerSettings";
import { pageScrollTop, readerSurface, setScrollTop, type Surface } from "./readerStreamLayout";

type Anchor = { chapterId: number; para: number };

/** 参与排版的设置项：任一项变化正文都会重排，之前的像素滚动位置随即失效。 */
const REFLOW_KEYS = ["font", "leading", "fontFamily", "para", "width", "align", "indent", "followSystemFont", "script"] as const;

/** 排版指纹：把参与重排的设置拼成一串，直接比对就知道「这次变化会不会重排」。 */
export function reflowSignature(settings: ReaderSettings): string {
  return REFLOW_KEYS.map((key) => String(settings[key])).join("|");
}

/** 重排前记下的阅读位置：锚点段顶端在正文里的位置、段高，以及读者读到了这一段的几成。 */
export interface ReflowAnchorBox {
  docTop: number;
  height: number;
  ratio: number;
}

/** 读者读到锚点段的几成：段首在屏幕顶是 0，段尾在屏幕顶是 1。 */
export function anchorRatio(scrollTop: number, docTop: number, height: number): number {
  // 段高退化成 0（还没布局出来）时没有位置信息可言，一律落回段首
  if (!(height > 0)) return 0;
  const ratio = (scrollTop - docTop) / height;
  if (!Number.isFinite(ratio)) return 0;
  return Math.min(1, Math.max(0, ratio));
}

/**
 * 重排后的落点：同一段里同样的「几成」对准屏幕顶。
 * 字号一变，这段的顶端与高度都换了；只按占比换算才落回读者刚读到的那一行，
 * 死守像素 scrollTop 会把他甩到别处（390×844 实测 54% → 18% / 100%）。
 */
export function scrollTopForAnchor(box: ReflowAnchorBox, next: { docTop: number; height: number }): number {
  return next.docTop + box.ratio * next.height;
}

/**
 * 滚动根：正文容器（H5 自己滚）或整页（PC，正文容器不滚）。
 *
 * 量位置、听 scroll、写回位置三处必须认同一个根。只认容器时，PC 上读到的
 * `surface.scrollTop` 恒为 0，docTop 却是容器坐标——算出的占比没有意义，
 * 重排后按它落点会把读者甩走（1280×800 实测 20→28px 可见段 −154~−189）。
 * 可滚性判定与赋值同样复用 `readerStreamLayout`：那里已经为「容器坐标与窗口坐标
 * 混用」付过一次代价（见 contentOffset 注释），两处各写一份必然再次分叉。
 */
export interface ScrollRoot {
  /** 当前滚动位置。 */
  get(): number;
  /** 段落顶端在滚动内容里的位置。 */
  offsetOf(node: HTMLElement): number;
  /** 把滚动位置设过去（直接赋值，不走平滑滚动）。 */
  set(value: number): void;
}

/**
 * 每次现取：挂载瞬间内容还没铺满，容器可能先不可滚、一秒后才可滚。
 *
 * 分页模式不经过这里：那边正文是页层克隆、段落 id 只挂在测量源上（见 chapterFragment），
 * 本 hook 取不到 para 节点就什么都不做——位置真值是页码，保位由 usePagedChapter 负责。
 */
export function scrollRoot(): ScrollRoot | null {
  const surface = readerSurface();
  if (!surface) return null;
  return surface.scrolls ? containerRoot(surface) : pageRoot();
}

function containerRoot(surface: Surface): ScrollRoot {
  const el = surface.el;
  return {
    get: () => el.scrollTop,
    offsetOf: (node) => node.getBoundingClientRect().top - el.getBoundingClientRect().top + el.scrollTop,
    set: (value) => setScrollTop(surface, value),
  };
}

function pageRoot(): ScrollRoot {
  return {
    get: pageScrollTop,
    // 整页滚时只有视口顶固定：正文容器自己也随页面一起跑，拿它的 rect 当基准算出来的是
    // 这一章在流里的偏移，再叠上 scrollY 就会落到别处（同 scrollToChapterTop 的教训）。
    offsetOf: (node) => node.getBoundingClientRect().top + pageScrollTop(),
    set: (value) => window.scrollTo(0, Math.max(0, value)),
  };
}

/**
 * 两处都听：滚动根会随可滚性变，挂错一次就再也不会跟踪。
 * 同一时刻只有一处真的产生 scroll 事件（另一处根本不滚），重复监听不带来重复刷新。
 */
function observeScroll(track: () => void): () => void {
  window.addEventListener("scroll", track, { passive: true });
  const el = readerSurface()?.el;
  el?.addEventListener("scroll", track, { passive: true });
  return () => {
    window.removeEventListener("scroll", track);
    el?.removeEventListener("scroll", track);
  };
}

/** 量一段相对滚动内容起点的位置与高度（视口矩形 + 滚动位置换算，含容器内边距）。 */
function measure(root: ScrollRoot, node: HTMLElement) {
  const rect = node.getBoundingClientRect();
  return {
    docTop: root.offsetOf(node),
    height: rect.height > 0 ? rect.height : 1,
  };
}

/**
 * 重排后按段落锚点回到原位。
 *
 * 判据是「改字号前后可见首段与段内位置不变」，不是「进度百分比不变」——百分比由章节几何
 * 派生，段落粒度粗（本章 4–6 段、单段 342px，32px 字号时 851px）时它必然跟着变：
 * 实测 20→32 是 54%→39%，32→16 是 →77%，而读者两次都停在同一行。
 * 想让百分比也不变，只能把读者甩到别处，正是这段代码要修的那个 bug。
 *
 * anchor 由 useReaderVisiblePara 持续跟踪，是「重排前」的事实：滚动时记下它那段的位置、
 * 段高与读者读到的几成；字号 / 行距 / 段距 / 字体 / 页宽 / 对齐一变，正文重排完就用
 * 同一段的同样几成重新算 scrollTop——直接赋值，不走平滑滚动，读者看不出一跳。
 * 锚点不在当前章时不处理（跨章由切章自己的恢复逻辑接管），首次挂载也不动。
 */
export function useReaderRepositionAfterReflow({
  settings,
  chapterDataId,
  anchor,
}: {
  settings: ReaderSettings;
  chapterDataId: number | undefined;
  anchor: Anchor;
}) {
  const signature = reflowSignature(settings);
  const lastSignature = useRef<string | null>(null);
  const lastChapter = useRef<number | undefined>(undefined);
  const lastBox = useRef<ReflowAnchorBox | null>(null);
  const para = anchor.para;

  // 滚动时刷新阅读位置：读者停在哪，「重排前」记下的就是哪。
  // 只跟 para 走——anchor 对象每次上报都是新的，跟着它会把这份记录反复重置。
  useEffect(() => {
    const track = () => {
      const root = scrollRoot();
      const node = document.getElementById(`para-${para}`);
      if (!root || !node) return;
      const at = measure(root, node);
      lastBox.current = { ...at, ratio: anchorRatio(root.get(), at.docTop, at.height) };
    };
    track();
    return observeScroll(track);
  }, [para]);

  useEffect(() => {
    const root = scrollRoot();
    const node = document.getElementById(`para-${para}`);
    const reflowed = lastSignature.current !== null && lastSignature.current !== signature;
    const chapterChanged = lastChapter.current !== chapterDataId;
    const sameChapter = chapterDataId !== undefined && anchor.chapterId === chapterDataId;
    const box = lastBox.current;

    lastSignature.current = signature;
    lastChapter.current = chapterDataId;
    if (!root || !node) return;

    // 此刻 DOM 已重排、滚动位置还是旧值：量到的是新布局，换算用的是旧占比
    const next = measure(root, node);
    if (reflowed && !chapterChanged && sameChapter && box) {
      root.set(scrollTopForAnchor(box, next));
    }
    // 落点定下来再按实际位置记一遍：下次变化从新位置起算，旧占比不会被重复套用；
    // 滚动被夹在 0 或章末时，记下的也是实际读到的位置，不是那个够不着的理想值
    const settled = measure(root, node);
    lastBox.current = { ...settled, ratio: anchorRatio(root.get(), settled.docTop, settled.height) };
  }, [signature, chapterDataId, para, anchor]);
}
