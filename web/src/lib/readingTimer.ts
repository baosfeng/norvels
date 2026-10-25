import { useEffect } from "react";
import type { ReadingSessionInput } from "../api/types";

/**
 * 阅读时长计时器：只在页面可见时累计，按需取出并清零。
 *
 * 阅读器在「切章 / 页面隐藏 / 卸载」三个时机取出秒数并上报一次会话，
 * 不在滚动时高频上报——一次会话一行，避免 reading_sessions 写放大。
 */

/** 单次上报的最小秒数：翻页停留几十秒不值得写一行。 */
export const MIN_REPORT_SECONDS = 60;

/** 单次上报上限，与后端 seconds 1–86400 对齐。 */
export const MAX_REPORT_SECONDS = 86400;

/** 长时段兜底：连续读 5 分钟就落一次，进程被杀也不至于全丢。 */
export const HEARTBEAT_MS = 5 * 60 * 1000;

export interface ReadingTimer {
  /** 开始计时（重复调用无副作用）。 */
  start: (now: number) => void;
  /** 暂停累计（页面隐藏、失焦）。 */
  pause: (now: number) => void;
  /** 取出并清零累计秒数；不足 minSeconds 时返回 0 且不清零。 */
  take: (now: number, minSeconds?: number) => number;
  /** 当前累计秒数（不消费）。 */
  pending: (now: number) => number;
}

/** 创建计时器（纯逻辑，不依赖 DOM，便于单测）。 */
export function createReadingTimer(): ReadingTimer {
  let accumulated = 0;
  let startedAt: number | null = null;

  function settle(now: number): void {
    if (startedAt !== null) {
      accumulated += Math.max(0, now - startedAt);
      startedAt = now;
    }
  }

  return {
    start(now) {
      if (startedAt === null) startedAt = now;
    },
    pause(now) {
      if (startedAt === null) return;
      accumulated += Math.max(0, now - startedAt);
      startedAt = null;
    },
    take(now, minSeconds = MIN_REPORT_SECONDS) {
      settle(now);
      const seconds = Math.floor(accumulated / 1000);
      if (seconds < minSeconds) return 0;
      accumulated = 0;
      return Math.min(seconds, MAX_REPORT_SECONDS);
    },
    pending(now) {
      settle(now);
      return Math.floor(accumulated / 1000);
    },
  };
}

/**
 * 把计时器接到一次阅读上：可见时计时，切章/隐藏/卸载时上报。
 *
 * 切章时 effect 重建，清理函数会先把上一章的时长落盘，
 * 所以「读了三章」会得到三条会话，而不是一条跨章的糊涂账。
 */
export function useReadingSessionReporter(options: {
  bookId: number;
  chapterId: number;
  report: (body: ReadingSessionInput) => Promise<boolean>;
}): void {
  const { bookId, chapterId, report } = options;

  useEffect(() => {
    if (!Number.isFinite(bookId) || !Number.isFinite(chapterId) || bookId <= 0 || chapterId <= 0) return;
    const timer = createReadingTimer();
    let alive = true;

    const flush = () => {
      const seconds = timer.take(Date.now());
      if (seconds > 0 && alive) void report({ bookId, chapterId, seconds });
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") timer.start(Date.now());
      else {
        timer.pause(Date.now());
        flush();
      }
    };

    if (document.visibilityState === "visible") timer.start(Date.now());
    document.addEventListener("visibilitychange", onVisibility);
    const heartbeat = window.setInterval(flush, HEARTBEAT_MS);

    return () => {
      flush();
      alive = false;
      document.removeEventListener("visibilitychange", onVisibility);
      window.clearInterval(heartbeat);
    };
  }, [bookId, chapterId, report]);
}
