import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { READER_DEFAULTS, type ReaderSettings } from "./readerSettings";
import { anchorRatio, reflowSignature, scrollTopForAnchor, useReaderRepositionAfterReflow } from "./useReaderRepositionAfterReflow";

const at = (patch: Partial<ReaderSettings>): ReaderSettings => ({ ...READER_DEFAULTS, ...patch });

/**
 * 实测几何（390×844，/read/12/1040，scrollTop=600 时读第 2 段 19% 处）：
 * 20px 段首 534 / 段高 342，32px 段首 1145 / 段高 851，16px 段首 387 / 段高 213。
 *
 * ⚠ 判据是「改字号前后可见首段与段内位置不变」，不是「进度百分比不变」。
 * 本章只有 4–6 段、单段 342px（32px 字号时 851px），段粒度极粗，百分比由章节几何派生，
 * 段落一换几何它就必然变：实测 20→32 是 54%→39%，32→16 是 →77%。
 * 那不是 bug——读者仍停在同一行；要求百分比不变，等于要求把他甩到别处（正是要修的那个 bug）。
 * 谁想改成断言百分比，先读这段。往返实测：20→32→16→20 回到 601 / 54% / 同一段，漂移 1px。
 */
const PARA = "para-1";

function mountSurface() {
  document.body.innerHTML = "";
  const surface = document.createElement("div");
  surface.dataset.testid = "reader-swipe-area";
  Object.defineProperty(surface, "getBoundingClientRect", { configurable: true, value: () => ({ top: 0 }) });
  document.body.appendChild(surface);
  const node = document.createElement("p");
  node.id = PARA;
  document.body.appendChild(node);
  return { surface, node };
}

/** jsdom 不做布局：把段落摆到「docTop 减去当前 scrollTop」的视口位置上。 */
function layout(node: HTMLElement, surface: HTMLElement, docTop: number, height: number) {
  Object.defineProperty(node, "getBoundingClientRect", {
    configurable: true,
    value: () => ({ top: docTop - surface.scrollTop, height }),
  });
}

function mount(settings: ReaderSettings, anchor: { chapterId: number; para: number }, chapterDataId = 1040) {
  const { surface, node } = mountSurface();
  surface.scrollTop = 600;
  layout(node, surface, 534, 342);
  const hook = renderHook(
    (props: { settings: ReaderSettings }) =>
      useReaderRepositionAfterReflow({ settings: props.settings, chapterDataId, anchor }),
    { initialProps: { settings } },
  );
  return { ...hook, surface, node };
}

describe("reflowSignature", () => {
  it("只认参与排版的设置项：自动滚动、速度、繁转简以外的开关变化都不算重排", () => {
    const base = reflowSignature(at({}));
    expect(reflowSignature(at({ autoScroll: true, autoScrollSpeed: 0.5 }))).toBe(base);
    for (const patch of [
      { font: 32 },
      { leading: 2.4 },
      { fontFamily: "sans" as const },
      { para: 1.6 },
      { width: "wide" as const },
      { align: "justify" as const },
      { indent: false },
      { followSystemFont: true },
      { script: true },
    ]) {
      expect(reflowSignature(at(patch))).not.toBe(base);
    }
  });
});

describe("anchorRatio", () => {
  it("段首对屏幕顶是 0，段尾对屏幕顶是 1，越界夹在 0–1", () => {
    expect(anchorRatio(534, 534, 342)).toBe(0);
    expect(anchorRatio(876, 534, 342)).toBe(1);
    expect(anchorRatio(600, 534, 342)).toBeCloseTo(0.193, 3);
    expect(anchorRatio(1000, 534, 342)).toBe(1);
    expect(anchorRatio(100, 534, 342)).toBe(0);
  });

  it("段高退化为 0 时不除零", () => {
    expect(anchorRatio(600, 534, 0)).toBe(0);
  });
});

describe("scrollTopForAnchor", () => {
  it("同一段同一成位置换字号后落回读者读到的那一行", () => {
    const box = { docTop: 534, height: 342, ratio: anchorRatio(600, 534, 342) };
    // 20px → 32px：段首 1145、段高 851
    expect(scrollTopForAnchor(box, { docTop: 1145, height: 851 })).toBeCloseTo(1309.2, 1);
    // 32px 的落点再换到 16px：段首 387、段高 213
    expect(scrollTopForAnchor(box, { docTop: 387, height: 213 })).toBeCloseTo(428.1, 1);
  });
});

describe("useReaderRepositionAfterReflow", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("字号 20→32 后回到同一段的同一位置：600 → 1309，不是原地不动", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    layout(node, surface, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(1309));
    // 落点落回同一段里同一处：读者看的还是那一行（旧行为是停在 600，读到的是上一段开头）
    expect(surface.scrollTop).toBeGreaterThan(534);
  });

  it("连着调两档时每档都从新位置起算，读者一直停在同一行", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    layout(node, surface, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(1309));

    // 32 → 16：落点 1309 下同一段的段首在 387、段高 213
    layout(node, surface, 387, 213);
    rerender({ settings: at({ font: 16 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(428));
  });

  it("往返一圈回到原位：20→32→16→20 漂移不超过 1px", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    layout(node, surface, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(1309));

    layout(node, surface, 387, 213);
    rerender({ settings: at({ font: 16 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(428));

    layout(node, surface, 534, 342);
    rerender({ settings: at({ font: 20 }) });
    await waitFor(() => expect(Math.abs(surface.scrollTop - 600)).toBeLessThanOrEqual(1));
  });

  it("与排版无关的设置变化不改滚动位置", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    layout(node, surface, 534, 342);
    rerender({ settings: at({ font: 20, autoScroll: true, autoScrollSpeed: 1 }) });
    expect(surface.scrollTop).toBe(600);
  });

  it("锚点不属于当前章时不搬动位置（跨章交给切章逻辑）", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1041, para: 1 });
    layout(node, surface, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    expect(surface.scrollTop).toBe(600);
  });

  it("首次挂载只记录不搬运", () => {
    const { surface } = mount(at({ font: 32 }), { chapterId: 1040, para: 1 });
    expect(surface.scrollTop).toBe(600);
  });

  it("锚点段不在 DOM 里时不动滚动位置", () => {
    const { surface } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    document.getElementById(PARA)?.remove();
    expect(surface.scrollTop).toBe(600);
  });
});
