import { useMemo, useState } from "react";
import { useChapter } from "../../api/hooks";
import type { ApiError } from "../../api/client";
import type { ChapterBrief, ChapterDetail } from "../../api/types";
import { splitParagraphs } from "../../lib/readerSearch";

/** 可见章前后各常驻一章：向上回看有上一章、向下预取有下一章，窗口又不会无限变长。 */
const WINDOW_BEFORE = 1;
const WINDOW_AFTER = 1;
/** 越界接续最多在两侧各多放一章：再多就把整本书铺进 DOM 了。 */
export const EDGE_SLOTS = 1;
/** 固定槽位数：目录未就绪时也要占满，hooks 数量不能随数据变化。 */
const SLOT_COUNT = WINDOW_BEFORE + WINDOW_AFTER + 1 + EDGE_SLOTS * 2;
/** 取过的正文留在内存里：窗口移动时同一章会被换到另一个槽位，别让它闪成加载态。 */
const CACHE_LIMIT = 12;

export interface StreamSegment {
  id: number;
  /** 在全书目录里的下标；目录未就绪时为 -1。 */
  index: number;
  title: string;
  wordCount: number;
  /** 原文段落（简繁映射由调用方按设置决定）。 */
  paragraphs: string[];
}

/** 窗口两侧越界补了几章（0 或 1）：滚到边界时加一，换章后归零。 */
interface EdgeExtras {
  head: number;
  tail: number;
}

export interface ChapterWindow {
  /** 渲染窗口（长度固定为 SLOT_COUNT，尾部为 null）。 */
  windowIds: (number | null)[];
  /** 窗口对应的目录下标区间：边界监听用它判断两端还有没有章。 */
  windowStart: number;
  windowEnd: number;
  segments: StreamSegment[];
  loading: boolean;
  error: ApiError | null;
  reload: () => void;
  chapterData: ChapterDetail | null;
  /** 窗口尾部那一章的状态：滚到底接续时的加载中 / 失败。 */
  tailLoading: boolean;
  tailError: ApiError | null;
  tailReload: () => void;
  /** 滚到顶 / 底：把窗口往那一侧多放一章。 */
  extendHead: () => void;
  extendTail: () => void;
}

/**
 * 连续流的渲染窗口：以可见章为中心，前后各留一章，滚到两端时再各多放一章。
 *
 * 窗口两侧的正文必须在读者滚到边界之前就已经在 DOM 里：等到滚到底再请求，
 * 读者会先看到一屏空白；滚到顶才请求，向上回看就断在章界上。
 * 窗口移动时上方内容增减的量由 useChapterStream 补进 scrollTop，正文位置不动。
 */
export function useChapterWindow({
  bookId,
  activeId,
  chapters,
  index,
}: {
  bookId: number;
  activeId: number;
  chapters: ChapterBrief[];
  index: number;
}): ChapterWindow {
  const [extras, setExtras] = useState<EdgeExtras>({ head: 0, tail: 0 });
  const [extrasFor, setExtrasFor] = useState(activeId);
  // 换章后两侧的越界扩展归零：它是「读到边界」的临时状态，不该跟着读者一路带下去
  if (extrasFor !== activeId) {
    setExtrasFor(activeId);
    if (extras.head !== 0 || extras.tail !== 0) setExtras({ head: 0, tail: 0 });
  }

  // 目录未就绪时只渲染入口章，就绪后补上前后章（含两侧越界补的章）
  const windowIds = useMemo<(number | null)[]>(() => {
    if (index < 0 || chapters.length === 0) return pad([activeId]);
    const start = Math.max(0, index - WINDOW_BEFORE - clampExtra(extras.head));
    const end = Math.min(chapters.length - 1, index + WINDOW_AFTER + clampExtra(extras.tail));
    const ids: (number | null)[] = [];
    for (let at = start; at <= end; at += 1) ids.push(chapters[at].id);
    return pad(ids);
  }, [chapters, index, activeId, extras.head, extras.tail]);

  const windowStart = index < 0 ? 0 : Math.max(0, index - WINDOW_BEFORE - clampExtra(extras.head));
  const windowEnd = index < 0 ? 0 : Math.min(chapters.length - 1, index + WINDOW_AFTER + clampExtra(extras.tail));

  // 真实窗口长度：windowIds 尾部是填充的 null 槽位，不能拿数组长度当窗口末尾
  const windowCount = windowEnd - windowStart + 1;

  // 槽位数量固定（含填充），hooks 不能按需增减
  const slot0 = useChapter(bookId, windowIds[0] ?? null);
  const slot1 = useChapter(bookId, windowIds[1] ?? null);
  const slot2 = useChapter(bookId, windowIds[2] ?? null);
  const slot3 = useChapter(bookId, windowIds[3] ?? null);
  const slot4 = useChapter(bookId, windowIds[4] ?? null);
  const slots = [slot0, slot1, slot2, slot3, slot4];

  // 取过的正文留着：窗口移动会把同一章换到另一个槽位，useChapter 换 key 会先清空数据，
  // 不清缓存的话上一章会在读者眼前闪一下加载态（内容消失 → 位置就跳了）。
  const [cache] = useState(() => new Map<number, ChapterDetail>());
  for (const slot of slots) if (slot.data) cache.set(slot.data.id, slot.data);
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
    // 可见章还没取到：不渲染别的章，免得翻章瞬间先闪出上一章的正文
    const active = build(activeId);
    if (!active) return [];
    // 窗口里可见章之前的章按顺序全部渲染（滚到顶多补一章时，上一章的末尾才接得上）；
    // 遇到没取到的章就停——中间留洞会让正文出现一段空白
    const head: StreamSegment[] = [];
    for (let at = activePos - 1; at >= 0; at -= 1) {
      const segment = build(windowIds[at] as number);
      if (!segment) break;
      head.unshift(segment);
    }
    const list: StreamSegment[] = [...head, active];
    for (let at = activePos + 1; at < windowIds.length; at += 1) {
      const segment = build(windowIds[at] as number);
      if (!segment) break;
      list.push(segment);
    }
    return list;
    // slotKey 是「某个槽位的数据变过」的信号；正文统一从 cache 取（同一章可能挪过槽位）
  }, [windowIds, chapters, activeId, slotKey, cache]);

  const activePos = windowIds.indexOf(activeId);
  const activeSlot = activePos >= 0 ? slots[activePos] : null;
  const tailSlot = slots[Math.max(0, windowCount - 1)] ?? null;
  const loaded = segments.length > 0;

  return {
    windowIds,
    windowStart,
    windowEnd,
    segments,
    loading: !loaded && Boolean(activeSlot?.loading),
    error: loaded ? null : (activeSlot?.error ?? null),
    reload: activeSlot?.reload ?? noop,
    chapterData: activeSlot?.data ?? null,
    tailLoading: Boolean(tailSlot?.loading) && loaded,
    tailError: loaded ? (tailSlot?.error ?? null) : null,
    tailReload: tailSlot?.reload ?? noop,
    extendHead: () => setExtras((state) => ({ ...state, head: Math.min(EDGE_SLOTS, state.head + 1) })),
    extendTail: () => setExtras((state) => ({ ...state, tail: Math.min(EDGE_SLOTS, state.tail + 1) })),
  };
}

function clampExtra(value: number): number {
  return Math.max(0, Math.min(EDGE_SLOTS, value));
}

function pad(ids: (number | null)[]): (number | null)[] {
  const out = ids.slice(0, SLOT_COUNT);
  while (out.length < SLOT_COUNT) out.push(null);
  return out;
}

function noop() {}
