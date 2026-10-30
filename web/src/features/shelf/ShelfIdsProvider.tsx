import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { AuthUser } from "../../api/types";
import { useAuth } from "../../lib/auth";
import { ShelfIdsContext, type ShelfIdsState } from "./shelfIds";

/** 稳定的空集合：换账号时对外暴露它，避免每次渲染新建引用触发下游重渲染。 */
const EMPTY: Set<number> = new Set();

/**
 * 已加入书架集合的 Provider（订阅 useAuth）。
 *
 * 失效口径：账号变了（登录 / 登出 / 换号）→ 立刻清空重取，
 * 绝不能把上一个账号的书架带过来。
 */
export function ShelfIdsProvider({ children, initialState }: { children: ReactNode; initialState?: ShelfIdsState }) {
  if (initialState) return <ShelfIdsContext.Provider value={initialState}>{children}</ShelfIdsContext.Provider>;
  return <ShelfIdsProviderLive>{children}</ShelfIdsProviderLive>;
}

function ShelfIdsProviderLive({ children }: { children: ReactNode }) {
  const auth = useAuth();
  const user = auth.user;
  // 只在"有账号"时取：未登录时 /api/shelf/ids 会 401；开放模式回落默认账号，
  // 但那种部署下书架本来就是"默认账号的书架"，auth.loggedIn 为假，这里也不取。
  const current = user !== null && auth.loggedIn ? user.id : null;
  const [ids, setIds] = useState<Set<number>>(() => new Set());
  const [loadedFor, setLoadedFor] = useState<AuthUser["id"] | null>(null);
  const [nonce, setNonce] = useState(0);
  const generation = useRef(0);

  // 换账号后的第一帧：集合属于旧账号，对外一律当空（渲染期派生，不在 effect 里 setState）
  const stale = loadedFor !== null && loadedFor !== current;
  const visible = stale ? EMPTY : ids;

  useEffect(() => {
    if (current === null) return;
    const controller = new AbortController();
    const mine = ++generation.current;
    api
      .listShelfIds({ signal: controller.signal })
      .then((list) => {
        if (mine !== generation.current) return;
        setIds(new Set(list));
        setLoadedFor(current);
      })
      .catch(() => {
        // 取不到就不打标记（不假装"都不在书架里"，也不阻断浏览）
      });
    return () => controller.abort();
  }, [current, nonce]);

  const refresh = useCallback(() => setNonce((n) => n + 1), []);
  const has = useCallback((bookId: number) => visible.has(bookId), [visible]);
  const toggle = useCallback(
    async (bookId: number) => {
      const was = visible.has(bookId);
      const next = new Set(visible);
      if (was) next.delete(bookId);
      else next.add(bookId);
      setIds(next);
      try {
        if (was) await api.removeFromShelf(bookId);
        else await api.addToShelf(bookId);
        return !was;
      } catch (error) {
        setIds(visible);
        throw error;
      }
    },
    [visible],
  );

  const value = useMemo<ShelfIdsState>(
    () => ({
      ids: visible,
      has,
      ready: current !== null && loadedFor === current,
      loading: current !== null && loadedFor !== current,
      toggle,
      refresh,
    }),
    [visible, has, current, loadedFor, toggle, refresh],
  );

  return <ShelfIdsContext.Provider value={value}>{children}</ShelfIdsContext.Provider>;
}
