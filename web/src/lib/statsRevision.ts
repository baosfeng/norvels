/**
 * 左栏统计的跨组件失效通道：改变库存量（书 / 章 / 字数）的写操作成功后 bump 一次，
 * 正在用 useStats 的地方（侧栏统计、驾驶舱卡片）就重取一次，不整页刷新。
 *
 * 为什么不用 Context / 全局 store 库：这条通道只有「变了一下」这一个信号，
 * 一个版本号加一组监听就够了；模块级实现还能在 vitest 里直接断（见 statsRevision.test.ts），
 * 不必先挂组件。写法与 lib/toastStore 同一套。
 */

let revision = 0;
const listeners = new Set<() => void>();

/** 库存量变了：通知所有订阅者重取（幂等，多 bump 只是多一次重取）。 */
export function bumpStatsRevision(): void {
  revision += 1;
  for (const listener of listeners) listener();
}

/** 订阅失效通知，返回取消函数——useEffect 可以直接 return 它。 */
export function subscribeStatsRevision(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** 当前版本号：只给测试确认「确实 bump 过一次」，业务代码不读。 */
export function statsRevision(): number {
  return revision;
}
