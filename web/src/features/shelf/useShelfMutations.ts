import { useAuth } from "../../lib/auth";
import { useShelfIds } from "./shelfIds";

/**
 * 「我的书架」（用户主动加入书架的书）的写入口。
 *
 * 口径（leader 已冻结）：书库共享，个人记录按账号隔离；「我的书架」与「全部书」并列，
 * 靠账号区分。后端契约见 docs/api/api-account.md：
 * GET /api/shelf · GET /api/shelf/ids · PUT /api/shelf/{bookId} · DELETE /api/shelf/{bookId}。
 *
 * 状态全部来自 `ShelfIdsContext`（已加入集合 + 乐观增删），这里只把"能不能用"翻译给 UI：
 * `ready` = **这本书真的能加入书架**，即已登录且 ids 已加载；未登录时调用方据此
 * 就地引导去登录页，而不是渲染一个点了没反应的按钮。
 */
export interface ShelfMutations {
  /** 账号与书架接口是否已就绪；false 时"加入书架"仍需渲染（用来引导登录），但点了会去登录页。 */
  ready: boolean;
  /** 这本书是否已在我的书架里；未就绪时恒为 false。 */
  has: (bookId: number) => boolean;
  /** 加入 / 移出我的书架；返回操作后的状态（true = 现在在书架里）。失败会抛出，由调用方提示。 */
  toggle: (bookId: number) => Promise<boolean>;
}

export function useShelfMutations(): ShelfMutations {
  const auth = useAuth();
  const shelf = useShelfIds();
  const ready = auth.loggedIn && shelf.ready;
  return {
    ready,
    has: (bookId: number) => (auth.loggedIn ? shelf.has(bookId) : false),
    toggle: shelf.toggle,
  };
}
