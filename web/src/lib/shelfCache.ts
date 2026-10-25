import type { BookSummary, ReadStatus } from "../api/types";

/**
 * 书架列表状态持久化（sessionStorage）：/read 是 AppShell 之外的顶层路由，
 * 进阅读器时 ShelfPage 整体卸载，组件内 state 必然丢失——把「列表 + 分页 + 滚动位置 + 筛选」
 * 存到会话里，返回时先用缓存渲染，避免白屏与回到第 1 页。
 *
 * 过期口径：30 分钟。清空缓存的唯一出口是手动刷新页面（F5）或超时，不提供「清缓存」按钮。
 */
export const SHELF_CACHE_KEY = "norvels.shelf.v1";
export const SHELF_CACHE_TTL_MS = 30 * 60 * 1000;

export interface ShelfCache {
  sort: string;
  authorId: number | null;
  tag: string | null;
  readStatus: Exclude<ReadStatus, ""> | null;
  /** 评分筛选（1–5）；null / 缺省 = 全部（老缓存的记录里没有这一项） */
  rating?: number | null;
  /** 书单筛选；null / 缺省 = 全部 */
  collection?: number | null;
  size: number;
  mode: "paged" | "infinite";
  items: BookSummary[];
  total: number;
  /** 已成功取到的页码（0 = 还没取过数据） */
  page: number;
  scrollTop: number;
  /** 返回书架时要滚进视口并高亮的书（最近打开的那本） */
  lastBookId: number | null;
  savedAt: number;
}

function storage(): Storage | null {
  try {
    return window.sessionStorage;
  } catch {
    // 隐私模式 / 存储被禁用：退化成「不缓存」，不影响浏览
    return null;
  }
}

/** 读缓存：超时、结构不对、JSON 坏了都当没有缓存，不抛错。 */
export function readShelfCache(now = Date.now()): ShelfCache | null {
  const store = storage();
  if (!store) return null;
  try {
    const raw = store.getItem(SHELF_CACHE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<ShelfCache> | null;
    if (!parsed || typeof parsed !== "object") return null;
    if (typeof parsed.savedAt !== "number" || now - parsed.savedAt > SHELF_CACHE_TTL_MS) return null;
    if (!Array.isArray(parsed.items) || typeof parsed.total !== "number") return null;
    return {
      sort: typeof parsed.sort === "string" ? parsed.sort : "updated",
      authorId: typeof parsed.authorId === "number" ? parsed.authorId : null,
      tag: typeof parsed.tag === "string" ? parsed.tag : null,
      readStatus: (parsed.readStatus ?? null) as Exclude<ReadStatus, ""> | null,
      rating: typeof parsed.rating === "number" && parsed.rating > 0 ? parsed.rating : null,
      collection: typeof parsed.collection === "number" && parsed.collection > 0 ? parsed.collection : null,
      size: typeof parsed.size === "number" && parsed.size > 0 ? parsed.size : 30,
      mode: parsed.mode === "paged" ? "paged" : "infinite",
      items: parsed.items as BookSummary[],
      total: parsed.total,
      page: typeof parsed.page === "number" && parsed.page >= 0 ? parsed.page : 0,
      scrollTop: typeof parsed.scrollTop === "number" && parsed.scrollTop >= 0 ? parsed.scrollTop : 0,
      lastBookId: typeof parsed.lastBookId === "number" ? parsed.lastBookId : null,
      savedAt: parsed.savedAt,
    };
  } catch {
    return null;
  }
}

/** 写缓存：配额满或存储不可用时静默失败（列表照常可用）。 */
export function writeShelfCache(state: Omit<ShelfCache, "savedAt">, now = Date.now()): void {
  const store = storage();
  if (!store) return;
  try {
    store.setItem(SHELF_CACHE_KEY, JSON.stringify({ ...state, savedAt: now }));
  } catch {
    /* 配额满：丢掉这次写入，不打断浏览 */
  }
}

export function clearShelfCache(): void {
  const store = storage();
  if (!store) return;
  try {
    store.removeItem(SHELF_CACHE_KEY);
  } catch {
    /* 忽略 */
  }
}

/**
 * 缓存够新时只渲染不刷新（避免返回瞬间又发第 1 页）；过期到「值得刷新」时才后台合并。
 * 契约 §2.1 要后台刷新、§3 要返回不发 page=1，这里用新近窗口调和：同一个会话里几秒内返回不重拉。
 */
export const SHELF_REFRESH_MIN_AGE_MS = 60_000;

/** 返回书架（阅读器「书架」出口）带给 /shelf 的 state。 */
export interface ShelfRestoreState {
  restore: true;
}

/** 卡片进详情/阅读器时带给目标页的 state：返回时优先 history 回退。 */
export interface ShelfFromState {
  from: "shelf";
  scrollTop: number;
}

/** 列表合并：新的在前，缓存里多出来的（还没被刷掉的）按原顺序补在后面，按 id 去重。 */
export function mergeBooks(fresh: BookSummary[], cached: BookSummary[], keep: number): BookSummary[] {
  const seen = new Set<number>();
  const merged: BookSummary[] = [];
  for (const book of [...fresh, ...cached]) {
    if (seen.has(book.id)) continue;
    seen.add(book.id);
    merged.push(book);
    if (merged.length >= keep) break;
  }
  return merged;
}
