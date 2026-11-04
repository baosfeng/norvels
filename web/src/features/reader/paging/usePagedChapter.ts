import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ReaderSettings } from "../readerSettings";
import { reflowSignature } from "../useReaderRepositionAfterReflow";
import { firstParaOnPage, paragraphRangeOfPage } from "../../../lib/paging/pageAnchor";
import type { TurnDir } from "../../../lib/paging/turnGesture";
import type { TurnEffect } from "../../../lib/paging/turnEffects";
import { PagedLayout } from "./PagedLayout";
import { PageTurnEngine } from "./PageTurnEngine";
import type { FragmentInput } from "./chapterFragment";
import { buildChapterFragment } from "./chapterFragment";

export interface PagedChapterInput {
  /** 排版源内容（正文 + 搜索命中标记）。 */
  content: FragmentInput;
  /** 内容指纹：正文 / 命中变了才重建排版源。 */
  contentKey: string;
  settings: ReaderSettings;
  effect: TurnEffect;
  reducedMotion: boolean;
  /** 进章要落到的段号（进度恢复 / 书签）；null = 章首。 */
  initialPara: number | null;
  onCrossChapter: (dir: TurnDir) => void;
  onAnchor: (para: number) => void;
  onPageChange: (page: number, pageCount: number) => void;
}

export interface PagedChapterApi {
  stageRef: (node: HTMLDivElement | null) => void;
  page: number;
  pageCount: number;
  /** 当前页覆盖的段落区间（读屏按它念，不是整章）。 */
  pageRange: { start: number; end: number } | null;
  /** 全章最后一段的段号（落末页时当锚点）。 */
  lastPara: number;
  turn: (dir: TurnDir) => boolean;
  /** 跳到第 page 页；anchorPara 给了就把它当重排锚点（书签 / 搜索的目标段）。 */
  gotoPage: (page: number, anchorPara?: number) => void;
  gotoPara: (paraIndex: number) => void;
  pointerDown: (x: number, y: number) => void;
  pointerMove: (x: number, y: number) => boolean;
  pointerUp: (x: number) => void;
  pointerCancel: () => void;
}

/**
 * 分页测量：页数、当前页、段落锚点、重排保位。
 *
 * 「当前页」的真值在这里（不是 scrollTop）：翻页 / 跳转 / 重排都只改这一个数。
 * 三条铁律：
 * - 锚点是段落号不是页码：页码在重排后没有意义，段边界跨字号稳定。
 * - 重排触发的重画**不许**改写锚点：否则字号来回调一次，锚点顺段落往下漂。
 * - 排版与引擎建好之后必须**当场落一次位**（不能指望后续 effect 补），
 *   否则 StrictMode 的二次挂载或任何一次「内容 key 没变」的重建都会留下一个空舞台。
 */
export function usePagedChapter(input: PagedChapterInput): PagedChapterApi {
  const { contentKey, settings, initialPara } = input;
  const stageRef = useRef<HTMLDivElement | null>(null);
  const layoutRef = useRef<PagedLayout | null>(null);
  const engineRef = useRef<PageTurnEngine | null>(null);
  const [state, setState] = useState<{ page: number; pageCount: number; pageRange: { start: number; end: number } | null }>({
    page: 0,
    pageCount: 1,
    pageRange: null,
  });
  const anchor = useRef(initialPara ?? 0);
  const placedKey = useRef<string | null>(null);
  // 引擎回调要读到最新入参；渲染期不写 ref，统一在 layout effect 里同步
  const latest = useRef(input);
  useLayoutEffect(() => {
    latest.current = input;
  });

  const build = useCallback(() => buildChapterFragment(latest.current.content), []);

  /** 一页的完整状态：页码、页数与这一页覆盖的段落区间（读屏按区间念，不是整章）。 */
  const snapshot = useCallback((page: number, pageCount: number) => {
    const layout = layoutRef.current;
    const range = layout ? paragraphRangeOfPage(layout.anchors, page, layout.pageBox.pageW) : null;
    return { page, pageCount, pageRange: range };
  }, []);

  /** 量尺寸 → 量页数 → 落位。targetPara 优先，否则沿用锚点。 */
  const place = useCallback((targetPara: number | null, reset: boolean) => {
    const layout = layoutRef.current;
    const engine = engineRef.current;
    if (!layout || !engine) return;
    layout.measure();
    const measured = layout.measureContent();
    if (!measured) return;
    const found = layout.pageOfPara(targetPara ?? anchor.current);
    const page = Math.min(measured.pageCount - 1, Math.max(0, found ?? engine.currentPage));
    if (reset) engine.reset(page, measured.pageCount);
    else engine.relocate(page, measured.pageCount);
    setState(snapshot(engine.currentPage, measured.pageCount));
    latest.current.onPageChange(engine.currentPage, measured.pageCount);
    if (targetPara !== null) latest.current.onAnchor(targetPara);
  }, [snapshot]);

  // 排版与引擎只建一次；建完立刻落一次位（见函数注释里那条铁律）。
  useLayoutEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const layout = new PagedLayout(stage, build);
    const engine = new PageTurnEngine({
      stage,
      createLayer: (index) => layout.createLayer(index),
      setLayerPage: (layer, index) => layout.setLayerPage(layer, index),
      box: () => layout.pageBox,
      effect: () => latest.current.effect,
      reducedMotion: () => latest.current.reducedMotion,
      onEdge: (dir) => latest.current.onCrossChapter(dir),
      onCommit: (index) => {
        const pi = firstParaOnPage(layout.anchors, index, layout.pageBox.pageW);
        if (pi !== null) anchor.current = pi;
        setState(snapshot(index, layout.pageCount));
        latest.current.onAnchor(pi ?? anchor.current);
        latest.current.onPageChange(index, layout.pageCount);
      },
    });
    layoutRef.current = layout;
    engineRef.current = engine;
    layout.setContent(build);
    place(anchor.current > 0 ? anchor.current : null, true);
    placedKey.current = latest.current.contentKey;
    return () => {
      engine.destroy();
      layout.destroy();
      layoutRef.current = null;
      engineRef.current = null;
      placedKey.current = null;
    };
  }, [build, place, snapshot]);

  // 内容变化（换章 / 搜索标记）：换排版源并重新落位。
  useLayoutEffect(() => {
    const layout = layoutRef.current;
    if (!layout) return;
    if (placedKey.current === contentKey) return;
    placedKey.current = contentKey;
    layout.setContent(build);
    anchor.current = initialPara ?? 0;
    place(initialPara, true);
    // initialPara 只在换章时作数，不单独触发重排
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [contentKey, build, place]);

  // 同一章内换目标段（书签 / 搜索跳到别处）：再落一次位。
  const lastPara = useRef<number | null>(initialPara);
  useLayoutEffect(() => {
    if (lastPara.current === initialPara) return;
    lastPara.current = initialPara;
    if (initialPara === null) return;
    place(initialPara, false);
  }, [initialPara, place]);

  // 排版设置变化：重排并按锚点保位（不改写锚点）。
  const signature = reflowSignature(settings);
  const lastSignature = useRef<string | null>(null);
  useLayoutEffect(() => {
    if (lastSignature.current === null) {
      lastSignature.current = signature;
      return;
    }
    if (lastSignature.current === signature) return;
    lastSignature.current = signature;
    place(null, false);
  }, [signature, place]);

  // 视口变化与字体加载：同样重排保位。
  useLayoutEffect(() => {
    const stage = stageRef.current;
    if (!stage || typeof ResizeObserver === "undefined") return;
    let frame = 0;
    const observer = new ResizeObserver(() => {
      if (frame) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => place(null, false));
    });
    observer.observe(stage);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      observer.disconnect();
    };
  }, [place]);

  useEffect(() => {
    const fonts = typeof document === "undefined" ? undefined : document.fonts;
    if (!fonts?.ready) return;
    let alive = true;
    void fonts.ready.then(() => {
      if (alive) place(null, false);
    });
    return () => {
      alive = false;
    };
  }, [place]);

  const turn = useCallback((dir: TurnDir) => engineRef.current?.turn(dir) ?? false, []);

  /**
   * 跳到某一页。
   *
   * anchorPara 是「这次跳转真正要落到的那一段」：书签 / 搜索给的是目标段，
   * 不能拿页首段顶替——重排（恢复提示收起会改变页高）时按页首段找回，
   * 段落一旦跨页边界就会滑到前一页，读者看到的是「跳过去又退一页」。
   * 不传时才退回「该页第一个起始于本页的段」。
   */
  const gotoPage = useCallback(
    (index: number, anchorPara?: number) => {
      const layout = layoutRef.current;
      const engine = engineRef.current;
      if (!layout || !engine) return;
      engine.relocate(index, layout.pageCount);
      const pi = anchorPara ?? firstParaOnPage(layout.anchors, engine.currentPage, layout.pageBox.pageW);
      if (pi !== null) anchor.current = pi;
      setState(snapshot(engine.currentPage, layout.pageCount));
      latest.current.onAnchor(pi ?? anchor.current);
      latest.current.onPageChange(engine.currentPage, layout.pageCount);
    },
    [snapshot],
  );

  const gotoPara = useCallback(
    (paraIndex: number) => {
      const layout = layoutRef.current;
      if (!layout) return;
      const target = layout.pageOfPara(paraIndex);
      if (target === null) return;
      anchor.current = paraIndex;
      gotoPage(target, paraIndex);
    },
    [gotoPage],
  );

  const stageRefCallback = useCallback((node: HTMLDivElement | null) => {
    stageRef.current = node;
  }, []);

  return {
    stageRef: stageRefCallback,
    page: state.page,
    pageCount: state.pageCount,
    pageRange: state.pageRange,
    lastPara: layoutRef.current?.anchors.at(-1)?.pi ?? 0,
    turn,
    gotoPage,
    gotoPara,
    pointerDown: (x, y) => engineRef.current?.pointerDown(x, y),
    pointerMove: (x, y) => engineRef.current?.pointerMove(x, y) ?? false,
    pointerUp: (x) => engineRef.current?.pointerUp(x),
    pointerCancel: () => engineRef.current?.pointerCancel(),
  };
}