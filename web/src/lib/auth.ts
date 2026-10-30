import { createContext, useContext } from "react";
import type { AuthUser } from "../api/types";
import { loginPath } from "../features/auth/authModel";

/**
 * 账号态**只放类型与读取口**，Provider 在 `lib/AuthProvider.tsx`。
 *
 * 拆开的理由与 lib/toast.tsx / lib/toastStore.ts 一致：一个文件同时导出组件与普通函数
 * 会让 Fast Refresh 失效（eslint react-refresh/only-export-components）。
 *
 * 口径（leader 已冻结，别在这里重新发明）：
 * - **匿名可用**：未登录也能浏览全部书、搜索、阅读；登录只决定"个人记录归谁"。
 *   所以这里不提供"守卫/重定向"：需要账号的动作自己问 `loggedIn` 并就地引导去登录页。
 * - **身份只认 `GET /api/auth/me`**：会话是 HttpOnly Cookie，前端读不到也不该读。
 *   200 = 有账号；401 = 未登录。
 * - **开放模式**（后端未配共享口令）下 me() 直接回默认账号 id=1「reader」：
 *   接口上没有"未登录"这个状态，于是 loggedIn 为真但不渲染登录引导——
 *   见下面 `DEFAULT_ACCOUNT_NAME` 的说明。向后兼容是硬要求：这种部署下
 *   用户看不到登录页也能正常用。
 */

/** 后端默认账号名（迁移 0016 的 id=1）：开放模式下 me() 返回的就是它。 */
export const DEFAULT_ACCOUNT_NAME = "reader";

export interface AuthState {
  /** 当前账号；未登录为 null。 */
  user: AuthUser | null;
  /** 真的有属于自己的会话（不是开放模式兜底的默认账号）。 */
  loggedIn: boolean;
  /** 首次判定登录态中：此时不要闪"未登录"提示。 */
  loading: boolean;
  /** 注册并直接登录：201 回账号，会话 Cookie 由后端下发。 */
  register: (name: string) => Promise<AuthUser>;
  /** 输账号登录：200 回账号；号没建 404。 */
  login: (name: string) => Promise<AuthUser>;
  /** 退出登录：清 Cookie + 清本地账号态；请求失败也照样切回未登录。 */
  logout: () => Promise<void>;
  /** 订阅"账号变了"（书架 ids 缓存据此失效，不引第三方状态库）。 */
  subscribe: (listener: () => void) => () => void;
  /** 只清本地账号态（登出进行中先用）。 */
  clearLocal: () => void;
}

/**
 * 缺 Provider 时的默认值 = **匿名**：未登录、没有账号。
 *
 * 与 lib/tier 的默认值（pc）、toast 的模块级 store 同一取舍——账号态是"可有可无的附加信息"，
 * 不是必须存在的依赖：任何一处忘了挂 Provider，界面也只会表现得像"没登录"，
 * 而不是整页白屏（那才是把匿名可用这件事做坏）。
 * 两个写动作显式抛错：静默 no-op 会变成"点了没反应"。
 */
const ANONYMOUS: AuthState = {
  user: null,
  loggedIn: false,
  loading: false,
  register: () => Promise.reject(new Error("账号功能未就绪")),
  login: () => Promise.reject(new Error("账号功能未就绪")),
  logout: async () => undefined,
  subscribe: () => () => undefined,
  clearLocal: () => undefined,
};

export const AuthContext = createContext<AuthState>(ANONYMOUS);

/** 账号态的唯一读取入口。 */
export function useAuth(): AuthState {
  return useContext(AuthContext);
}

/** 当前页对应的登录地址（登录成功后原路回来）。 */
export function loginHrefFor(location: { pathname: string; search: string }): string {
  return loginPath(`${location.pathname}${location.search}`);
}
