import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  buildChapterBuckets,
  buildChapterLength,
  buildImportTrend,
  buildReadingDist,
  buildStorage,
  buildTopAuthors,
  RADIUS,
  storageRatioNote,
} from "../../lib/charts";
import type { Bucket, ImportTrendPoint, ReadingProgress, Stats } from "../../api/types";

/**
 * 六张图：纯 SVG/CSS 手写，不引图表库（ADR-0001 §6）。
 * 每张图结构一致：标题 → 一句实算结论 → 图体。结论与图体由同一份转换结果生成，不会各说各话。
 * 四态由 ChartFrame 统一处理（读取中 / 失败 / 空 / 有数据），颜色全取 CSS 变量，深浅主题自动跟随。
 */

export type ChartId = "trend" | "authors" | "buckets" | "reading" | "storage" | "chapterlen";

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
  /** 第二句结论（可选）：同一份数据算出的补充说明，不重复主结论 */
  detail?: string;
  isEmpty: boolean;
  children: ReactNode;
}

/** 重试最短可见时长：点完立刻成功也要让「重试中…」被看见（契约 C-31）。 */
const RETRY_MIN_MS = 240;
/** 连续失败到这个次数就补一句排查提示（契约 C-31 边界）。 */
const RETRY_HINT_AFTER = 3;

function ChartFrame({ id, title, note, detail, loading, failed, onRetry, isEmpty, stale, children }: ChartFrameProps) {
  const [retrying, setRetrying] = useState(false);
  const [attempts, setAttempts] = useState(0);
  const titleRef = useRef<HTMLHeadingElement | null>(null);
  const retriedRef = useRef(false);

  /** 重试成功（失败态消失）后把焦点送回卡片标题——按钮那时已经不存在了。 */
  useEffect(() => {
    if (!retriedRef.current || failed || loading) return;
    retriedRef.current = false;
    titleRef.current?.focus();
  }, [failed, loading]);

  function retry() {
    if (retrying) return;
    setRetrying(true);
    const started = Date.now();
    onRetry();
    const wait = Math.max(0, RETRY_MIN_MS - (Date.now() - started));
    window.setTimeout(() => {
      setRetrying(false);
      setAttempts((n) => n + 1);
      retriedRef.current = true;
    }, wait);
  }

  return (
    <article className="chart-card" data-chart={id} data-testid={`chart:${id}`} data-od-id={`chart:${id}`}>
      <div className="chart-head">
        <h3 tabIndex={-1} ref={titleRef}>
          {title}
        </h3>
        <p className="chart-note">
          {loading ? "正在读取…" : note}
          {!loading && !failed && stale ? <span className="chart-stale">数据更新中</span> : null}
        </p>
        {!loading && !failed && detail ? (
          <p className="chart-note" data-testid={`chart:${id}:detail`}>
            {detail}
          </p>
        ) : null}
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
              data-od-id="C-31"
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

/** 重试时的骨架块：卡片不许在等待里闪白（契约 C-31 反馈）。 */
function RetrySkeleton() {
  return (
    <div className="retry-skeleton" aria-hidden="true" data-testid="chart-retry-skeleton">
      <span />
      <span />
      <span />
    </div>
  );
}

/* ── 1 近 14 天导入量（竖条） ── */
export function ImportTrendChart({ points, ...state }: { points: ImportTrendPoint[] } & ChartState) {
  const model = buildImportTrend(points);
  return (
    <ChartFrame id="trend" title="近 14 天导入量" note={model.note} isEmpty={model.empty} {...state}>
      <div className="bars">
        {model.data.bars.map((bar) => (
          <div className="bar-col" key={bar.label} title={bar.title}>
            {/* 0 值不画柱：CSS 的 min-height 会把它渲染成一条 2px 横线，连续多天没导入时纯噪音 */}
            {bar.value > 0 ? <span className="bar-fill" style={{ height: `${bar.heightPct}%` }} /> : null}
            <span className="bar-lbl">{bar.labelVisible ? bar.label : ""}</span>
          </div>
        ))}
      </div>
    </ChartFrame>
  );
}

/* ── 2 章节数分布（直方图） ── */
export function ChapterBucketsChart({ buckets, ...state }: { buckets: Bucket[] } & ChartState) {
  const model = buildChapterBuckets(buckets);
  return (
    <ChartFrame id="buckets" title="章节数分布" note={model.note} isEmpty={model.empty} {...state}>
      <div className="bars">
        {model.data.bars.map((bar) => (
          <div className="bar-col" key={bar.label} title={bar.title}>
            <span className="bar-val">{bar.value}</span>
            {bar.value > 0 ? <span className="bar-fill" style={{ height: `${bar.heightPct}%` }} /> : null}
            <span className="bar-lbl">{bar.label}</span>
          </div>
        ))}
      </div>
    </ChartFrame>
  );
}

/* ── 3 字数 Top 10 作者（横条） ── */
export function TopAuthorsChart({
  authors,
  totalWords,
  ...state
}: { authors: { name: string; wordCount: number }[]; totalWords: number } & ChartState) {
  const model = buildTopAuthors(authors, totalWords);
  return (
    <ChartFrame id="authors" title="字数 Top 10 作者" note={model.note} isEmpty={model.empty} {...state}>
      <div className="hbars">
        {model.data.rows.map((row) => (
          <div className="hbar-row" key={row.name}>
            <span className="hbar-name" title={row.name}>
              {row.name}
            </span>
            <span className="hbar-track">
              <span className="hbar-fill" style={{ width: `${row.widthPct}%` }} />
            </span>
            <span className="hbar-val">{row.display}</span>
          </div>
        ))}
      </div>
    </ChartFrame>
  );
}

/* ── 4 阅读进度分布（环形） ── */
export function ReadingDistChart({
  progress,
  totalBooks,
  limitReached,
  buckets,
  ...state
}: {
  progress: ReadingProgress[];
  totalBooks: number;
  limitReached: boolean;
  /** 后端聚合的三态；有则以它为准（精确），没有才回退到按最近记录估算 */
  buckets?: { unread: number; reading: number; finished: number } | null;
} & ChartState) {
  const model = buildReadingDist(progress, totalBooks, limitReached, buckets);
  const note = model.data.partial ? `${model.note}（按最近 50 条记录聚合）` : model.note;
  const [hidden, setHidden] = useState<Set<string>>(new Set());

  /** 图例过滤（契约 C-26）：只改这张图的显隐，不改任何数字，也不发请求。 */
  function toggleLegend(key: string) {
    setHidden((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  return (
    <ChartFrame id="reading" title="阅读进度分布" note={note} isEmpty={model.empty} {...state}>
      <div className="donut-wrap">
        <svg className="donut" viewBox="0 0 120 120" role="img" aria-label={note}>
          {model.data.segments.map((segment) =>
            segment.value > 0 ? (
              <circle
                key={segment.key}
                className="donut-seg"
                cx="60"
                cy="60"
                r={RADIUS}
                fill="none"
                stroke={segment.color}
                strokeWidth="16"
                strokeDasharray={segment.dash}
                strokeDashoffset={String(segment.offset)}
                transform="rotate(-90 60 60)"
                opacity={hidden.has(segment.key) ? 0.16 : 1}
              >
                <title>{`${segment.label} ${segment.value} 本`}</title>
              </circle>
            ) : null,
          )}
        </svg>
        <div className="donut-legend">
          {model.data.segments.map((segment) => (
            <button
              className={`legend-row legend-btn${hidden.has(segment.key) ? " is-off" : ""}`}
              key={segment.key}
              type="button"
              data-od-id="C-26"
              data-testid={`legend-${segment.key}`}
              aria-pressed={!hidden.has(segment.key)}
              aria-label={`${segment.label} ${segment.value} 本`}
              onClick={() => toggleLegend(segment.key)}
            >
              <span className="legend-dot" style={{ background: segment.color }} />
              {segment.label} {segment.value} 本
            </button>
          ))}
          {hidden.size > 0 ? (
            <p className="inline-note" data-testid="legend-hidden-note">
              已隐藏的段不画在环上，总数不变。
            </p>
          ) : null}
        </div>
      </div>
    </ChartFrame>
  );
}

/* ── 5 库体积构成（堆叠条） ── */
export function StorageChart({ stats, ...state }: { stats: Stats | null } & ChartState) {
  const model = buildStorage(stats);
  return (
    <ChartFrame
      id="storage"
      title="库体积构成"
      note={model.note}
      detail={storageRatioNote(model.data)}
      isEmpty={model.empty}
      {...state}
    >
      <div style={{ width: "100%" }}>
        <div className="stack">
          {model.data.parts.map((part) => (
            // 三段宽度已归一化到总和 100，堆叠条铺满且不溢出
            <span
              className="stack-seg"
              key={part.key}
              style={{ width: `${part.widthPct}%`, background: part.color }}
              title={`${part.label} ${part.display}`}
            />
          ))}
        </div>
        <div className="stack-legend">
          {/* 占比不足 0.1% 的段不出图例：真实库里「其他」只有 8 KB（页头、空闲页），
              按 1024 进制四舍五入成「0.0 MB」——一行 0 字节的图例只会让人怀疑漏统计 */}
          {model.data.parts
            .filter((part) => part.bytes > 0 && part.sharePct >= 0.1)
            .map((part) => (
              <span className="legend-row" key={part.key}>
                <span className="legend-dot" style={{ background: part.color }} />
                {part.label} {part.display}
              </span>
            ))}
        </div>
      </div>
    </ChartFrame>
  );
}

/* ── 6 章节长度分布 ── */
export function ChapterLengthChart({ buckets, ...state }: { buckets: Bucket[] } & ChartState) {
  const model = buildChapterLength(buckets);
  return (
    <ChartFrame id="chapterlen" title="章节长度分布" note={model.note} isEmpty={model.empty} {...state}>
      <div className="cols">
        {model.data.cols.map((col) => (
          <div className="col-item" key={col.label} title={col.title}>
            {col.value > 0 ? (
              <span
                className={`col-fill${col.label === model.data.peak ? " is-peak" : ""}`}
                style={{ height: `${col.heightPct}%` }}
              />
            ) : null}
            <span className="bar-lbl">{col.label}</span>
          </div>
        ))}
      </div>
    </ChartFrame>
  );
}