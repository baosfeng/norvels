import { useEffect, useRef } from "react";
import type { ReaderSettings } from "./readerSettings";

type Anchor = { chapterId: number; para: number };

/** 正文滚动容器（与 useReaderScrollPercent / dismissHint 取的是同一个节点）。 */
const SURFACE_SELECTOR = '[data-testid="reader-swipe-area"]';

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

/** 量一段相对正文内容起点的位置与高度（视口矩形 + scrollTop 换算，含容器内边距）。 */
function measure(surface: HTMLElement, node: HTMLElement) {
  const rect = node.getBoundingClientRect();
  return {
    docTop: rect.top - surface.getBoundingClientRect().top + surface.scrollTop,
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
    const surface = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
    if (!surface) return;
    const track = () => {
      const node = document.getElementById(`para-${para}`);
      if (!node) return;
      const at = measure(surface, node);
      lastBox.current = { ...at, ratio: anchorRatio(surface.scrollTop, at.docTop, at.height) };
    };
    track();
    surface.addEventListener("scroll", track, { passive: true });
    return () => surface.removeEventListener("scroll", track);
  }, [para]);

  useEffect(() => {
    const surface = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
    const node = document.getElementById(`para-${para}`);
    const reflowed = lastSignature.current !== null && lastSignature.current !== signature;
    const chapterChanged = lastChapter.current !== chapterDataId;
    const sameChapter = chapterDataId !== undefined && anchor.chapterId === chapterDataId;
    const box = lastBox.current;

    lastSignature.current = signature;
    lastChapter.current = chapterDataId;
    if (!surface || !node) return;

    // 此刻 DOM 已重排、scrollTop 还是旧值：量到的是新布局，换算用的是旧占比
    const next = measure(surface, node);
    if (reflowed && !chapterChanged && sameChapter && box) {
      surface.scrollTop = scrollTopForAnchor(box, next);
    }
    // 落点定下来再按实际位置记一遍：下次变化从新位置起算，旧占比不会被重复套用；
    // scrollTop 被夹在 0 或章末时，记下的也是实际读到的位置，不是那个够不着的理想值
    const settled = measure(surface, node);
    lastBox.current = { ...settled, ratio: anchorRatio(surface.scrollTop, settled.docTop, settled.height) };
  }, [signature, chapterDataId, para, anchor]);
}
