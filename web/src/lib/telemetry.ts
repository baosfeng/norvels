import { useEffect, type RefObject } from "react";
import { api } from "../api/client";
import type { TrackEvent, TrackEventKind } from "../api/types";

/**
 * 埋点 SDK（docs/specs/2026-11-01-recommend-design.md §5）。
 *
 * 三条纪律：
 * 1. **不拖慢交互**：track 只入内存队列，1s 内的点击 / 曝光合并成一次 POST；
 * 2. **不打断页面**：发送失败不抛、不弹提示，网络类失败静默重试一次，4xx 直接丢（数据不被接受，重试无意义）；
 * 3. **卸载前不丢**：pagehide / 隐藏时先广播「收尾事件」让页面把停留秒数补进队列，再走 sendBeacon。
 *
 * 为什么不复用 useResource：埋点不参与列表的取消与骨架态；sendBeacon 又是浏览器给的独立通道
 * （同源自动带 Cookie，没有 AbortController 这一层），两件事凑不到同一个抽象里。
 */

/** 一批的上限：契约 ≤100 条/次，超过就拆成多批。 */
export const MAX_BATCH = 100;

/** 合并窗口：1s 内的事件合成一次 POST。 */
export const FLUSH_DELAY_MS = 1000;

/** 未发送队列的硬上限：长时间离线也不让它无限涨（丢最旧的，保留最近的信号）。 */
export const MAX_QUEUE = 500;

/** 曝光口径：可见比例 ≥50% 且持续 ≥500ms 才算看见（§5）。 */
export const IMPRESSION_RATIO = 0.5;
export const IMPRESSION_DWELL_MS = 500;

/**
 * pagehide 之前先广播这个事件：详情页停留这类「必须在卸载前入队」的收尾事件
 * 听到它就同步 track，随后整队一起 sendBeacon。
 * 用事件而不是让各处自己监听 pagehide：监听器的执行顺序由注册早晚决定，
 * 页面组件必然晚于 installTelemetry 注册，等它跑完队已经发走了。
 */
export const TELEMETRY_FINALE_EVENT = "norvels:telemetry-finale";

let queue: TrackEvent[] = [];
let timer: ReturnType<typeof setTimeout> | null = null;
let retryBudget = 1;
let inFlight = false;
let installed = false;

/**
 * 记一条事件：立即入队，最多 1s 后随批发出。
 * payload 只带**已知**维度；后端拿 bookId 反查作者与分类，这里不猜。
 */
export function track(kind: TrackEventKind, payload: Omit<TrackEvent, "kind"> = {}): void {
  if (queue.length >= MAX_QUEUE) queue.splice(0, queue.length - MAX_QUEUE + 1);
  queue.push({ ...payload, kind });
  schedule();
}

function schedule(): void {
  if (timer !== null || inFlight) return;
  timer = setTimeout(() => {
    timer = null;
    void flush();
  }, FLUSH_DELAY_MS);
}

/**
 * 把队列发出去。返回本次实际送出的条数（0 = 没发 / 失败）。
 *
 * 失败口径：4xx 是「这批数据后端不认」（非法 kind / 超长字段），重试没有意义，直接丢；
 * 网络 / 超时 / 5xx 静默重试一次，再失败就丢——埋点不能变成一条永远重试的死循环。
 */
export async function flush(): Promise<number> {
  if (inFlight || queue.length === 0) return 0;
  const batch = queue.splice(0, MAX_BATCH);
  inFlight = true;
  try {
    await api.postEvents(batch);
    retryBudget = 1;
    return batch.length;
  } catch (error) {
    if (retryBudget > 0 && !isRejected(error)) {
      retryBudget -= 1;
      queue = batch.concat(queue).slice(-MAX_QUEUE);
      schedule();
    }
    return 0;
  } finally {
    inFlight = false;
    if (queue.length > 0) schedule();
  }
}

/**
 * 页面要走了：先广播收尾事件（让组件把停留秒数补进队列），再把整队交给 sendBeacon。
 * sendBeacon 不可用（老浏览器 / 测试环境）时退回普通 flush —— 能发出去总比丢了好。
 */
export function flushOnHide(): void {
  try {
    window.dispatchEvent(new Event(TELEMETRY_FINALE_EVENT));
  } catch {
    /* 没有 window：没有收尾事件可广播 */
  }
  if (queue.length === 0) return;
  const batch = queue.splice(0);
  if (!sendBeacon(batch)) {
    queue = batch.concat(queue);
    void flush();
  }
}

function sendBeacon(events: TrackEvent[]): boolean {
  if (typeof navigator === "undefined" || typeof navigator.sendBeacon !== "function") return false;
  try {
    const body = new Blob([JSON.stringify({ events })], { type: "application/json" });
    return navigator.sendBeacon("/api/events", body);
  } catch {
    return false;
  }
}

/** 挂 pagehide / 隐藏监听：重复调用无副作用（main.tsx 调一次，测试按用例调）。 */
export function installTelemetry(): void {
  if (installed || typeof window === "undefined") return;
  installed = true;
  window.addEventListener("pagehide", flushOnHide);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") flushOnHide();
  });
}

/** 测试用：清空队列、计时器与状态（生产代码不要调用）。 */
export function resetTelemetry(): void {
  queue = [];
  retryBudget = 1;
  inFlight = false;
  if (timer !== null) clearTimeout(timer);
  timer = null;
}

/** 队列里还有多少条没发（测试断言 / 调试用）。 */
export function pendingEvents(): number {
  return queue.length;
}

/** 4xx = 数据本身不被接受：不重试。其余（网络 / 超时 / 5xx）值得再试一次。 */
function isRejected(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const status = (error as { status?: unknown }).status;
  return typeof status === "number" && status >= 400 && status < 500;
}

/**
 * 推荐卡的曝光：可见 ≥50% 且持续 ≥500ms 记一次 impression，每张卡只记一次。
 *
 * 用 IntersectionObserver 而不是滚动事件：卡片多时滚动回调会把主线程占满。
 * 测试 / 老浏览器没有 IntersectionObserver 时直接不记（宁可没有曝光，也不要错报）。
 */
export function useImpression(
  ref: RefObject<Element | null>,
  payload: { bookId: number; categoryL1?: string; categoryL2?: string },
  options: { enabled?: boolean } = {},
): void {
  const enabled = options.enabled ?? true;
  const { bookId, categoryL1, categoryL2 } = payload;

  useEffect(() => {
    const node = ref.current;
    if (!enabled || !node || !Number.isFinite(bookId) || bookId <= 0) return;
    if (typeof IntersectionObserver === "undefined") return;

    let dwell: ReturnType<typeof setTimeout> | null = null;
    let recorded = false;
    const clearDwell = () => {
      if (dwell === null) return;
      clearTimeout(dwell);
      dwell = null;
    };
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (recorded) return;
          const visible = entry.isIntersecting && entry.intersectionRatio >= IMPRESSION_RATIO;
          if (!visible) {
            clearDwell();
            continue;
          }
          if (dwell !== null) continue;
          dwell = setTimeout(() => {
            dwell = null;
            recorded = true;
            observer.disconnect();
            track("impression", { bookId, categoryL1, categoryL2 });
          }, IMPRESSION_DWELL_MS);
        }
      },
      { threshold: [IMPRESSION_RATIO] },
    );
    observer.observe(node);
    return () => {
      clearDwell();
      observer.disconnect();
    };
  }, [ref, enabled, bookId, categoryL1, categoryL2]);
}
