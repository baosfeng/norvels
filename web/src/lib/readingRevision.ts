/**
 * 阅读记录的跨组件失效通道：变更某个账号的阅读会话（删一条 / 清空）成功后 bump 一次，
 * 正在用 useReadingHistory / useReadingStats 的地方就重取一次，不整页刷新。
 *
 * 与 lib/statsRevision 分开是**有意的**：那条通道表示「库存量变了」（书 / 章 / 字数），
 * 影响的是侧栏与驾驶舱的库存卡片；这条表示「我读了多久变了」，影响的是阅读历史与
 * 「今天已读」。把两件事塞进一个版本号，删一条阅读记录会让库存卡片白重取一次。
 *
 * 为什么不用 Context / 全局 store 库：只有「变了一下」这一个信号，一个版本号加一组
 * 监听就够了；模块级实现还能在 vitest 里直接断，不必先挂组件。写法与 lib/toastStore 同一套。
 */

let revision = 0;
const listeners = new Set<() => void>();

/** 阅读记录变了：通知所有订阅者重取（幂等，多 bump 只是多一次重取）。 */
export function bumpReadingRevision(): void {
  revision += 1;
  for (const listener of listeners) listener();
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
