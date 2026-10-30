/**
 * 书架的两个视图：**我的书架**（账号里主动加入的书）与 **全部书**（共享书库）。
 *
 * 口径（leader 已冻结）：①书库共享，个人记录按账号隔离；②「我的书架」= 用户主动加入
 * 书架的书，全部书仍然可浏览；③只注册 + 输账号登录，无密码。
 *
 * 状态进地址（?view=mine|all），与 B1 的筛选参数同一套路（shelfQuery.ts）：
 * 刷新、后退、把链接发给别人都能复现同一屏；会话缓存只兜「返回时看到哪」。
 * 默认值不落地址：未登录 = 全部书，已登录 = 我的书架，两者都是"这个人的默认书架"。
 */
export const SHELF_VIEW_KEY = "view";

export type ShelfView = "mine" | "all";

/**
 * 解析 ?view=：只认 mine / all，非法值当没给。
 * 返回 null = 地址上没有这个参数（调用方据此走"按登录态选默认"）。
 */
export function parseShelfView(raw: string | null): ShelfView | null {
  if (raw === "mine" || raw === "all") return raw;
  return null;
}

/**
 * 实际生效的视图：地址优先；地址没写时按登录态给默认值——
 * 已登录的人打开书架想看的是自己的书，未登录的人只有全部书可看。
 */
export function resolveShelfView(raw: string | null, loggedIn: boolean): ShelfView {
  return parseShelfView(raw) ?? (loggedIn ? "mine" : "all");
}

/** 把视图写进地址；等于默认视图时不写（链接保持短）。 */
export function shelfViewQuery(view: ShelfView, loggedIn: boolean): URLSearchParams | null {
  const out = new URLSearchParams();
  const fallback: ShelfView = loggedIn ? "mine" : "all";
  if (view !== fallback) out.set(SHELF_VIEW_KEY, view);
  return out;
}

/** 「全部书」地址：空书架页的出口按钮用它。 */
export const ALL_BOOKS_LINK = "/shelf?view=all";
