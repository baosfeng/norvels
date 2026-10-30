import { useCallback, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { api } from "../api/client";
import type { AuthUser } from "../api/types";
import { useResource } from "./useResource";
import { AuthContext, DEFAULT_ACCOUNT_NAME, type AuthState } from "./auth";

/**
 * 账号态的 Provider。挂在 App 根（与 TierProvider 同层），不引入状态库。
 *
 * 登录态是"一次取数"：`GET /api/auth/me` 的 401 是**正常状态**而不是故障，
 * 所以这里只吃 data / loading，error 一律当未登录（网络抖动不该把用户挡在登录页外）。
 */
export function AuthProvider({ children, initialState }: { children: ReactNode; initialState?: AuthState }) {
  if (initialState) return <AuthContext.Provider value={initialState}>{children}</AuthContext.Provider>;
  return <AuthProviderLive>{children}</AuthProviderLive>;
}

function AuthProviderLive({ children }: { children: ReactNode }) {
  const { data, loading, reload } = useResource((signal) => api.me({ signal }), []);
  const [override, setOverride] = useState<{ user: AuthUser | null } | null>(null);
  const [listeners] = useState(() => new Set<() => void>());

  const user = override ? override.user : (data ?? null);
  // 开放模式的默认账号：接口上"永远登录着"，但那不是用户自己登录的结果。
  // 按默认账号名判定（后端把 id=1 写成 reader，见迁移 0016）。代价：用户真的注册了
  // 一个叫 reader 的号时也会被当成开放模式——此时写操作仍落到默认账号，
  // 与开放模式行为一致，不会出现"记录写进别人名下"的错误归属。
  const fakeOpenMode = user !== null && user.name === DEFAULT_ACCOUNT_NAME;

  const notify = useCallback(() => {
    for (const listener of listeners) listener();
  }, [listeners]);

  const adopt = useCallback(
    (next: AuthUser) => {
      setOverride({ user: next });
      notify();
    },
    [notify],
  );

  const register = useCallback(
    async (name: string) => {
      const next = await api.register(name);
      adopt(next);
      return next;
    },
    [adopt],
  );

  const login = useCallback(
    async (name: string) => {
      const next = await api.login(name);
      adopt(next);
      return next;
    },
    [adopt],
  );

  const clearLocal = useCallback(() => {
    setOverride({ user: null });
    notify();
  }, [notify]);

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      /* 登出幂等：网络断了也照样把界面切回未登录；下次 me() 会自证真实状态 */
    }
    clearLocal();
    reload();
  }, [clearLocal, reload]);

  const subscribe = useCallback(
    (listener: () => void) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    [listeners],
  );

  const value = useMemo<AuthState>(
    () => ({
      user,
      loggedIn: user !== null && !fakeOpenMode,
      loading,
      register,
      login,
      logout,
      subscribe,
      clearLocal,
    }),
    [user, fakeOpenMode, loading, register, login, logout, subscribe, clearLocal],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
