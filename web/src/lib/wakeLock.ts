/**
 * Screen Wake Lock：读长章时别让手机先睡着。
 *
 * 薄封装，只做两件事——申请与释放；**所有出口都静默**：
 * 不支持的浏览器、非安全上下文、系统省电策略拒绝，都只是「屏幕照旧会熄」，
 * 不该打断阅读，更不该把异常抛给调用方。
 *
 * 为什么不缓存「是否已持锁」这个布尔：系统切后台时会自己收走锁，
 * 真相只有一个——sentinel.released。本地再存一份就一定会和它漂移。
 */

/** 当前持有的 sentinel；没有就是 null。 */
let sentinel: WakeLockSentinel | null = null;

/** 取 navigator.wakeLock；老浏览器上没有这个属性。 */
function wakeLockApi(): WakeLock | null {
  if (typeof navigator === "undefined") return null;
  const api = (navigator as Navigator & { wakeLock?: WakeLock }).wakeLock;
  return typeof api?.request === "function" ? api : null;
}

/** 此刻真正握着的 sentinel：系统已经收回的（released）算没有。 */
export function currentScreenWakeLock(): WakeLockSentinel | null {
  return sentinel && !sentinel.released ? sentinel : null;
}

/**
 * 申请屏幕常亮锁。
 * 已经握着就直接返回 true，不重复申请（重复申请在部分实现上会直接拒绝）。
 * @returns 是否成功持锁；环境不支持或被拒一律 false。
 */
export async function requestScreenWakeLock(): Promise<boolean> {
  if (currentScreenWakeLock()) return true;
  const api = wakeLockApi();
  if (!api) return false;
  try {
    const next: WakeLockSentinel | null = (await api.request("screen")) ?? null;
    sentinel = next;
    return next !== null;
  } catch {
    sentinel = null;
    return false;
  }
}

/**
 * 释放屏幕常亮锁。
 * 本来就没锁也算成功：调用方只关心「现在不持锁」，不该为此写分支。
 * @returns 是否已确定释放；release 抛错时为 false。
 */
export async function releaseScreenWakeLock(): Promise<boolean> {
  const current = sentinel;
  sentinel = null;
  if (!current || current.released) return true;
  try {
    await current.release();
    return true;
  } catch {
    return false;
  }
}
