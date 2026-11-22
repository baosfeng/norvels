/**
 * 阅读记录的跨组件失效通道：变更某个账号的阅读会话（删一条 / 清空 / 翻章写进度）成功后
 * bump 一次，正在用 useReadingHistory / useReadingStats 的地方就重取一次，不整页刷新。
 *
 * 与 lib/statsRevision 分开是**有意的**：那条通道表示「库存量变了」（书 / 章 / 字数），
 * 影响的是侧栏与驾驶舱的库存卡片；这条表示「我读了多久变了」，影响的是阅读历史与
 * 「今天已读」。把两件事塞进一个版本号，删一条阅读记录会让库存卡片白重取一次。
 *
 * 为什么不用 Context / 全局 store 库：只有「变了一下」这一个信号，一个版本号加一组
 * 监听就够了；模块级实现还能在 vitest 里直接断，不必先挂组件。写法与 lib/toastStore 同一套。
 *
 * 为什么要跨标签页：同一本书在两个标签页各开一个阅读器（或者一边读书、一边开着书架），
 * 写完进度只有本页的 listeners 知道，另一个标签页的「继续阅读 / 书架进度」会一直是旧值。
 * 所以 bump 时额外广播一条**不含业务数据**的信号，别的标签页收到后各自重取。
 */

let revision = 0;
const listeners = new Set<() => void>();

/** 通道名沿用 norvels- 前缀（与导出的 norvels-bookmarks-* 同一套命名），
 *  在 devtools 的 BroadcastChannel 面板里能一眼认出是本应用的。 */
const READING_CHANNEL = "norvels-reading-revision";

/** 信号只带一个类型标记：不传书 / 章节 / 账号，两边就不会因为「谁的口径对」分叉；
 *  收到方唯一能做的事是「按自己的账号重取一次」。 */
type ReadingSignal = { type: "reading" };

/**
 * 常驻一个通道实例，不在 subscribe 里开关：
 * - 每次订阅都 new + close，会在组件频繁挂载卸载（切章、开关抽屉）时反复建通道；
 * - close 之后再广播就收不到，反过来要求「有订阅者时通道必须已经开着」，等价于常驻；
 * - 空闲通道的成本只是一个事件源，订阅数归零也不 close，下次订阅立刻可用。
 */
const channel = openChannel();

function openChannel(): BroadcastChannel | null {
  // jsdom 与老浏览器没有这个构造函数：退化成纯本地通知，不抛、不影响调用方。
  if (typeof BroadcastChannel === "undefined") return null;
  try {
    const opened = new BroadcastChannel(READING_CHANNEL);
    // 收到别的标签页的信号：只通知本地订阅者，**不再 bump、也不再 postMessage**。
    // 否则 A 收到 B 的消息后又广播一次，两个标签页会来回弹成死循环。
    opened.onmessage = () => notify();
    return opened;
  } catch {
    // 极少数内嵌 webview / 严格隐私模式下构造会抛：同样退化成本地通知。
    return null;
  }
}

function notify(): void {
  for (const listener of listeners) listener();
}

/** 阅读记录变了：通知所有订阅者重取（幂等，多 bump 只是多一次重取），并告知其它标签页。 */
export function bumpReadingRevision(): void {
  revision += 1;
  notify();
  // 通道默认不回送自己，所以这里不会让本页的 listeners 再响一次（用例里守着这一点）。
  channel?.postMessage({ type: "reading" } satisfies ReadingSignal);
}

/** 订阅失效通知，返回取消函数——useEffect 可以直接 return 它。 */
export function subscribeReadingRevision(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** 当前版本号：只给测试确认「确实 bump 过一次」，业务代码不读。 */
export function readingRevision(): number {
  return revision;
}
