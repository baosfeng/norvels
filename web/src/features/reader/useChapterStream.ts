import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useChapter } from "../../api/hooks";
import type { ApiError } from "../../api/client";
import type { BookDetail, ChapterBrief, ChapterDetail, Volume } from "../../api/types";
import { splitParagraphs } from "../../lib/readerSearch";
import { useReaderChapterIndex } from "./useReaderChapterIndex";

/**
 * 连续滚动阅读流：可见章前后各留一章渲染，读者滚过章末时下一章的标题与正文已经在下面，
 * 不用点击、不用跳转、不重新挂载。
 *
 * 三条规则：
 * - 读到哪一章由视口中部的判定带决定（IntersectionObserver）；URL 用 navigate replace 跟上，
 *   不新增历史条目——浏览器后退仍然是离开阅读器之前的那一页。
 * - 窗口移动会让可见章上方的内容增减（向上补章、向下摘章、上一章正文迟到）：
 *   增多少就往 scrollTop 里补多少、减多少就减多少，读者眼前的正文一动不动。
 *   不用空白占位补——占位块一旦被滚到就是整屏空白，向上回看会断在章界上。
 * - 「这次换章是被滚出来的」要留给恢复逻辑：它决定不重置滚动位置；翻章（滑动 / 点按 / 键盘）
 *   仍然落章首。
 */

/** 可见章前后各渲染一章：向上回看还有上一章，窗口又不会无限变长。 */
const WINDOW_BEFORE = 1;
const WINDOW_AFTER = 1;

/**
 * 可见章判定带：视口 10%–15% 这条窄带落在哪一章，就算读到哪一章。
 * 贴着顶沿而不是正中——进入章节时读者停在章首，正中会把「上方还留着的上一章」当成可见章。
 */
const VISIBLE_BAND = "-10% 0px -85% 0px";

/** 取过的正文留在内存里：窗口移动时同一章会被换到另一个槽位，别让它闪成加载态。 */
const CACHE_LIMIT = 8;

/** 换章后的判定冷却：覆盖「切换那一帧已经排队、但报告的是旧位置」的 observer 回调。 */
const ADVANCE_COOLDOWN_MS = 220;

const SURFACE_SELECTOR = '[data-testid="reader-swipe-area"]';
const SEGMENT_SELECTOR = "[data-chapter-id]";

/** 正文滚动容器：H5 正文自己滚（.reader-main 定高 overflow），PC 由整页滚。 */
interface Surface {
  el: HTMLElement;
  scrolls: boolean;
}

function readerSurface(): Surface | null {
  const el = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
  if (!el) return null;
  return { el, scrolls: el.scrollHeight - el.clientHeight > 1 };
}

function pageScrollTop(): number {
  return window.scrollY || document.documentElement.scrollTop || 0;
}

/**
 * 章节在正文内容里的位置：与滚动位置无关，只有「它上方的内容增减」才会让它变。
 *
 * 不写成「视口位置 + scrollTop」：正文容器的可滚性在挂载瞬间会反复变（内容还没铺满
 * 一屏时容器不可滚、由整页滚），容器坐标与窗口坐标混用会量出假的增减量，
 * 于是补一次、判定带翻一章、再补一次，读者被带着往上一路退。
 */
function contentOffset(node: HTMLElement): number {
  const root = document.querySelector<HTMLElement>(".reader-stream") ?? node;
  return node.getBoundingClientRect().top - root.getBoundingClientRect().top;
}

/** 把滚动位置挪 delta：上方涨了多少就多滚多少、缩了多少就少滚多少，读者眼前的正文不动。 */
function nudgeScroll(surface: Surface, delta: number): void {
  if (surface.scrolls) {
    surface.el.scrollTop = Math.max(0, surface.el.scrollTop + delta);
    return;
  }
  window.scrollTo(0, Math.max(0, pageScrollTop() + delta));
}

export interface StreamSegment {
  id: number;
  /** 在全书目录里的下标；目录未就绪时为 -1。 */
  index: number;
  title: string;
  wordCount: number;
  /** 原文段落（简繁映射由调用方按设置决定）。 */
  paragraphs: string[];
}

export interface ChapterStream {
  /** 渲染窗口：从可见章开始、向后连续，最多 3 章。 */
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
  /** 窗口尾部之后的那一章：自动滚动滚到底时翻它。 */
  flipTarget: ChapterBrief | null;
  flipIndex: number;
  /** 取出并清掉「这次换章是滚出来的」标记，恢复逻辑读一次就消费掉。 */
  consumeAdvance: () => boolean;
}

export function useChapterStream({
  bookId,
  routeChapterId,
  book,
}: {
  bookId: number;
  routeChapterId: number;
  book: BookDetail | null;
}): ChapterStream {
  const navigate = useNavigate();
  const [activeId, setActiveId] = useState(routeChapterId);
  const advanced = useRef(false);

  const { chapters, volumes, index, prev, next, percent } = useReaderChapterIndex(book, activeId);

  // 渲染窗口：目录未就绪时只渲染入口章，就绪后补上前后章
  const windowIds = useMemo(() => {
    if (index < 0) return [activeId];
    const ids: number[] = [];
    if (index - WINDOW_BEFORE >= 0) ids.push(chapters[index - WINDOW_BEFORE].id);
    ids.push(activeId);
    if (index + WINDOW_AFTER < chapters.length) ids.push(chapters[index + WINDOW_AFTER].id);
    return ids;
  }, [chapters, index, activeId]);

  const slotA = useChapter(bookId, windowIds[0] ?? null);
  const slotB = useChapter(bookId, windowIds[1] ?? null);
  const slotC = useChapter(bookId, windowIds[2] ?? null);
  const slots = [slotA, slotB, slotC];

  // 取过的正文留着：窗口移动会把同一章换到另一个槽位，useChapter 换 key 会先清空数据，
  // 不清缓存的话上一章会在读者眼前闪一下加载态（内容消失 → 位置就跳了）。
  const [cache] = useState(() => new Map<number, ChapterDetail>());
  for (const slot of slots) {
    if (slot.data) cache.set(slot.data.id, slot.data);
  }
  while (cache.size > CACHE_LIMIT) {
    const oldest = cache.keys().next().value;
    if (oldest === undefined) break;
    cache.delete(oldest);
  }

  // 槽位数据变化（取到 / 换章 / 重试）要重算段落：槽位自身的 key 变化不体现在 windowIds 上
  const slotKey = slots.map((slot) => `${slot.data?.id ?? 0}:${slot.loading ? 1 : 0}:${slot.error ? 1 : 0}`).join(",");

  const segments = useMemo<StreamSegment[]>(() => {
    // slotKey 只是重算信号：它把「槽位数据变了」这件事带进依赖数组
    if (!slotKey) return [];
    const activePos = windowIds.indexOf(activeId);
    if (activePos < 0) return [];
    const build = (id: number) => {
      const data = cache.get(id);
      if (!data) return null;
      return {
        id,
        index: chapters.findIndex((chapter) => chapter.id === id),
        title: data.title,
        wordCount: data.wordCount,
        paragraphs: splitParagraphs(data.content),
      } satisfies StreamSegment;
    };
    const active = build(activeId);
    // 可见章还没取到：不渲染别的章，免得翻章瞬间先闪出上一章的正文
    if (!active) return [];
    const list: StreamSegment[] = [];
    const before = activePos > 0 ? build(windowIds[activePos - 1]) : null;
    if (before) list.push(before);
    list.push(active);
    for (let at = activePos + 1; at < windowIds.length; at += 1) {
      const segment = build(windowIds[at]);
      if (!segment) break;
      list.push(segment);
    }
    return list;
    // slotKey 是「某个槽位的数据变过」的信号；正文统一从 cache 取（同一章可能挪过槽位）
  }, [windowIds, chapters, activeId, slotKey, cache]);

  const activeSlot = (() => {
    const at = windowIds.indexOf(activeId);
    return at >= 0 ? slots[at] : null;
  })();
  const loaded = segments.length > 0;
  const loading = !loaded && Boolean(activeSlot?.loading);
  const error = loaded ? null : (activeSlot?.error ?? null);

  /**
   * 切换后的冷却期：IntersectionObserver 的回调是异步的，切换那一刻已经在队列里的 entry
   * 报告的是「补偿之前」的交叉状态——照单全收就会把刚读到的章又判回上一章，
   * 补偿再把位置推回去，快速滚动时形成 12149↔13549 的来回震荡。
   * 冷却期内的判定一律丢弃，冷却结束时用最新的可见集合补判一次。
   */
  const cooldownUntil = useRef(0);
  const judgeTimer = useRef<number | null>(null);
  const seenRef = useRef<Set<HTMLElement>>(new Set());
  const orderRef = useRef<Map<HTMLElement, number>>(new Map());

  /** 滚过章界线：把可见章换成 targetId，URL 跟上。 */
  const advanceTo = useCallback(
    (targetId: number) => {
      if (chapters.findIndex((chapter) => chapter.id === targetId) < 0 || targetId === activeId) return;
      advanced.current = true;
      cooldownUntil.current = performance.now() + ADVANCE_COOLDOWN_MS;
      setActiveId(targetId);
      // replace 不新增历史条目；也不走 remount，正文节点原地复用，滚动位置不动
      navigate(`/read/${bookId}/${targetId}`, { replace: true });
    },
    [activeId, bookId, chapters, navigate],
  );

  /** 冷却结束后的补判回调：judge 内部要引用自己，走 ref 免得 lint 报「声明前访问」。 */
  const judgeRef = useRef<() => void>(() => {});

  /** 用当前的可见集合判一次章；冷却期内挂一个补判，避免用户正好停在冷却期里。 */
  const judge = useCallback(() => {
    const seen = seenRef.current;
    if (seen.size === 0) return;
    const wait = cooldownUntil.current - performance.now();
    if (wait > 0) {
      if (judgeTimer.current === null) {
        judgeTimer.current = window.setTimeout(() => {
          judgeTimer.current = null;
          judgeRef.current();
        }, wait + 16);
      }
      return;
    }
    // 判定带里同时压着两章时取靠前的那一章：界线就在眼前，先算还在上一章
    let pick: HTMLElement | null = null;
    for (const node of seen) {
      if (!pick || (orderRef.current.get(node) ?? 0) < (orderRef.current.get(pick) ?? 0)) pick = node;
    }
    if (!pick) return;
    const id = Number(pick.dataset.chapterId);
    if (Number.isFinite(id)) advanceTo(id);
  }, [advanceTo]);

  useEffect(() => {
    judgeRef.current = judge;
  }, [judge]);

  const segmentKey = windowIds.join(",");
  // 窗口是「想要哪几章」，renderedKey 是「真的渲染出来了哪几章」：
  // 正文到货前 effect 已经跑过一次（那时 DOM 里还没有 section），必须按渲染结果再订阅一次。
  const renderedKey = segments.map((segment) => segment.id).join(",");
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") return;
    if (!renderedKey) return;
    const nodes = Array.from(document.querySelectorAll<HTMLElement>(SEGMENT_SELECTOR));
    if (nodes.length === 0) return;
    // H5 正文自己滚（.reader-main 定高 overflow），判定带就相对这个容器；PC 由整页滚，相对视口
    const surface = document.querySelector<HTMLElement>(SURFACE_SELECTOR);
    const scroller = surface && surface.scrollHeight - surface.clientHeight > 1 ? surface : null;
    orderRef.current = new Map(nodes.map((node, at) => [node, at]));
    const seen = new Set<HTMLElement>();
    seenRef.current = seen;
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const node = entry.target as HTMLElement;
          if (entry.isIntersecting) seen.add(node);
          else seen.delete(node);
        }
        judge();
      },
      { root: scroller, rootMargin: VISIBLE_BAND },
    );
    nodes.forEach((node) => observer.observe(node));
    return () => {
      observer.disconnect();
      seenRef.current = new Set();
      if (judgeTimer.current !== null) {
        window.clearTimeout(judgeTimer.current);
        judgeTimer.current = null;
      }
    };
  }, [segmentKey, renderedKey, judge]);

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
    // H5 滚正文容器、PC 滚整页：两边都按「本章 section 的视口位置」对齐
    const top = node.getBoundingClientRect().top;
    if (surface.scrolls) {
      const delta = top - surface.el.getBoundingClientRect().top;
      if (Math.abs(delta) >= 0.5) surface.el.scrollTop = Math.max(0, surface.el.scrollTop + delta);
    } else if (Math.abs(top) >= 0.5) {
      window.scrollBy(0, top);
    }
  });

  /**
   * 上方内容增减的补偿：可见章在流里的位置变了多少，就把滚动位置挪多少。
   *
   * 会变的情况全走这一条路：向上滚补上上一章（加高）、滚到下一章摘掉更早一章（减高）、
   * 上一章正文迟到后插进来、恢复提示条收起。布局效应里补，赶在绘制之前，读者看不到跳变。
   *
   * 基准是「上一次提交时可见章的位置与高度」：可见章没变时直接比它自己的位置；
   * 换章了（滚过界线 / 左右滑 / 键盘）就用上一个可见章的上沿或下沿算当前章本该在哪——
   * 相邻两章的几何是同一帧量出来的，不用猜方向。锚点章已经不在流里（目录 / 书签跳远）
   * 就只重取基准，落点交给恢复逻辑。
   *
   * 不设依赖数组：每一次提交都要量。恢复提示条收起、上一章正文迟到都发生在「可见章没变」
   * 的提交里，漏量的那 80px 就足以让判定带翻到隔壁章，再被补偿推回来，来回打成死循环。
   */
  const anchorOffset = useRef<{ id: number; offset: number; height: number } | null>(null);
  useLayoutEffect(() => {
    const surface = readerSurface();
    if (!surface) return;
    const activeNode = document.querySelector<HTMLElement>(`[data-chapter-id="${activeId}"]`);
    if (!activeNode) {
      anchorOffset.current = null;
      return;
    }
    const prev = anchorOffset.current;
    const prevNode = prev ? document.querySelector<HTMLElement>(`[data-chapter-id="${prev.id}"]`) : null;
    if (prev && prev.id === activeId) {
      // 可见章没变：只有「它上方的内容」变了，差值就是要补的滚动量
      const growth = contentOffset(activeNode) - prev.offset;
      if (Math.abs(growth) >= 0.5) nudgeScroll(surface, growth);
    } else if (prev && prevNode) {
      // 换章了（滚过界线 / 左右滑 / 键盘）：上一页的可见章通常还留在流里，与当前章相邻，
      // 用它的下沿（它在上）或上沿（它在下）算出当前章本该在哪
      const height = activeNode.getBoundingClientRect().height;
      const prevAbove = Boolean(prevNode.compareDocumentPosition(activeNode) & Node.DOCUMENT_POSITION_FOLLOWING);
      const base = prevAbove ? prev.offset + prev.height : prev.offset - height;
      const growth = contentOffset(activeNode) - base;
      if (Math.abs(growth) >= 0.5) nudgeScroll(surface, growth);
    }
    // 基准一律记在可见章上：下一次提交量的就是同一个节点
    anchorOffset.current = {
      id: activeId,
      offset: contentOffset(activeNode),
      height: activeNode.getBoundingClientRect().height,
    };
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

  const tailIndex = loaded ? segments[segments.length - 1].index : index;
  const flipTarget = tailIndex >= 0 && tailIndex + 1 < chapters.length ? chapters[tailIndex + 1] : null;

  return {
    segments,
    activeId,
    activeIndex: index,
    activeParagraphs: segments.find((segment) => segment.id === activeId)?.paragraphs ?? [],
    chapterData: activeSlot?.data ?? null,
    loading,
    error,
    reload: activeSlot?.reload ?? noop,
    chapters,
    volumes,
    prev,
    next,
    percent,
    flipTarget,
    flipIndex: tailIndex,
    consumeAdvance,
  };
}

function noop() {}