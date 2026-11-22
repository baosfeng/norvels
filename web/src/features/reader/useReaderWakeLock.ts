import { useEffect, useRef } from "react";
import {
  currentScreenWakeLock,
  releaseScreenWakeLock,
  requestScreenWakeLock,
} from "../../lib/wakeLock";

/**
 * 阅读器挂载期间保持屏幕常亮。
 *
 * 只给阅读器用（书架、目录页不需要读者一直盯着，没有理由替他们改屏幕行为）。
 * 不做开关：读者的诉求就是「别读到一半熄屏」，多一个设置项等于把决定推回去；
 * 拿不到锁就静默跳过，此时的表现与没有这段代码完全一致。
 *
 * 浏览器在切到后台时一定会收回锁（规范允许，省电优先），所以回到可见时要**重新申请**，
 * 而不是沿用内存里那个已经 released 的 sentinel。
 */
export function useReaderWakeLock(enabled = true): void {
  /** 已经挂过 release 监听的 sentinel：同一个对象只挂一次，回调计数才准。 */
  const watched = useRef(new WeakSet<WakeLockSentinel>());

  useEffect(() => {
    if (!enabled || typeof document === "undefined") return;
    let disposed = false;

    const acquire = async () => {
      // 在后台申请会被持久化策略直接拒绝：等回到前台由 visibilitychange 再发
      if (disposed || document.visibilityState === "hidden") return;
      const granted = await requestScreenWakeLock();
      const held = granted ? currentScreenWakeLock() : null;
      if (disposed) {
        // 等待期间组件已卸载：别把锁留给已经离开的页面
        void releaseScreenWakeLock();
        return;
      }
      if (!held || watched.current.has(held)) return;
      watched.current.add(held);
      // 只记录「锁没了」，不在这里抢回：被系统收回后立刻重申请会被省电策略拒绝
      held.addEventListener("release", () => watched.current.delete(held));
    };

    void acquire();

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") void acquire();
    };
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      disposed = true;
      document.removeEventListener("visibilitychange", onVisibilityChange);
      void releaseScreenWakeLock();
    };
  }, [enabled]);
}
