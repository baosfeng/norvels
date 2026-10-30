import { createContext, useContext } from "react";

/**
 * 「我的书架」已加入集合的类型与读取口（Provider 在 ShelfIdsProvider.tsx）。
 * 拆开的理由同 lib/auth.ts：一个文件同时导出组件与普通函数会让 Fast Refresh 失效。
 *
 * 为什么要缓存：GET /api/shelf/ids 一次回全量 id，列表每张卡都要问"这本在不在书架里"。
 * 每张卡各自发一次请求既慢又不一致，所以在这里预热一次，写操作就地增删（乐观更新）。
 */
export interface ShelfIdsState {
  /** 已加入书架的 id 集合。 */
  ids: Set<number>;
  has: (bookId: number) => boolean;
  /** 是否已经拿到过一次（未登录恒为 false：没有账号就没有书架）。 */
  ready: boolean;
  loading: boolean;
  /** 加入 / 移出：乐观更新，失败回滚并抛出，由调用方提示；返回操作后的状态。 */
  toggle: (bookId: number) => Promise<boolean>;
  /** 重新拉一次（外部改了书架时用）。 */
  refresh: () => void;
}

/**
 * 缺 Provider 时的默认值 = **空书架且未就绪**（与 useAuth 的匿名默认值同一口径）：
 * 调用方看到 ready=false 就不会声称"这本书在书架里"，也不会出现"点了没反应"的假按钮。
 */
const EMPTY_SHELF: ShelfIdsState = {
  ids: new Set(),
  has: () => false,
  ready: false,
  loading: false,
  toggle: () => Promise.reject(new Error("书架功能未就绪")),
  refresh: () => undefined,
};

export const ShelfIdsContext = createContext<ShelfIdsState>(EMPTY_SHELF);

export function useShelfIds(): ShelfIdsState {
  return useContext(ShelfIdsContext);
}
