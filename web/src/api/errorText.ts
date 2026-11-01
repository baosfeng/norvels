import type { ApiError } from "./client";

/**
 * 后端错误码 → 前端语言的唯一映射（code 事实源在 internal/api/errors.go）。
 *
 * 之前所有 503 都说成「数据库不可用」：单章正文损坏（content_corrupted）也是 503，
 * 于是局部故障被说成整库故障，把用户引向「等数据库恢复」。这里按 code 分流：
 * 先问「该在哪儿说」（levelOf），再问「说什么」（codeToText），组件不再自己拼文案。
 * 同一个 code 的文案只在这里定义一次。
 */

/** 展示层级：整页 / 当前区块 / 当前章 / 行内。 */
export type ErrorLevel = "page" | "section" | "chapter" | "inline";

/** 层级表：只有需要跨区块改变页面结构的 code 才登记，其余默认是按行的「区块」。 */
const LEVELS: Record<string, ErrorLevel> = {
  unauthorized: "page", // 身份没了：留在原页面只会继续失败，去登录
  unavailable: "section", // 这项能力没启用：只影响当前区块
  content_corrupted: "chapter", // 只有这一章的正文坏了：退到下一章还能读
  timeout: "inline", // 等太久：提示缩小范围
  query_too_short: "inline", // 搜索框下方
  invalid_param: "inline", // 表单字段
  invalid_id: "inline", // 表单字段
  not_appendable: "inline", // 导入文件卡片
  review_state: "inline", // 行内提示刷新
  method_not_allowed: "inline", // 前端 bug：不外显技术细节
  not_found: "inline", // 同上（接口不存在）
  internal: "inline",
  network: "inline",
  aborted: "inline",
  parse: "inline",
};

/** 未登录 / 身份过期：整页层级唯一的出口。 */
export const LOGIN_PATH = "/login";

const TEXTS: Record<string, string> = {
  content_corrupted: "这一章的正文损坏了，打不开；可以先去读下一章，这本书的其它章节不受影响。",
  unavailable: "这项功能尚未启用，暂时用不了。",
  timeout: "读取超时了，稍后再试一次；范围大的话缩小范围再试。",
  unauthorized: "登录状态失效了，重新登录后就能继续。",
  method_not_allowed: "这个操作暂时不支持，刷新页面后再试。",
  invalid_id: "要操作的内容找不到了，刷新页面后再试。",
  invalid_param: "填写的内容不符合要求，按提示改一下再提交。",
  not_found: "要找的内容不在了，刷新页面后再试。",
  not_appendable: "这个文件和已有的章节对不上，换回正确的文件再试。",
  review_state: "这条记录的状态变了，刷新后看最新状态。",
  query_too_short: "搜索词太短了，多输入几个字再搜。",
  internal: "服务出了点问题，稍后再试。",
  network: "网络不太顺，检查一下网络再试。",
  aborted: "这次请求已经取消。",
  parse: "返回的内容识别不了，稍后再试。",
};

/** 没给 code、也没给状态时的一句话兜底：不许空白，也不许出现 undefined。 */
const FALLBACK = "读取时出了点问题，可以重试，或者稍后再回来看看。";
/** 只有状态码没有 code：503 既不等于整库故障，也不指向数据库。 */
const SERVICE_UNAVAILABLE = "服务暂时不可用，稍后再试。";

/**
 * 客户端侧失败（超时 / 断网 / 解析）没有后端 code，用 kind 顶替同一个 code 空间，
 * 免得每个组件各写一遍 kind → 文案的映射。
 */
export function effectiveCode(error: ApiError | null | undefined): string | undefined {
  if (!error) return undefined;
  if (error.code) return error.code;
  if (error.kind === "http") return undefined;
  return error.kind;
}

/** 该在哪儿说：整页 / 当前区块 / 当前章 / 行内；未知 code 按当前区块处理。 */
export function levelOf(code?: string | null): ErrorLevel {
  if (!code) return "section";
  return LEVELS[code] ?? "section";
}

/**
 * 说什么：code 优先，其次按状态码给最小口径，最后兜底。
 * 传了 code 就一定按 code 说话——503 + content_corrupted 不能说成服务不可用。
 */
export function codeToText(code?: string | null, status?: number | null): string {
  const known = code ? TEXTS[code] : undefined;
  if (known) return known;
  if (status === 401) return TEXTS.unauthorized;
  if (status === 503) return SERVICE_UNAVAILABLE;
  return FALLBACK;
}
