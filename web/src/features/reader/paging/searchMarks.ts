import { useLayoutEffect } from "react";
import type { ReaderHit } from "../../../lib/readerSearch";
import { PG_MEASURE, PG_SOURCE } from "./PagedLayout";

/**
 * 分页正文的内容指纹：**只认章号**。
 *
 * 命中标记不进指纹——它们由 paintSearchHits 事后打点。指纹带上命中集合（数量或位置）时，
 * 读者每打一个字都会把整章排版源拆了重建：一次输入 2–3 帧的卡顿就是这么来的。
 * 换章仍然要重建，那是真的换了正文。
 */
export function pagingContentKey(chapterId: number): string {
  return `chapter-${chapterId}`;
}

/** 打点入参：命中集合、当前那一处、是否在搜。 */
export interface HitPaintInput {
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
}

/** 每个排版源已经打过的状态：结构指纹 + 当前命中，用来跳过无变化的重复打点。 */
const painted = new WeakMap<HTMLElement, { structure: string; current: number }>();

/**
 * 搜索命中事后打点：排版源按纯文本建好后，把 <mark> 补进**已存在的 DOM**。
 *
 * 为什么不在排版时带标记：mark 只画背景与描边（见 ix-reader-paging.css，没有内外边距、
 * 不改字体），对列宽与页高零影响，却会让「每打一个字」变成「重建整章排版」。拆成两步之后，
 * 命中集合变化只改动命中的那几个段落，排版、页数与锚点全部不动。
 *
 * 两层判定：结构（命中位置）变了才重插标记；只有「当前这一处」变时只切 is-current 的类，
 * 文本节点一个都不碰。返回本次真正改动过的排版源个数（判据与取证用）。
 *
 * 常驻测量源（.pg-measure）跳过：它隐形，标记画了没人看，每次输入还多动一份 DOM。
 */
export function paintSearchHits(stage: HTMLElement | null, input: HitPaintInput): number {
  if (!stage) return 0;
  const structure = input.searching ? hitsSignature(input.hitsByPara) : "";
  let changed = 0;
  stage.querySelectorAll<HTMLElement>(`.${PG_SOURCE}:not(.${PG_MEASURE})`).forEach((source) => {
    const done = painted.get(source);
    if (done && done.structure === structure && done.current === input.currentHit) return;
    if (done && done.structure === structure) markCurrent(source, input.currentHit);
    else rebuildMarks(source, input.searching ? input : null);
    painted.set(source, { structure, current: input.currentHit });
    changed += 1;
  });
  return changed;
}

/**
 * 命中结构指纹：段号 + 每处的位置与序号。
 * 用位置而不是「命中数量」——同为 3 处但换了位置时，只比数量会把旧位置的高亮留在正文上。
 */
export function hitsSignature(hitsByPara: Map<number, ReaderHit[]>): string {
  if (hitsByPara.size === 0) return "";
  const parts: string[] = [];
  hitsByPara.forEach((hits, para) => {
    parts.push(`${para}:${hits.map((hit) => `${hit.index}-${hit.start}-${hit.end}`).join(",")}`);
  });
  return parts.join("|");
}

/** 结构变了：逐段重插标记；input 为 null 表示没在搜，全部还原成纯文本。 */
function rebuildMarks(source: HTMLElement, input: HitPaintInput | null): void {
  source.querySelectorAll<HTMLElement>("[data-para]").forEach((para) => {
    const hits = input ? input.hitsByPara.get(Number(para.dataset.para)) ?? [] : [];
    paintPara(para, hits, input?.currentHit ?? -1);
  });
}

/**
 * 单段打点：取出原文 → 清空 → 有命中就按区间切开。
 * 无命中且当前也没有标记时直接跳过——绝大多数段落走这条路，一章里通常只有一两段有命中。
 */
function paintPara(para: HTMLElement, hits: ReaderHit[], currentHit: number): void {
  const hasMarks = para.querySelector("mark[data-hit]") !== null;
  if (!hasMarks && hits.length === 0) return;
  // 先取出原文再清空节点：mark 的 textContent 拼回来就是原文，不会丢字
  const text = para.textContent ?? "";
  para.replaceChildren();
  if (hits.length === 0) {
    if (text) para.appendChild(document.createTextNode(text));
    return;
  }
  appendHits(para, text, hits, currentHit);
}

/** 命中包 <mark>：与连续流 / chapterFragment 同一套标记（data-hit / is-current）。 */
function appendHits(para: HTMLElement, text: string, hits: ReaderHit[], currentHit: number): void {
  let from = 0;
  for (const hit of hits) {
    if (hit.start > from) para.appendChild(document.createTextNode(text.slice(from, hit.start)));
    const mark = document.createElement("mark");
    mark.dataset.hit = String(hit.index);
    if (hit.index === currentHit) mark.className = "is-current";
    mark.textContent = text.slice(hit.start, hit.end);
    para.appendChild(mark);
    from = hit.end;
  }
  if (from < text.length) para.appendChild(document.createTextNode(text.slice(from)));
}

/** 只换「当前这一处」：切类，不动文本节点（零重排）。 */
function markCurrent(root: ParentNode, currentHit: number): void {
  root.querySelectorAll<HTMLElement>("mark[data-hit]").forEach((mark) => {
    mark.classList.toggle("is-current", Number(mark.dataset.hit) === currentHit);
  });
}
/**
 * 打点时机：进章 / 排版源重建后、命中或当前处变化后、以及**翻页之后**。
 * 翻页会重建页层（克隆），新层里没有标记，所以 page 与 chapterId 都是依赖。
 */
export function usePaintSearchHits(
  stageEl: { current: HTMLElement | null },
  input: HitPaintInput & { chapterId: number; page: number },
): void {
  const { hitsByPara, currentHit, searching, page, chapterId } = input;
  useLayoutEffect(() => {
    paintSearchHits(stageEl.current, { hitsByPara, currentHit, searching });
  }, [stageEl, hitsByPara, currentHit, searching, page, chapterId]);
}
