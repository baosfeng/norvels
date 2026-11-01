import { useEffect } from "react";
import type { BookDetail } from "../../api/types";
import { createReadingTimer } from "../../lib/readingTimer";
import { TELEMETRY_FINALE_EVENT, track } from "../../lib/telemetry";

/** 只带埋点需要的字段：详情页拿到 BookDetail 就够，不多取一次。 */
type ViewedBook = Pick<BookDetail, "id" | "authorId"> &
  Partial<Pick<BookDetail, "categoryL1" | "categoryL2">>;

/**
 * 详情页停留埋点：离开时上报一条 `view`，value = 可见停留秒数。
 *
 * 设计 §5 写的是「挂载记 start、卸载记 view」，但 `start` 不在 §2/§4 的 kind 白名单里，
 * 后端会整批 400（见 api/types 的 TrackEventKind 注释）。这里只发 view：
 * 一条事件就够表达「进来过、待了多久」，且不会牵连同批次的其他信号。
 *
 * 「离开」有三个入口，合成一条 view（只报一次）：
 * - 组件卸载（SPA 内跳走）——最常见；
 * - pagehide（关标签 / 移动端切走）——卸载不会发生，所以走 telemetry 的收尾事件，
 *   它保证在 sendBeacon 之前同步入队；
 * - 中途隐藏再回来不算离开：隐藏期间暂停计时（复用 readingTimer，与阅读时长同一口径）。
 */
export function useBookView(book: ViewedBook | null): void {
  const bookId = book?.id ?? 0;
  const authorId = book?.authorId;
  const categoryL1 = book?.categoryL1;
  const categoryL2 = book?.categoryL2;

  useEffect(() => {
    if (!bookId) return;
    const payload = { bookId, authorId, categoryL1, categoryL2 };
    const timer = createReadingTimer();
    const visible = () => document.visibilityState !== "hidden";
    if (visible()) timer.start(Date.now());

    let reported = false;
    const report = () => {
      if (reported) return;
      reported = true;
      track("view", { ...payload, value: timer.pending(Date.now()) });
    };
    const onVisibility = () => {
      if (visible()) timer.start(Date.now());
      else timer.pause(Date.now());
    };

    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener(TELEMETRY_FINALE_EVENT, report);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener(TELEMETRY_FINALE_EVENT, report);
      report();
    };
  }, [bookId, authorId, categoryL1, categoryL2]);
}
