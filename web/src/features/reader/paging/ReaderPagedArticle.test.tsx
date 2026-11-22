import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReaderHit } from "../../../lib/readerSearch";
import { READER_DEFAULTS } from "../readerSettings";
import { PagedLayout, PG_MEASURE, PG_SOURCE } from "./PagedLayout";
import { ReaderPagedArticle } from "./ReaderPagedArticle";
import { hitsSignature, pagingContentKey, paintSearchHits } from "./searchMarks";

/** 12 段正文：够造多段落命中，也够验「不整章重建」。 */
const PARAGRAPHS = Array.from({ length: 12 }, (_, i) => `第 ${i} 段正文，雨从这里落下。`);

function hitsOf(para: number, count: number, from = 0): ReaderHit[] {
  return Array.from({ length: count }, (_, k) => ({
    index: k,
    paraIndex: para,
    start: from + k * 10,
    end: from + k * 10 + 1,
  }));
}

function mapOf(entries: Array<[number, ReaderHit[]]>): Map<number, ReaderHit[]> {
  return new Map(entries);
}

/**
 * 分页舞台的最小 DOM：常驻测量源 + 一页（.pg-layer > .pg-window > .pg-source 克隆）。
 * 与 PagedLayout 建出来的结构一致——打点只认 .pg-source，识别靠这个结构。
 */
function buildStage(): { stage: HTMLElement; measure: HTMLElement; layer: HTMLElement } {
  const stage = document.createElement("div");
  stage.className = "pg-stage";
  const measure = document.createElement("div");
  measure.className = `${PG_SOURCE} ${PG_MEASURE}`;
  const layer = document.createElement("div");
  layer.className = "pg-layer";
  const win = document.createElement("div");
  win.className = "pg-window";
  const source = document.createElement("div");
  source.className = PG_SOURCE;
  for (const [i, text] of PARAGRAPHS.entries()) {
    const p = document.createElement("p");
    p.dataset.para = String(i);
    p.textContent = text;
    source.appendChild(p);
    const measurePara = p.cloneNode(true);
    measure.appendChild(measurePara);
  }
  win.appendChild(source);
  layer.appendChild(win);
  stage.append(measure, layer);
  document.body.appendChild(stage);
  return { stage, measure, layer };
}

function pageSource(layer: HTMLElement): HTMLElement {
  return layer.querySelector<HTMLElement>(`.${PG_SOURCE}`) as HTMLElement;
}

describe("U19 章内搜索：命中标记事后打点，不重建排版", () => {
  beforeEach(() => {
    document.body.replaceChildren();
  });

  it("contentKey 只认章号：命中集合 / 当前命中怎么变都不变", () => {
    const key = pagingContentKey(42);
    expect(pagingContentKey(42)).toBe(key);
    // 命中集合变了、当前命中变了，指纹仍是同一个——它压根不接收这些参数
    expect(hitsSignature(mapOf([[0, hitsOf(0, 1)]]))).not.toBe(hitsSignature(mapOf([[0, hitsOf(0, 3)]])));
    expect(pagingContentKey(42)).toBe("chapter-42");
    expect(pagingContentKey(43)).not.toBe(key);
  });

  it("命中集合变化：只补 <mark>，段落节点与文本都不重建", () => {
    const { stage, layer } = buildStage();
    const source = pageSource(layer);
    const para = source.querySelector<HTMLElement>('[data-para="1"]') as HTMLElement;
    const textNode = para.firstChild;

    const changed = paintSearchHits(stage, {
      hitsByPara: mapOf([[1, hitsOf(1, 2)]]),
      currentHit: 0,
      searching: true,
    });

    // 只改到页层那一份排版源；常驻测量源（.pg-measure）隐形，不参与打点
    expect(changed).toBe(1);
    expect(para.querySelectorAll("mark[data-hit]")).toHaveLength(2);
    expect(para.querySelector("mark.is-current")).not.toBeNull();
    expect(para.textContent).toBe(PARAGRAPHS[1]);
    // 排版源与段落节点都没被换掉（换掉 = 整章重排）
    expect(pageSource(layer)).toBe(source);
    expect(source.querySelector('[data-para="1"]')).toBe(para);
    expect(textNode?.isConnected).toBe(false);
  });

  it("命中结构不变、只换当前那一处：只切类，不动任何文本节点", () => {
    const { stage, layer } = buildStage();
    const hits = mapOf([[2, hitsOf(2, 3)]]);
    paintSearchHits(stage, { hitsByPara: hits, currentHit: 0, searching: true });
    const source = pageSource(layer);
    const marks = Array.from(source.querySelectorAll<HTMLElement>("mark[data-hit]"));

    const changed = paintSearchHits(stage, { hitsByPara: hits, currentHit: 2, searching: true });

    expect(changed).toBe(1);
    expect(source.querySelectorAll("mark[data-hit]")).toHaveLength(3);
    const current = source.querySelectorAll<HTMLElement>("mark.is-current");
    expect(current).toHaveLength(1);
    expect(current[0].dataset.hit).toBe("2");
    // 同一个 mark 节点（只切了 className）
    expect(marks[0]).toBe(source.querySelector('mark[data-hit="0"]'));
    // 重复打同一份状态 = 零改动
    expect(paintSearchHits(stage, { hitsByPara: hits, currentHit: 2, searching: true })).toBe(0);
  });

  it("命中集合换了位置：旧位置的高亮被清掉（不能只看数量）", () => {
    const { stage, layer } = buildStage();
    paintSearchHits(stage, { hitsByPara: mapOf([[1, hitsOf(1, 1, 0)]]), currentHit: 0, searching: true });
    expect(pageSource(layer).querySelector('[data-para="1"] mark[data-hit]')).not.toBeNull();

    paintSearchHits(stage, { hitsByPara: mapOf([[4, hitsOf(4, 1, 0)]]), currentHit: 0, searching: true });

    const source = pageSource(layer);
    expect(source.querySelector('[data-para="1"] mark[data-hit]')).toBeNull();
    expect(source.querySelector('[data-para="4"] mark[data-hit]')).not.toBeNull();
    expect(source.querySelector('[data-para="1"]')?.textContent).toBe(PARAGRAPHS[1]);
  });

  it("退出搜索：还原成纯文本，标记全部消失", () => {
    const { stage, layer } = buildStage();
    paintSearchHits(stage, { hitsByPara: mapOf([[0, hitsOf(0, 2)]]), currentHit: 1, searching: true });

    paintSearchHits(stage, { hitsByPara: mapOf([[0, hitsOf(0, 2)]]), currentHit: 1, searching: false });

    const source = pageSource(layer);
    expect(source.querySelectorAll("mark[data-hit]")).toHaveLength(0);
    expect(source.querySelector('[data-para="0"]')?.textContent).toBe(PARAGRAPHS[0]);
  });

  it("单次打点 ≤ 20 ms（12 段正文，10 次连续改命中集合取中位）", () => {
    const { stage } = buildStage();
    const samples: number[] = [];
    for (let round = 0; round < 10; round += 1) {
      const hits = mapOf([[round % 12, hitsOf(round % 12, (round % 3) + 1, round)]]);
      const started = performance.now();
      paintSearchHits(stage, { hitsByPara: hits, currentHit: 0, searching: true });
      samples.push(performance.now() - started);
    }
    samples.sort((a, b) => a - b);
    const median = samples[Math.floor(samples.length / 2)];
    expect(median, `单次打点中位 ${median.toFixed(2)} ms`).toBeLessThan(20);
  });
});

describe("U19 组件层：搜索结果变化不触发整章重排", () => {
  beforeEach(() => {
    document.body.replaceChildren();
  });

  function renderArticle(hitsByPara: Map<number, ReaderHit[]>, searching: boolean) {
    const handleRef = { current: null };
    const props = {
      chapterId: 7,
      title: "第一章 起风",
      paragraphs: PARAGRAPHS,
      hitsByPara,
      currentHit: 0,
      searching,
      settings: READER_DEFAULTS,
      effect: "slide" as const,
      reducedMotion: false,
      initialPara: null,
      landAtEnd: false,
      onCrossChapter: vi.fn(),
      onAnchor: vi.fn(),
      onPageChange: vi.fn(),
      handleRef,
    };
    const view = render(<ReaderPagedArticle {...props} />);
    return {
      rerenderHits: (hits: Map<number, ReaderHit[]>, searching2: boolean) =>
        view.rerender(<ReaderPagedArticle {...props} hitsByPara={hits} currentHit={0} searching={searching2} />),
    };
  }

  it("逐字输入 3 次：排版源重建次数恒为 1（只有进章那一次）", () => {
    const spy = vi.spyOn(PagedLayout.prototype, "setContent");
    try {
      const view = renderArticle(new Map(), false);
      const afterMount = spy.mock.calls.length;
      expect(afterMount).toBe(1);

      // 模拟「雨」→「雨天」→「雨天里」：命中集合逐次变大
      view.rerenderHits(mapOf([[0, hitsOf(0, 1)]]), true);
      view.rerenderHits(mapOf([[0, hitsOf(0, 1)], [1, hitsOf(1, 1)]]), true);
      view.rerenderHits(mapOf([[0, hitsOf(0, 2)], [1, hitsOf(1, 1)]]), true);

      expect(spy.mock.calls.length, "命中集合变化只该打点，不该重建排版源").toBe(afterMount);
    } finally {
      spy.mockRestore();
    }
  });
});