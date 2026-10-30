import { useCallback, useEffect, useRef, useState } from "react";
import {
  SHELF_REFRESH_MIN_AGE_MS,
  readShelfCache,
  writeShelfCache,
  type ShelfCache,
} from "../../lib/shelfCache";
import type { BookSummary, ReadStatus } from "../../api/types";
import type { CategoryValue } from "./CategoryFilter";

export interface ShelfSnapshot {
  sort: string;
  authorId: number | null;
  tag: string | null;
  readStatus: Exclude<ReadStatus, ""> | null;
  rating: number | null;
  collection: number | null;
  category: CategoryValue;
  size: number;
  mode: "paged" | "infinite";
  items: BookSummary[];
  total: number;
  page: number;
}

/**
 * 书架的会话缓存与滚动恢复（`/read` 在 AppShell 之外，进阅读器时本页整体卸载）。
 *
 * 三件事，都在本 hook 内闭环，页面只拿 `rememberBook`：
 * 1. 写缓存：滚动停止 300ms、筛选/加载变化、卸载前各落一次；进详情前额外记「最近打开的书」。
 * 2. 恢复：首次拿到数据后滚回原位置，并把最近打开的那本高亮 1.5 秒。
 * 3. 位置记忆：卸载时 DOM 已换成更短的下一页，再读 window.scrollY 会被夹到 0，所以实时记在 ref 里。
 */
export function useShelfScrollRestore({
  snapshot,
  cached,
  enabled,
}: {
  /** 当前列表状态（每次变化都会落一次缓存；调用方传的是派生值，不是 state） */
  snapshot: ShelfSnapshot;
  /** 首帧读到的会话缓存（无缓存时为 null） */
  cached: ShelfCache | null;
  /** 列表有内容才写缓存（空列表写进去只会让下次返回白屏） */
  enabled: boolean;
}) {
  const lastBookRef = useRef<number | null>(cached?.lastBookId ?? null);
  const restoredRef = useRef(false);
  const scrollYRef = useRef(cached?.scrollTop ?? 0);

  const persist = useCallback(
    (lastBookId?: number | null) => {
      if (!enabled) return;
      writeShelfCache({
        sort: snapshot.sort,
        authorId: snapshot.authorId,
        tag: snapshot.tag,
        readStatus: snapshot.readStatus,
        rating: snapshot.rating,
        collection: snapshot.collection,
        categoryL1: snapshot.category.l1,
        categoryL2: snapshot.category.l2,
        size: snapshot.size,
        mode: snapshot.mode,
        items: snapshot.items,
        total: snapshot.total,
        page: snapshot.page,
        scrollTop: Math.round(scrollYRef.current),
        lastBookId: lastBookId === undefined ? lastBookRef.current : lastBookId,
      });
    },
    [snapshot, enabled],
  );

  /** 进详情/阅读器前记一下「最近打开的书 + 当前滚动位置」，返回时据此定位。 */
  const rememberBook = useCallback(
    (id: number) => {
      lastBookRef.current = id;
      scrollYRef.current = typeof window === "undefined" ? 0 : window.scrollY || 0;
      persist(id);
    },
    [persist],
  );

  // 滚动停止 300ms 写一次；卸载（进阅读器）前再补一次，保证返回时位置准确
  useEffect(() => {
    if (!enabled) return;
    let timer = 0;
    function onScroll() {
      scrollYRef.current = window.scrollY || 0;
      window.clearTimeout(timer);
      timer = window.setTimeout(() => persist(), 300);
    }
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("scroll", onScroll);
      persist();
    };
  }, [persist, enabled]);

  // 加载完成后落一次（筛选/排序变化时列表会变，缓存要跟着变）
  useEffect(() => {
    persist();
  }, [persist, snapshot.items.length]);

  // 恢复滚动位置，再把最近打开的那本滚进视口并高亮 1.5 秒（已经在视口里就只高亮，不动位置）
  useEffect(() => {
    if (restoredRef.current || !cached || snapshot.items.length === 0) return;
    restoredRef.current = true;
    scrollYRef.current = cached.scrollTop;
    window.scrollTo({ top: cached.scrollTop });
    const target = cached.lastBookId;
    if (target === null) return;
    const node = document.querySelector<HTMLElement>(`[data-book-id="${target}"]`);
    if (!node) return;
    // 位置优先：缓存里有滚动位置就按它恢复，不再把最近那本拽到中间（否则位置差会被放大）。
    // 只有没有可用位置（新会话 / 从详情页深链进来）时才把这本书滚进视口居中。
    if (cached.scrollTop <= 0) node.scrollIntoView?.({ block: "center" });
    node.classList.add("is-recent");
    window.setTimeout(() => node.classList.remove("is-recent"), 1500);
  }, [cached, snapshot.items.length]);

  return { rememberBook };
}

/**
 * 读缓存（只在首帧读一次）：地址权威 / 要求重置筛选时返回 null —— 不沿用「上次看到哪」。
 * 这两个条件在页面生命周期里不会变化，所以一次读取就够。
 */
export function useShelfCache(ignore: boolean): ShelfCache | null {
  const [snapshot] = useState(() => (ignore ? null : readShelfCache()));
  return snapshot;
}

/** 缓存够新（60 秒内）时连后台刷新都不做，返回瞬间不再发第 1 页。 */
export function shouldRefresh(cached: ShelfCache, now: number): boolean {
  return now - cached.savedAt > SHELF_REFRESH_MIN_AGE_MS;
}
