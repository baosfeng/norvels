/**
 * 本设备能力自检：此刻这台设备、这个地址上，哪些浏览器能力真的能用。
 *
 * 所有端都是「打开浏览器访问 NAS 上的服务」，能力随地址而变：https / localhost 是安全上下文，
 * 屏幕常亮、离线缓存、持久化存储都在；http://192.168.x.x 这类局域网地址不是，
 * 它们会静默消失——读者只看到「这功能没用」，没有任何提示。
 *
 * 清单只收**界面里真有消费点**的能力：剪贴板与分享全仓零调用，列进来等于承诺一个
 * 界面上找不到入口的功能。
 *
 * 这里只做**判定与说明**，不碰任何能力的调用路径：调用方照旧静默降级，面板只负责让人看清原因。
 * 判定写成纯函数（环境由参数传入），组件不直接读 navigator，便于单测。
 */

/** 能力标识；顺序即面板展示顺序。 */
export type CapabilityId = "secureContext" | "wakeLock" | "serviceWorker" | "storagePersist";

/** 一条自检结果。 */
export interface CapabilityReport {
  id: CapabilityId;
  /** 面板里展示的名字。 */
  label: string;
  available: boolean;
  /** 不可用时的一句话：怎么才能可用。可用时为空串。 */
  hint: string;
}

/**
 * 判定所需的最小环境：浏览器 navigator 的一个子集。
 * 全部可选——属性缺失（老内核、非浏览器）按不可用算，这正是要让人看见的情形。
 */
export interface CapabilityEnv {
  isSecureContext?: boolean;
  wakeLock?: { request?: unknown };
  serviceWorker?: { register?: unknown };
  storage?: { persist?: unknown };
}

/** 非安全上下文里各项的统一归因：问题在地址，不在浏览器。离线徽标也复用它。 */
export const NEED_SECURE_CONTEXT_HINT = "需 https 或 localhost 访问";
const NEED_BROWSER = "当前浏览器不支持";

function hasFunction(value: unknown): boolean {
  return typeof value === "function";
}

/** 能力清单：一条一行。probe 只看「这个环境里有没有入口」。 */
const CHECKS: { id: CapabilityId; label: string; probe: (env: CapabilityEnv) => boolean }[] = [
  { id: "secureContext", label: "安全上下文", probe: (env) => env.isSecureContext === true },
  { id: "wakeLock", label: "屏幕常亮（Wake Lock）", probe: (env) => hasFunction(env.wakeLock?.request) },
  {
    id: "serviceWorker",
    label: "离线缓存（Service Worker）",
    probe: (env) => hasFunction(env.serviceWorker?.register),
  },
  { id: "storagePersist", label: "持久化存储", probe: (env) => hasFunction(env.storage?.persist) },
];

/** 不可用时该说什么：安全上下文缺失优先归因到地址，否则才归因到浏览器。 */
function hintOf(id: CapabilityId, secure: boolean): string {
  return id === "secureContext" || !secure ? NEED_SECURE_CONTEXT_HINT : NEED_BROWSER;
}

/**
 * 逐项判定。安全上下文是这四项的共同前提，缺了它其余一律按不可用算——
 * 即使某个 API 对象还在，调下去也只是静默失败，说「可用」会让人误判。
 */
export function detectCapabilities(env: CapabilityEnv): CapabilityReport[] {
  const secure = env.isSecureContext === true;
  return CHECKS.map(({ id, label, probe }) => {
    const available = probe(env) && (secure || id === "secureContext");
    return { id, label, available, hint: available ? "" : hintOf(id, secure) };
  });
}

/** 浏览器里的真实环境：组件只需调这一个函数，不直接抛 navigator 属性。 */
export function browserCapabilityEnv(): CapabilityEnv {
  if (typeof window === "undefined") return {};
  const nav = window.navigator as Navigator & {
    wakeLock?: { request?: unknown };
    storage?: { persist?: unknown };
  };
  return {
    isSecureContext: window.isSecureContext,
    wakeLock: nav.wakeLock,
    serviceWorker: nav.serviceWorker,
    storage: nav.storage,
  };
}
