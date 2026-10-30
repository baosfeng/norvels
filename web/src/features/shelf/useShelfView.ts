import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../lib/auth";
import { parseShelfView, resolveShelfView, shelfViewQuery, type ShelfView } from "./shelfView";

/**
 * 书架视图（我的书架 / 全部书）⇄ 地址栏。
 *
 * 只写非默认值：已登录的默认是「我的书架」，未登录的默认是「全部书」。
 * 切换用 replace —— 视图是同一页的两种看法，不该在后退栈里堆两条（返回键应当离开书架）。
 */
export function useShelfView() {
  const auth = useAuth();
  const [params, setParams] = useSearchParams();
  const raw = parseShelfView(params.get("view"));
  const view = resolveShelfView(params.get("view"), auth.loggedIn);

  const setView = useCallback(
    (next: ShelfView) => {
      setParams(
        (prev) => {
          const merged = new URLSearchParams(prev);
          merged.delete("view");
          for (const [key, value] of shelfViewQuery(next, auth.loggedIn) ?? []) merged.set(key, value);
          return merged;
        },
        { replace: true },
      );
    },
    [setParams, auth.loggedIn],
  );

  return {
    view,
    /** 地址上明确写过的视图；null = 用的是默认值 */
    explicitView: raw,
    setView,
    loggedIn: auth.loggedIn,
  };
}
