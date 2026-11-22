import { fireEvent, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { READER_DEFAULTS, type ReaderSettings } from "./readerSettings";
import { anchorRatio, reflowSignature, scrollRoot, scrollTopForAnchor, useReaderRepositionAfterReflow } from "./useReaderRepositionAfterReflow";

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

/** 正文容器 H5 自己滚时的可滚高度：给 readerSurface 一个「容器可滚」的判定。 */
function makeScrollable(surface: HTMLElement) {
  Object.defineProperty(surface, "scrollHeight", { configurable: true, value: 3200 });
  Object.defineProperty(surface, "clientHeight", { configurable: true, value: 844 });
}

function mountSurface(options: { scrolls: boolean }) {
  document.body.innerHTML = "";
  const surface = document.createElement("div");
  surface.dataset.testid = "reader-swipe-area";
  Object.defineProperty(surface, "getBoundingClientRect", { configurable: true, value: () => ({ top: 0 }) });
  if (options.scrolls) makeScrollable(surface);
  document.body.appendChild(surface);
  const node = document.createElement("p");
  node.id = PARA;
  document.body.appendChild(node);
  return { surface, node };
}

/**
 * jsdom 不做布局：把段落摆到「docTop 减去当前滚动位置」的视口位置上。
 * 滚动位置现取（传函数）：落点写完滚动位置之后，下一步量到的必须是新位置下的视口矩形——
 * 把数字封进闭包会让「落定后再记一遍」量到过期的位置。
 */
function layout(node: HTMLElement, scrollTop: () => number, docTop: number, height: number) {
  Object.defineProperty(node, "getBoundingClientRect", {
    configurable: true,
    value: () => ({ top: docTop - scrollTop(), height }),
  });
}

/**
 * 整页滚（PC）：容器不滚，滚的是 window。jsdom 的 scrollY 是只读访问器、scrollTo 未实现，
 * 这里接管这两处，让它表现得像真的滚过。
 */
function stubPageScroll(start: number) {
  let y = start;
  const calls: number[] = [];
  Object.defineProperty(window, "scrollY", { configurable: true, get: () => y });
  vi.spyOn(window, "scrollTo").mockImplementation((_x?: number | ScrollToOptions, next?: number) => {
    const value = Math.max(0, typeof next === "number" ? next : 0);
    calls.push(value);
    y = value;
  });
  return {
    calls,
    get: () => y,
    read: (value: number) => {
      y = value;
    },
  };
}

function mount(
  settings: ReaderSettings,
  anchor: { chapterId: number; para: number },
  chapterDataId = 1040,
  options: { scrolls: boolean } = { scrolls: true },
) {
  const { surface, node } = mountSurface(options);
  surface.scrollTop = 600;
  layout(node, () => surface.scrollTop, 534, 342);
  const hook = renderHook(
    (props: { settings: ReaderSettings }) =>
      useReaderRepositionAfterReflow({ settings: props.settings, chapterDataId, anchor }),
    { initialProps: { settings } },
  );
  return { ...hook, surface, node };
}

afterEach(() => {
  document.body.innerHTML = "";
  Reflect.deleteProperty(window, "scrollY");
  vi.restoreAllMocks();
});

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

describe("scrollRoot：容器可滚用容器，否则用整页", () => {
  it("H5 正文容器自己滚时取容器", () => {
    const { surface } = mountSurface({ scrolls: true });
    surface.scrollTop = 600;
    expect(scrollRoot()?.get()).toBe(600);
  });

  it("PC 正文容器不滚时取整页滚动（scrollY）", () => {
    mountSurface({ scrolls: false });
    const page = stubPageScroll(600);
    expect(scrollRoot()?.get()).toBe(page.get());
  });
});

describe("useReaderRepositionAfterReflow：容器可滚（H5）", () => {
  it("字号 20→32 后回到同一段的同一位置：600 → 1309，不是原地不动", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    layout(node, () => surface.scrollTop, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(1309));
    // 落点落回同一段里同一处：读者看的还是那一行（旧行为是停在 600，读到的是上一段开头）
    expect(surface.scrollTop).toBeGreaterThan(534);
  });

  it("连着调两档时每档都从新位置起算，读者一直停在同一行", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    layout(node, () => surface.scrollTop, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(1309));

    // 32 → 16：落点 1309 下同一段的段首在 387、段高 213
    layout(node, () => surface.scrollTop, 387, 213);
    rerender({ settings: at({ font: 16 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(428));
  });

  it("往返一圈回到原位：20→32→16→20 漂移不超过 1px", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    layout(node, () => surface.scrollTop, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(1309));

    layout(node, () => surface.scrollTop, 387, 213);
    rerender({ settings: at({ font: 16 }) });
    await waitFor(() => expect(Math.round(surface.scrollTop)).toBe(428));

    layout(node, () => surface.scrollTop, 534, 342);
    rerender({ settings: at({ font: 20 }) });
    await waitFor(() => expect(Math.abs(surface.scrollTop - 600)).toBeLessThanOrEqual(1));
  });

  it("与排版无关的设置变化不改滚动位置", async () => {
    const { surface, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 });
    rerender({ settings: at({ font: 20, autoScroll: true, autoScrollSpeed: 1 }) });
    expect(surface.scrollTop).toBe(600);
  });

  it("锚点不属于当前章时不搬动位置（跨章交给切章逻辑）", async () => {
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1041, para: 1 });
    layout(node, () => surface.scrollTop, 1145, 851);
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

/**
 * PC（>768px）：正文容器定高但内容不滚，滚的是页面。
 * 旧实现只认容器 scrollTop（恒 0），算出的占比没有意义——1280×800 实测改字号后
 * 可见段 idx 979 → 825（−154 段）。判据与 H5 一致：改字号前后停在同段的同一行。
 */
describe("useReaderRepositionAfterReflow：整页滚（PC）", () => {
  it("字号 20→32 后同样回到同一段：window 滚到 1309，容器滚动位置不动", async () => {
    const page = stubPageScroll(600);
    const { surface, node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 }, 1040, { scrolls: false });
    layout(node, page.get, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    await waitFor(() => expect(Math.round(page.get())).toBe(1309));
    expect(Math.round(page.calls.at(-1) ?? 0)).toBe(1309);
    // 整页滚时不该动容器：它本来就不滚，写它的 scrollTop 只会让下一次量测失真
    expect(surface.scrollTop).toBe(600);
  });

  it("PC 下滚动页面也刷新锚点：从读者当下所在的占比起算", async () => {
    const page = stubPageScroll(600);
    const { node, rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 }, 1040, { scrolls: false });
    // 读者滚到 700（仍在同一段：段首 534、段高 342）
    page.read(700);
    layout(node, page.get, 534, 342);
    fireEvent.scroll(window);
    layout(node, page.get, 1145, 851);
    rerender({ settings: at({ font: 32 }) });
    const expected = Math.round(1145 + ((700 - 534) / 342) * 851);
    await waitFor(() => expect(Math.round(page.get())).toBe(expected));
  });

  it("PC 下与排版无关的设置变化同样不动页面位置", async () => {
    const page = stubPageScroll(600);
    const { rerender } = mount(at({ font: 20 }), { chapterId: 1040, para: 1 }, 1040, { scrolls: false });
    rerender({ settings: at({ font: 20, autoScroll: true }) });
    expect(page.calls).toHaveLength(0);
    expect(page.get()).toBe(600);
  });
});
