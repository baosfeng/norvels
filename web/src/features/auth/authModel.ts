import type { ApiError } from "../../api/client";

/**
 * 账号页的纯逻辑（无 React、无取数）：名字校验、`?next=` 收敛、错误 → 引导动作，
 * 以及账号选择器要的那几件事——`?switch=1` 的取舍、「上次活跃」的说法、成功 toast 的文案。
 *
 * 单独一层是为了让"什么时候该显示注册按钮"这种判断能被直接单测：
 * 状态码（404 / 409）是后端的契约，文案与按钮是前端的决定，两者不该糊在一起。
 */

/** 与后端 maxUserNameRunes 同一口径：按 rune（不是字节）计 32 字。 */
export const MAX_ACCOUNT_NAME_RUNES = 32;

const MINUTE_MS = 60_000;
const HOUR_MS = 60 * MINUTE_MS;

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

/**
 * 账号页只有一个写动作：**进入**（不存在就建号，存在就登录）。
 *
 * 以前这里分 login / register 两个值，界面据此在 404 时给「去注册」、409 时给
 * 「去登录」——那是在让用户替后端猜「这个名字有没有人用」。单入口下这些分支
 * 全部消失：动作只有一个值，失败提示也不再引导去点第二个按钮。
 */
export type AuthAction = "enter";

/**
 * 后端错误 → 界面提示。
 *
 * 单入口下 404 account_not_found / 409 conflict 不再可能出现（后端内部消化成
 * 「直接登录」），保留兜底文案只为容忍旧版后端；**任何分支都不再给引导动作**。
 *
 * - 400 invalid_param：名字本身不合规 → 提示换一个；
 * - 409 default_account：删账号时才出现 → 提示这个账号是书库底座，不能删；
 * - 404 其它：账号 / 记录不在了 → 提示刷新后重试；
 * - 网络 / 超时 / 5xx → 提示重试，不猜原因。
 */
export interface AuthErrorHint {
  message: string;
}

export function accountErrorHint(error: ApiError): AuthErrorHint {
  if (error.status === 400) return { message: error.message || "账号名不合规，换一个试试。" };
  if (error.code === "default_account") {
    return { message: "这是书库的默认账号，不能删除；换成你自己的账号再删。" };
  }
  if (error.status === 404) return { message: "账号不在了，刷新页面后再试一次。" };
  if (error.status === 409) return { message: "这个账号名刚被占用，换一个试试。" };
  if (error.status === 401) return { message: "登录状态失效了，重新进入一次。" };
  if (error.kind === "timeout") return { message: "请求超时了，稍后再试一次。" };
  return { message: error.message || "没能连上服务，稍后再试。" };
}
// --- 账号选择器（登录页列表 + 侧栏切换入口） ---

/** 「切换账号」开关：/login?switch=1 时已登录也不再自动跳走。 */
export const AUTH_SWITCH_PARAM = "switch";

/** 是不是用户主动来换号（而不是被动重定向来登录）。 */
export function isSwitchMode(params: URLSearchParams): boolean {
  return params.get(AUTH_SWITCH_PARAM) === "1";
}

/**
 * 已登录的人打开登录页：默认立刻回来源页（保持鉴权重定向体验），
 * 只有带 ?switch=1 才留下来——那是用户自己点的「切换账号」。
 */
export function shouldLeaveLoginPage(loggedIn: boolean, switching: boolean): boolean {
  return loggedIn && !switching;
}

/** 侧栏「切换账号」的地址：带 ?switch=1 与回跳地址，换完号原地回来。 */
export function switchAccountPath(next: string): string {
  return `${loginPath(next)}&${AUTH_SWITCH_PARAM}=1`;
}

/** 「最后活跃」在列表里是一行字，绝对值放 title（悬停可查，不占版面）。 */
export function absoluteSeenTitle(raw: string | null | undefined): string {
  const ts = parseIso(raw);
  if (ts === null) return "建号后还没有登录过";
  const d = new Date(ts);
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/**
 * 「上次活跃」的相对说法：刚刚 / N 分钟前 / N 小时前 / 昨天 HH:mm / M月D日 / YYYY年M月D日。
 *
 * 分档理由是读的人只关心"是不是刚才那个人"：一小时内说分钟，当天说小时，
 * 再往前精确到分钟已无意义，换成日期。跨年保留年份。
 */
export function relativeSeenText(raw: string | null | undefined, now: number = Date.now()): string {
  const ts = parseIso(raw);
  if (ts === null) return "从未登录";
  const diff = now - ts;
  // 负数 = 时钟偏差（或后端时间略快）：按「刚刚」处理，不写成"未来"
  if (diff < MINUTE_MS) return "刚刚";
  if (diff < HOUR_MS) return `${Math.floor(diff / MINUTE_MS)} 分钟前`;

  const seen = new Date(ts);
  const today = new Date(now);
  if (sameDay(seen, today)) return `${Math.floor(diff / HOUR_MS)} 小时前`;

  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  if (sameDay(seen, yesterday)) return `昨天 ${pad2(seen.getHours())}:${pad2(seen.getMinutes())}`;

  if (seen.getFullYear() === today.getFullYear()) return `${seen.getMonth() + 1}月${seen.getDate()}日`;
  return `${seen.getFullYear()}年${seen.getMonth() + 1}月${seen.getDate()}日`;
}

/** 列表里那一行副标题：从没登录过就别写"上次活跃 从未登录"这种绕口说法。 */
export function seenLine(raw: string | null | undefined, now: number = Date.now()): string {
  const rel = relativeSeenText(raw, now);
  return rel === "从未登录" ? rel : `上次活跃 ${rel}`;
}

/**
 * 成功后的 toast：单入口下只有一种说法。
 *
 * 不再区分「创建了」与「进来了」：那是后端状态码（201/200）的差别，
 * 对读者是同一件事——「我现在是这个账号了」。分成两句只会让人以为选错了按钮。
 */
export function authToastText(name: string): string {
  return `已登录为 ${name}`;
}

/** 删账号成功后回哪：登录页（客户端已是未登录态）。 */
export const AUTH_AFTER_DELETE = "/login";

/** 列表里给当前账号打「当前」标记：按 id 比，不按名字（重名判断在后端，前端只认身份）。 */
export function isCurrentUser(candidate: { id: number }, current: { id: number } | null): boolean {
  return current !== null && candidate.id === current.id;
}

function pad2(value: number): string {
  return String(value).padStart(2, "0");
}

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/** 后端时间可能是空串、null 或不带时区的本地时间；解析不了就当「没有」。 */
function parseIso(raw: string | null | undefined): number | null {
  const value = (raw ?? "").trim();
  if (!value) return null;
  const ts = Date.parse(value);
  return Number.isNaN(ts) ? null : ts;
}
