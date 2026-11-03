import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../lib/auth";
import { hasShelfFilterQuery } from "./shelfQuery";
import { parseShelfView, resolveShelfView, type ShelfView } from "./shelfView";

/**
 * 书架视图（我的书架 / 全部书）⇄ 地址栏。
 *
 * 视图**不进 URL 写入这条链**：地址由 useShelfFilters 的写地址 effect 统一写（筛选 + view 一次写完）。
 * 之前是 setView 自己 setSearchParams —— 切视图同时清筛选时，两个 setSearchParams 在同一批次里
 * 各拿旧地址，后写的把先写的覆盖掉（实测第一次点「我的书架」无效，要点第二次）。
 * 现在 setView 只改 state，写地址只有一处。
 *
 * 地址上带了共享书库的筛选（作者 / 标签 / 分类 / 状态 / 评分 / 书单）时视图一律落在全部书：
 * 那些筛选器只在全部书存在。从详情页点作者（/shelf?author=8）就是这条路径——
 * 否则已登录的人会停在空的「我的书架」，什么条件都看不到。
 */
export function useShelfView() {
  const auth = useAuth();
  const [params] = useSearchParams();
  const raw = parseShelfView(params.get("view"));
  const fromUrl: ShelfView = hasShelfFilterQuery(params) ? "all" : resolveShelfView(params.get("view"), auth.loggedIn);
  const [view, setViewState] = useState<ShelfView>(fromUrl);

  // 外部导航（浏览器后退 / 分享链接）跟着地址走；本地切换时 state 先变、地址随后由筛选的写地址 effect 补齐，
  // 两者一致后这里不会把刚点的结果翻回去。
  useEffect(() => {
    setViewState(fromUrl);
  }, [fromUrl]);

  const setView = useCallback((next: ShelfView) => setViewState(next), []);

  return {
    view,
    /** 地址上明确写过的视图；null = 用的是默认值或地址只带筛选 */
    explicitView: raw,
    setView,
    loggedIn: auth.loggedIn,
  };
}
