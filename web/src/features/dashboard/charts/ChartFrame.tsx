import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

/**
 * 图表卡的外壳：标题 → 一句实算结论 → 图体，四态（读取中 / 失败 / 空 / 有数据）统一在这里处理。
 *
 * 三条口径（对应实测缺陷）：
 *   - 结论一行：说明文字超过一行就会把卡片撑高，同一行的两张卡高度参差；这里一行 + 省略号，
 *     全文放在 title 上（悬停可读）；
 *   - 图体固定 132px：所有图的图体等高，卡片高度只由「标题 + 一行结论 + 图体」决定；
 *   - 失败可重试：重试最短可见 240ms，连续失败 3 次补一句排查提示。
 */

/** 所有图的图体高度：等高是「同一行卡片不等高」的根治办法（标题 + 一行结论 + 132px 图体）。 */
export const CHART_BODY_H = 132;

export type ChartId =
  | "trend"
  | "cumulative"
  | "authors"
  | "buckets"
  | "reading"
  | "storage"
  | "chapterlen"
  | "reading-trend"
  | "events"
  | "interest"
  | "hot"
  | "funnel"
  | "reasonmix"
  | "categoryclicks";

/** 单张图的取数状态：与页面其它数据区同一套四态口径。 */
export interface ChartState {
  loading: boolean;
  failed: boolean;
  onRetry: () => void;
  /** 后端统计表正在刷新：数值可用，但提示用户「数据更新中」而不是报错 */
  stale?: boolean;
}

interface ChartFrameProps extends ChartState {
  id: ChartId;
  title: string;
  note: string;
  isEmpty: boolean;
  children: ReactNode;
}

/** 重试最短可见时长：点完立刻成功也要让「重试中…」被看见。 */
const RETRY_MIN_MS = 240;
/** 连续失败到这个次数就补一句排查提示（边界）。 */
const RETRY_HINT_AFTER = 3;

export function ChartFrame({ id, title, note, loading, failed, onRetry, isEmpty, stale, children }: ChartFrameProps) {
  const [retrying, setRetrying] = useState(false);
  const [attempts, setAttempts] = useState(0);
  const titleRef = useRef<HTMLHeadingElement | null>(null);
  const retriedRef = useRef(false);
  /** 重试最短可见时长的定时器：卸载时清掉，避免组件卸掉之后还 setState。 */
  const retryTimer = useRef<number | null>(null);

  /** 重试成功（失败态消失）后把焦点送回卡片标题——按钮那时已经不存在了。 */
  useEffect(() => {
    if (!retriedRef.current || failed || loading) return;
    retriedRef.current = false;
    titleRef.current?.focus();
  }, [failed, loading]);

  useEffect(
    () => () => {
      if (retryTimer.current !== null) window.clearTimeout(retryTimer.current);
    },
    [],
  );

  function retry() {
    if (retrying) return;
    setRetrying(true);
    const started = Date.now();
    onRetry();
    const wait = Math.max(0, RETRY_MIN_MS - (Date.now() - started));
    retryTimer.current = window.setTimeout(() => {
      setRetrying(false);
      setAttempts((n) => n + 1);
      retriedRef.current = true;
    }, wait);
  }

  return (
    <article className="chart-card" data-chart={id} data-testid={`chart:${id}`}>
      <div className="chart-head">
        <h3 tabIndex={-1} ref={titleRef}>
          {title}
        </h3>
        <p className="chart-note" title={note}>
          {loading ? "正在读取…" : note}
          {!loading && !failed && stale ? <span className="chart-stale">数据更新中</span> : null}
        </p>
      </div>
      <div className="chart-body">
        {loading || retrying ? (
          <div className="chart-empty" data-testid={`chart:${id}:loading`}>
            {retrying ? <RetrySkeleton /> : "正在读取…"}
          </div>
        ) : failed ? (
          <div className="chart-empty" data-testid={`chart:${id}:error`}>
            <span>数据暂不可用</span>
            <button
              className="load-more retry-btn"
              type="button"
              disabled={retrying}
              onClick={retry}
            >
              {retrying ? "重试中…" : "重试"}
            </button>
            {attempts >= RETRY_HINT_AFTER ? (
              <p className="inline-note" data-testid={`chart:${id}:retry-hint`}>
                多次失败，检查服务是否在运行。
              </p>
            ) : null}
          </div>
        ) : isEmpty ? (
          <div className="chart-empty" data-testid={`chart:${id}:empty`}>
            暂无数据
          </div>
        ) : (
          children
        )}
      </div>
    </article>
  );
}

/** 重试时的骨架块：卡片不许在等待里闪白（反馈）。 */
function RetrySkeleton() {
  return (
    <div className="retry-skeleton" aria-hidden="true" data-testid="chart-retry-skeleton">
      <span />
      <span />
      <span />
    </div>
  );
}
