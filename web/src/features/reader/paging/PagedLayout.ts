import {
  countPages,
  computePageBox,
  EMPTY_BOX,
  isEmptyBox,
  pageOfOffset,
  translateForPage,
  type PageBox,
} from "../../../lib/paging/pageGeometry";
import type { ParaBox } from "../../../lib/paging/pageAnchor";

/** 页层与测量源共用的类名（样式在 styles/ix-reader-paging.css）。 */
export const PG_SOURCE = "pg-source";
export const PG_MEASURE = "pg-measure";
export const PG_LAYER = "pg-layer";
export const PG_WINDOW = "pg-window";

/**
 * 分页排版：多栏排版源 + 页层裁剪窗口。
 *
 *   .pg-stage                     视口（尺寸 = 一页的可视区）
 *     .pg-source.pg-measure       常驻、隐形、不参与渲染：页数与段落位置的唯一真值
 *     .pg-layer > .pg-window > .pg-source(clone)   一页 = 一份排版源 + 负向 translateX
 *
 * 两条硬要求（原型实测踩出来）：
 * 1. 排版源不能 overflow:hidden——列是横向溢出排布的，一裁就只剩第一列，
 *    translateX 永远翻不出后面的页。裁剪只能在 .pg-window 上做。
 * 2. 尺寸一律用 offsetWidth/offsetHeight，不用 getBoundingClientRect 量容器。
 */
export class PagedLayout {
  readonly stage: HTMLElement;
  private readonly padX: number;
  private content: () => DocumentFragment;
  private measureNode: HTMLElement | null = null;
  private box: PageBox = EMPTY_BOX;
  private paraBoxes: ParaBox[] = [];
  private pages = 1;

  constructor(stage: HTMLElement, content: () => DocumentFragment, padX?: number) {
    this.stage = stage;
    this.content = content;
    // 版心留白从样式变量读：改 CSS 就跟着变，不在 JS 里复述一份数字
    this.padX = padX ?? readVar(stage, "--pg-pad-x", 18);
    this.ensureMeasure();
  }

  /** 换章 / 搜索标记变化：排版源内容变了，页数与锚点都要重新量。 */
  setContent(content: () => DocumentFragment): void {
    this.content = content;
    this.fillMeasure();
  }

  get pageBox(): PageBox {
    return this.box;
  }

  get pageCount(): number {
    return this.pages;
  }

  get anchors(): readonly ParaBox[] {
    return this.paraBoxes;
  }

  /**
   * 量尺寸并写 CSS 变量。行高从真实渲染值读（探针），不在 JS 里复述一份字号 / 行距——
   * 读者在设置面板改字号，这里跟着变。
   */
  measure(): PageBox {
    const fontSize = probe(this.stage, "fontSize");
    const lineH = probe(this.stage, "lineHeight", fontSize);
    this.box = computePageBox({
      stageW: this.stage.offsetWidth,
      stageH: this.stage.offsetHeight,
      padX: this.padX,
      lineH,
    });
    const style = this.stage.style;
    style.setProperty("--pg-w", `${this.box.pageW}px`);
    style.setProperty("--pg-h", `${this.box.pageH}px`);
    style.setProperty("--pg-top", `${this.box.padY}px`);
    style.setProperty("--pad-x", `${this.box.padX}px`);
    return this.box;
  }

  /** 量页数与每个段落的列位置；尺寸不可用（隐藏 / 还没布局）时返回 null，调用方保持旧值。 */
  measureContent(box: PageBox = this.box): { pageCount: number; anchors: ParaBox[] } | null {
    const measure = this.measureNode;
    if (!measure || isEmptyBox(box)) return null;
    const pages = countPages(measure.scrollWidth, box.pageW);
    const base = measure.getBoundingClientRect().left;
    const anchors: ParaBox[] = [];
    measure.querySelectorAll<HTMLElement>("[data-para]").forEach((node) => {
      const pi = Number(node.dataset.para);
      if (!Number.isFinite(pi)) return;
      anchors.push({ pi, left: node.getBoundingClientRect().left - base });
    });
    this.pages = pages;
    this.paraBoxes = anchors.sort((a, b) => a.pi - b.pi);
    return { pageCount: pages, anchors: this.paraBoxes };
  }

  /** 段号 → 页码（重排后按锚点回到原文位置）。 */
  pageOfPara(pi: number): number | null {
    const box = this.paraBoxes.find((item) => item.pi === pi);
    return box ? pageOfOffset(box.left, this.box.pageW) : null;
  }

  /** 建一页：裁剪窗口 + 排版源克隆 + 定位到第 N 列。 */
  createLayer(page: number): HTMLElement {
    const layer = document.createElement("div");
    layer.className = `${PG_LAYER} fx-cur`;
    layer.appendChild(this.createWindow());
    this.stage.appendChild(layer);
    // 建完立刻定位：漏了这步，页层永远显示第 1 列（原型里也踩过同一个坑）
    this.setLayerPage(layer, page);
    return layer;
  }

  /** 换页：只改克隆的位移，DOM 不动（换页零重排）。 */
  setLayerPage(layer: HTMLElement, page: number): void {
    layer.dataset.page = String(page);
    const src = layer.querySelector<HTMLElement>(`.${PG_SOURCE}`);
    if (src) src.style.transform = `translateX(${translateForPage(page, this.box.pageW)}px)`;
  }

  destroy(): void {
    this.measureNode?.remove();
    this.measureNode = null;
  }

  /** 常驻测量源：隐形、不参与渲染与手势，专门算页数与锚点。 */
  private ensureMeasure(): void {
    if (this.measureNode) return;
    const node = document.createElement("div");
    node.className = `${PG_SOURCE} ${PG_MEASURE}`;
    node.setAttribute("aria-hidden", "true");
    this.stage.appendChild(node);
    this.measureNode = node;
    this.fillMeasure();
  }

  private fillMeasure(): void {
    if (this.measureNode) this.measureNode.replaceChildren(this.content());
  }

  private createWindow(): HTMLElement {
    const win = document.createElement("div");
    win.className = PG_WINDOW;
    const src = document.createElement("div");
    src.className = PG_SOURCE;
    src.appendChild(this.content());
    win.appendChild(src);
    return win;
  }
}

/** 读一个 px 数值的 CSS 变量（缺省 / 脏值时用 fallback）。 */
function readVar(el: HTMLElement, name: string, fallback: number): number {
  const raw = getComputedStyle(el).getPropertyValue(name).trim();
  const value = Number.parseFloat(raw);
  return Number.isFinite(value) && value >= 0 ? value : fallback;
}

/**
 * 读排版源的真实字号 / 行高：探针插在 stage 里，继承同一份 --reader-font 与 --reader-leading。
 * 用探针而不是 getComputedStyle(stage)：stage 自己不是文字容器，问它拿到的是继承值。
 */
function probe(stage: HTMLElement, prop: "fontSize" | "lineHeight", fontSize = 0): number {
  const el = document.createElement("p");
  el.className = PG_SOURCE;
  el.style.cssText = "position:absolute;visibility:hidden;left:0;top:0;width:auto;height:auto;margin:0";
  el.textContent = "字";
  stage.appendChild(el);
  const computed = getComputedStyle(el);
  const raw = prop === "fontSize" ? computed.fontSize : computed.lineHeight;
  el.remove();
  const value = Number.parseFloat(raw);
  if (Number.isFinite(value) && raw !== "normal") return value;
  // line-height:normal 时浏览器按字体 metrics 算，中文栈多数落在 1.15em 附近
  return prop === "lineHeight" && fontSize > 0 ? Math.round(fontSize * 1.15) : value || 0;
}
