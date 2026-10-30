import { useCallback, useMemo, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { ReadingHistoryResult } from "../../api/types";

/**
 * 历史页的删除交互（单条 + 清空）。
 *
 * 三个决定：
 * - **乐观更新**：点确认后立刻从列表里拿掉，不等后端往返——删除是"读者已经决定的事"，
 *   等一个往返再消失只会让人以为没点中。失败则把整份快照放回去并给 toast（回滚不是可选项）。
 * - **二段式内联确认**：不用 window.confirm——原生弹窗在 H5 上是浏览器自己的样式，
 *   而且它阻塞在主线程上，toast 会被压在后面；卡片就地变成「确认删除 / 取消」，误触代价只有一次点击。
 * - **一次只删一条**：删除中把其余入口一起禁用（busy !== null），避免两次删除的乐观状态互相覆盖。
 * - **404 当成功**：要删的记录本来就不在库里，说明手里的列表已经过时（别处删过 / 换过账号）。
 *   这时该做的是"让它从眼前消失"，弹一句"删除失败"只会让人反复点——所以照样移除并刷新。
 *
 * 页码回退的判定也在这里：本页删空且不在第一页 → 交给页面回退一页（重新取数），
 * 否则就地 reload 拿新的一页。
 */
export interface HistoryDeletion {
  /** 正在删除的目标：行用 bookId，清空用 "all"；null = 空闲。 */
  busy: number | "all" | null;
  /** 已展开二次确认的目标；null = 都没有展开。 */
  pending: number | "all" | null;
  /** 乐观删除后的数据视图；没删过就是 null（页面用原数据）。 */
  view: ReadingHistoryResult | null;
  /** 打开二次确认；再次点同一个目标视为取消。 */
  ask: (target: number | "all") => void;
  cancel: () => void;
  /** 确认执行；返回 true 表示后端已经删掉（页面据此决定是否回退页码）。 */
  confirm: () => Promise<boolean>;
}

export interface HistoryDeletionOptions {
  data: ReadingHistoryResult | null;
  reload: () => void;
  /** 本页删空时请求回退一页；返回 true 表示已经回退（页面不用再 reload）。 */
  onPageEmptied: () => boolean;
  notify: (text: string, tone?: "info" | "error") => void;
}

export function useHistoryDeletion(options: HistoryDeletionOptions): HistoryDeletion {
  const { data, reload, onPageEmptied, notify } = options;
  const [busy, setBusy] = useState<number | "all" | null>(null);
  const [pending, setPending] = useState<number | "all" | null>(null);
  // 乐观视图绑在"提交那一刻的数据快照"上：服务端新数据一到（data 换引用）派生结果就是 null，
  // 以它为准。这样既不用 effect 里 setState（渲染期派生，与 useResource 同一写法），
  // 也不会让本地猜测盖住库里的事实。
  const [optimistic, setOptimistic] = useState<{ source: ReadingHistoryResult; value: ReadingHistoryResult } | null>(
    null,
  );
  const view = optimistic && optimistic.source === data ? optimistic.value : null;

  const ask = useCallback((target: number | "all") => {
    setPending((current) => (current === target ? null : target));
  }, []);

  const cancel = useCallback(() => setPending(null), []);

  const confirm = useCallback(async () => {
    const target = pending;
    const snapshot = data;
    if (target === null || busy !== null || snapshot === null) return false;

    const removed = target === "all" ? snapshot.items.length : 1;
    const nextItems = target === "all" ? [] : snapshot.items.filter((row) => row.bookId !== target);
    setPending(null);
    setBusy(target);
    setOptimistic({ source: snapshot, value: { items: nextItems, total: Math.max(0, snapshot.total - removed) } });

    // 成功与"本来就不在"共用这一步：本页删空且能回退 → 交给页码变化触发重新取数；
    // 否则就地刷新（reload 不清空当前画面，不闪骨架）。
    const settle = () => {
      if (!(nextItems.length === 0 && onPageEmptied())) reload();
    };

    try {
      if (target === "all") await api.clearReadingHistory();
      else await api.deleteReadingHistory(target);
      settle();
      notify(target === "all" ? "已清空阅读历史。" : "已删除这条阅读记录。");
      return true;
    } catch (error) {
      // 后端说"当前账号没有这条记录"：目标状态已经达成，按成功收尾，只把列表拉回事实
      if (isAlreadyGone(error)) {
        settle();
        notify(target === "all" ? "已清空阅读历史。" : "这条记录已经不在了。");
        return true;
      }
      setOptimistic(null);
      notify((target === "all" ? "清空" : "删除") + "没成功：" + failureText(error), "error");
      return false;
    } finally {
      setBusy(null);
    }
  }, [busy, data, notify, onPageEmptied, pending, reload]);

  return useMemo(
    () => ({ busy, pending, view, ask, cancel, confirm }),
    [ask, busy, cancel, confirm, pending, view],
  );
}

/** 404 not_found：记录本来就不在库里（列表过时），不是失败。 */
function isAlreadyGone(error: unknown): boolean {
  return error instanceof ApiError && (error.status === 404 || error.code === "not_found");
}

/** 失败原因说中文：浏览器原生的 "Failed to fetch" 原样弹给读者等于没解释。 */
function failureText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.kind === "timeout") return "请求超时，稍后再试一次";
    if (error.kind === "network") return "没能连上服务，稍后再试一次";
    return error.message || "稍后再试一次";
  }
  return "稍后再试一次";
}
