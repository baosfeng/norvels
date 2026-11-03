import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import type { ApiError } from "../../api/client";
import type { BookDetail, ChapterBrief, ChapterDetail, Volume } from "../../api/types";
import { useChapterWindow, type StreamSegment } from "./useChapterWindow";
import { useChapterEdges } from "./useChapterEdges";
import { useVisibleChapter } from "./useVisibleChapter";
import { useStreamAnchor } from "./useStreamAnchor";
import { measureChapters, readerSurface, scrollMetrics, type ChapterAnchor } from "./readerStreamLayout";
import { settleChapterFlip, startChapterFlip } from "../../lib/chapterFlip";
import { useReaderChapterIndex } from "./useReaderChapterIndex";

export type { StreamSegment } from "./useChapterWindow";

/**
 * 连续滚动阅读流：可见章前后各留一章渲染，读者滚过章末时下一章的标题与正文已经在下面，
 * 不用点击、不用跳转、不重新挂载。
 *
 * 五条规则：
 * - 读到哪一章由视口顶部的判定带决定（IntersectionObserver）；URL 用 navigate replace 跟上，
 *   不新增历史条目——浏览器后退仍然是离开阅读器之前的那一页。
 * - 滚到边界（距顶 / 距底不足一屏）只扩窗口，不动可见章：读者滚到底时下一章就在下面，
 *   滚到顶时上一章已经补在头上，两者都不需要等一次请求、也不会看到空白。
 * - 窗口移动会让可见章上方的内容增减（向上补章、向下摘章、上一章正文迟到）：
 *   增多少就往 scrollTop 里补多少，读者眼前的正文一动不动（见 useStreamAnchor）。
 *   不用空白占位补——占位块一旦被滚到就是整屏空白，向上回看会断在章界上。
 * - 显式翻章（按钮 / 键盘 / 滑动 / 滑块）落在新章章首：目标章常常已经在窗口里，
 *   光换 URL 不滚位置的话，判定带会把可见章判回旧章、URL 立刻被 replace 回去——
 *   表现就是「点了没反应」。
 * - 「这次换章是被滚出来的」要留给恢复逻辑：它决定不重置滚动位置。
 */

/**
 * 换章后的判定冷却：覆盖「切换那一帧已经排队、但报告的是旧位置」的 observer 回调。
 * 翻章还套着 260ms 的翻页转场——冷却必须比转场长，否则转场没放完判定就开始读旧位置，
 * 读者会看到「翻到下一章又自己退回上一章」。
 */
const ADVANCE_COOLDOWN_MS = 400;

/**
 * 翻章后的方向守卫：这段时间内，判定带不许把可见章退回「翻章前的那一章」。
 * 转场期间 DOM 被快照、observer 的报告会滞后；补判一旦用了旧集合，
 * 读者就会在松手后被送回上一章（实测现象：停在上一章的末尾）。
 */
const FLIP_BACK_GUARD_MS = 700;

export interface ChapterStream {
  /** 渲染窗口：可见章前后各至少一章。 */
  segments: StreamSegment[];
  activeId: number;
  activeIndex: number;
  activeParagraphs: string[];
  /** 可见章的取数状态（首屏四态用；正文渲染只看 segments）。 */
  chapterData: ChapterDetail | null;
  loading: boolean;
  error: ApiError | null;
  reload: () => void;
  chapters: ChapterBrief[];
  volumes: Volume[];
  /** 目录口径的前后章；翻章手势 / 键盘用它。 */
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  /** 全书进度（按可见章算）。 */
  percent: number;
  /** 显式翻到某一章：加载该章正文并落章首。 */
  flipTo: (target: ChapterBrief | null) => void;
  /** 滚到底时窗口尾部那一章的加载中 / 失败（失败给可见重试）。 */
  tailLoading: boolean;
  tailError: ApiError | null;
  tailReload: () => void;
  /** 取出并清掉「这次换章是滚出来的」标记，恢复逻辑读一次就消费掉。 */
  consumeAdvance: () => boolean;
}

export function useChapterStream({
  bookId,
  routeChapterId,
  book,
  animatedFlips,
}: {
  bookId: number;
  routeChapterId: number;
  book: BookDetail | null;
  /** 显式翻章要不要翻页转场（H5 开；PC 整页滚，快照会跨出正文区，不参与）。 */
  animatedFlips: boolean;
}): ChapterStream {
  const navigate = useNavigate();
  const [activeId, setActiveId] = useState(routeChapterId);
  const advanced = useRef(false);

  const { chapters, volumes, index, prev, next, percent } = useReaderChapterIndex(book, activeId);
  const chapterWindow = useChapterWindow({ bookId, activeId, chapters, index });

  /** 换章后的冷却期截止时间：见 ADVANCE_COOLDOWN_MS。 */
  const cooldownUntil = useRef(0);
  const judgeTimer = useRef<number | null>(null);
  /** 显式翻章还没落位：这期间不许判定带改回可见章（否则点了「下一章」会被判回上一章）。 */
  const pendingFlip = useRef<number | null>(null);
  /** 刚由显式翻章落位：这一提交里的位置补偿必须让路（否则刚对齐的章首会被推回去）。 */
  const flipLanded = useRef(false);
  /** 上一次显式翻章是从哪一章翻走的：方向守卫用它挡住「刚翻过去又被判回来」。 */
  const flipFrom = useRef<number | null>(null);
  const flipAt = useRef(0);
  /** 换章前各章的位置与滚动量：提交后的补偿要拿它们对比，理由见 useStreamAnchor。 */
  const anchorSnapshot = useRef<Map<number, ChapterAnchor>>(new Map());
  const scrollTopBefore = useRef<number | null>(null);
  /** 冷却期结束后的补判回调：judge 要引用自己，走 ref 免得 lint 报「声明前访问」。 */
  const judgeRef = useRef<() => void>(() => {});

  /** 滚过章界线：把可见章换成 targetId，URL 跟上。 */
  const advanceTo = useCallback(
    (targetId: number) => {
      if (chapters.findIndex((chapter) => chapter.id === targetId) < 0 || targetId === activeId) return;
      // 换章前量一次：提交后同一批章节的位置差就是补偿量（见 useStreamAnchor 的两条规则）
      anchorSnapshot.current = measureChapters();
      scrollTopBefore.current = scrollMetrics().scrollTop;
      advanced.current = true;
      cooldownUntil.current = performance.now() + ADVANCE_COOLDOWN_MS;
      setActiveId(targetId);
      // replace 不新增历史条目；也不走 remount，正文节点原地复用，滚动位置不动
      navigate(`/read/${bookId}/${targetId}`, { replace: true });
    },
    [activeId, bookId, chapters, navigate],
  );

  /** 最近一次观察到的可见集合：冷却期结束后的补判要用同一份输入。 */
  const seenRef = useRef<Set<HTMLElement>>(new Set());
  const orderRef = useRef<Map<HTMLElement, number>>(new Map());

  /** 用判定带里压着的章判一次：同时压着两章时取靠前的（界线就在眼前，先算还在上一章）。 */
  const judge = useCallback(
    (seen: Set<HTMLElement>, order: Map<HTMLElement, number>) => {
      seenRef.current = seen;
      orderRef.current = order;
      if (seen.size === 0) return;
      // 显式翻章还没落位：此刻的可见集合还是旧位置
      if (pendingFlip.current !== null) return;
      const wait = cooldownUntil.current - performance.now();
      if (wait > 0) {
        // 冷却期内挂一个补判，避免读者正好停在冷却期里就不再判定
        if (judgeTimer.current === null) {
          judgeTimer.current = window.setTimeout(() => {
            judgeTimer.current = null;
            judgeRef.current();
          }, wait + 16);
        }
        return;
      }
      let pick: HTMLElement | null = null;
      for (const node of seen) {
        if (!pick || (order.get(node) ?? 0) < (order.get(pick) ?? 0)) pick = node;
      }
      if (!pick) return;
      const id = Number(pick.dataset.chapterId);
      if (!Number.isFinite(id)) return;
      // 刚翻过去：判定带还压着翻章前那一章时不许退回去
      if (id === flipFrom.current && performance.now() - flipAt.current < FLIP_BACK_GUARD_MS) return;
      advanceTo(id);
    },
    [advanceTo],
  );
  // 冷却期补判：拿最近一次观察到的集合再判一次（依赖不能挂 judge 自身，走 ref）
  useLayoutEffect(() => {
    judgeRef.current = () => judge(seenRef.current, orderRef.current);
  }, [judge]);

  useVisibleChapter({
    segmentKey: chapterWindow.windowIds.join(","),
    renderedKey: chapterWindow.segments.map((segment) => segment.id).join(","),
    judge,
  });

  /**
   * 首次落位：挂载后正文停在入口章的章首。
   *
   * 流顶部留着上一章，scrollTop = 0 停在的是上一章的开头；判定带第一次判定是异步的，
   * 如果这时已经判到上一章，读者一进来就被「推进」到上一章、入口章反而在下面。
   * 恢复逻辑（进度 / 书签 / 回退）稍后仍会按自己的口径再落一次位，这里只管第一次。
   */
  const placedEntry = useRef(false);
  useLayoutEffect(() => {
    if (placedEntry.current) return;
    const surface = readerSurface();
    const node = surface ? document.querySelector<HTMLElement>(`[data-chapter-id="${routeChapterId}"]`) : null;
    if (!surface || !node) return;
    placedEntry.current = true;
    const top = node.getBoundingClientRect().top;
    if (surface.scrolls) {
      const delta = top - surface.el.getBoundingClientRect().top;
      if (Math.abs(delta) >= 0.5) surface.el.scrollTop = Math.max(0, surface.el.scrollTop + delta);
    } else if (Math.abs(top) >= 0.5) {
      window.scrollBy(0, top);
    }
  });

  /**
   * 显式翻章：先换可见章与 URL，落位与补偿交给 useStreamAnchor（目标章可能还没渲染出来）。
   *
   * 换章这一下由翻页转场包住：旧页滑出、新页滑入。转场要等落位完成才拍照，
   * 所以 settleChapterFlip 挂在 onFlipPlaced 上；目标章还没到时由转场自己的兜底放行。
   */
  const flipTo = useCallback(
    (target: ChapterBrief | null) => {
      if (!target || target.id === activeId) return;
      const targetIndex = chapters.findIndex((chapter) => chapter.id === target.id);
      if (targetIndex < 0) return;
      pendingFlip.current = target.id;
      cooldownUntil.current = performance.now() + ADVANCE_COOLDOWN_MS;
      flipFrom.current = activeId;
      flipAt.current = performance.now();
      const apply = () => {
        anchorSnapshot.current = measureChapters();
        scrollTopBefore.current = scrollMetrics().scrollTop;
        setActiveId(target.id);
        navigate(`/read/${bookId}/${target.id}`, { replace: true });
      };
      // 目标章还没渲染出来（正文在路上）时不做快照转场：转场会把「还没换过去」的画面拍成新页，
      // 拍完再落位，读者看到的是翻页之后又跳一下。走直接切换，等正文到货再落章首。
      const targetRendered = Boolean(document.querySelector(`[data-chapter-id="${target.id}"]`));
      if (animatedFlips && targetRendered) startChapterFlip(targetIndex > index ? "next" : "prev", apply);
      else apply();
    },
    [activeId, animatedFlips, bookId, chapters, index, navigate],
  );

  useStreamAnchor({
    activeId,
    pendingFlipRef: pendingFlip,
    flipLandedRef: flipLanded,
    snapshotRef: anchorSnapshot,
    scrollTopBeforeRef: scrollTopBefore,
    onFlipPlaced: () => {
      // 落位后仍要冷却一下：observer 队列里还压着落位前的交叉状态
      cooldownUntil.current = performance.now() + ADVANCE_COOLDOWN_MS;
      // 位置对上了：转场可以拍新页，入场动画从这里开始
      settleChapterFlip();
    },
    flipFailed: !chapterWindow.loading && Boolean(chapterWindow.error),
  });

  /**
   * 两端越界：只扩窗口，不动可见章。
   * 滚到顶而窗口上方还有章 → 再往上放一章，读者接着往上滚就是上一章的末尾；
   * 滚到底而窗口下方还有章 → 再往下放一章，读者接着往下滚就是下一章的开头。
   */
  useChapterEdges({
    onTop: () => {
      if (chapterWindow.windowStart > 0) chapterWindow.extendHead();
    },
    onBottom: () => {
      if (chapterWindow.windowEnd < chapters.length - 1) chapterWindow.extendTail();
    },
  });

  // 路由换了（翻章 / 目录 / 书签 / 浏览器后退）才重置窗口：自己 replace 出来的回声不算——
  // 回声的 routeChapterId 就等于当前可见章，此时什么都不做。
  // 与 useResource 同一写法：渲染期派生 state，不放进 effect（避免级联渲染）。
  const [syncedRoute, setSyncedRoute] = useState(routeChapterId);
  if (syncedRoute !== routeChapterId) {
    setSyncedRoute(routeChapterId);
    if (routeChapterId !== activeId) setActiveId(routeChapterId);
  }

  const consumeAdvance = useCallback(() => {
    const was = advanced.current;
    advanced.current = false;
    return was;
  }, []);

  return {
    segments: chapterWindow.segments,
    activeId,
    activeIndex: index,
    activeParagraphs: chapterWindow.segments.find((segment) => segment.id === activeId)?.paragraphs ?? [],
    chapterData: chapterWindow.chapterData,
    loading: chapterWindow.loading,
    error: chapterWindow.error,
    reload: chapterWindow.reload,
    chapters,
    volumes,
    prev,
    next,
    percent,
    flipTo,
    tailLoading: chapterWindow.tailLoading,
    tailError: chapterWindow.tailError,
    tailReload: chapterWindow.tailReload,
    consumeAdvance,
  };
}