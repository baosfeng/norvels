import type { ApiError } from "../../api/client";

/**
 * 账号页的纯逻辑（无 React、无取数）：名字校验、`?next=` 收敛、错误 → 引导动作。
 *
 * 单独一层是为了让"什么时候该显示注册按钮"这种判断能被直接单测：
 * 状态码（404 / 409）是后端的契约，文案与按钮是前端的决定，两者不该糊在一起。
 */

/** 与后端 maxUserNameRunes 同一口径：按 rune（不是字节）计 32 字。 */
export const MAX_ACCOUNT_NAME_RUNES = 32;

/** 登录成功后去哪里：`?next=` 缺省回书架。 */
export const AUTH_DEFAULT_NEXT = "/shelf";

export type NameCheck = { ok: true; name: string } | { ok: false; message: string };

/**
 * 前端先校验再发请求：空名 / 超 32 字是**本地就能判定**的错，
 * 打一趟后端只会让用户多等一个往返，还换回一句同样的提示。
 */
export function checkAccountName(raw: string): NameCheck {
  const name = raw.trim();
  if (!name) return { ok: false, message: "先输入账号名。" };
  if ([...name].length > MAX_ACCOUNT_NAME_RUNES) {
    return { ok: false, message: `账号名最多 ${MAX_ACCOUNT_NAME_RUNES} 字。` };
  }
  return { ok: true, name };
}

/**
 * `?next=` 只接受站内路径：`/` 开头，且不能是 //host 或 /\host 这种协议相对地址（会跳到站外）。
 * 非法值一律回落到书架——登录页不是开放重定向入口。
 */
export function safeNext(raw: string | null | undefined): string {
  const value = (raw ?? "").trim();
  if (!value.startsWith("/") || value.startsWith("//") || value.startsWith("/\\")) return AUTH_DEFAULT_NEXT;
  return value;
}

/** 生成去登录页的地址：带回跳地址，登录成功原地回来。 */
export function loginPath(next: string): string {
  return `/login?next=${encodeURIComponent(next)}`;
}

/** 登录页表单的两个动作。 */
export type AuthAction = "login" | "register";

/**
 * 后端错误 → 界面引导。
 *
 * - 404 account_not_found：号还没建 → 直接给「注册新账号」按钮（带上已输入的名字）；
 * - 409 conflict：名字被占 → 直接给「直接进入」（登录）按钮；
 * - 400 invalid_param：名字本身不合规 → 只提示，不给动作；
 * - 其它：网络 / 超时 / 5xx → 提示重试，不猜原因。
 */
export interface AuthErrorHint {
  message: string;
  /** 出现这个按钮就说明「下一步该点什么」；null = 只提示。 */
  action: AuthAction | null;
}

export function accountErrorHint(error: ApiError): AuthErrorHint {
  if (error.status === 404 || error.code === "account_not_found") {
    return { message: "账号不存在，请先注册。", action: "register" };
  }
  if (error.status === 409 || error.code === "conflict") {
    return { message: "该账号已存在，直接进入即可。", action: "login" };
  }
  if (error.status === 400) return { message: error.message || "账号名不合规，换一个试试。", action: null };
  if (error.kind === "timeout") return { message: "请求超时了，稍后再试一次。", action: null };
  return { message: error.message || "没能连上服务，稍后再试。", action: null };
}
