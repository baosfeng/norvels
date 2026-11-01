import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  buildChapterBuckets,
  buildChapterLength,
  buildEventTrend,
  buildFunnel,
  buildHotBooks,
  buildCategoryClicks,
  buildImportTrend,
  buildInterestBars,
  buildReasonMix,
  buildReadingDist,
  buildStorage,
  buildTopAuthors,
  compact,
  RADIUS,
  storageRatioNote,
} from "../../lib/charts";
import type {
  Bucket,
  ImportTrendPoint,
  ReadingProgress,
  RecommendCategoryClicks,
  RecommendHotBook,
  RecommendInterest,
  RecommendReasonMix,
  RecommendStatsDay,
  RecommendStatsSummary,
  Stats,
} from "../../api/types";
import { ChartTipLayer } from "./ChartTip";
import { useChartTip } from "./useChartTip";

/**
 * 十二张图：纯 SVG/CSS 手写，不引图表库（ADR-0001 §6）。
 * 每张图结构一致：标题 → 一句实算结论 → 图体。结论与图体由同一份转换结果生成，不会各说各话。
 * 四态由 ChartFrame 统一处理（读取中 / 失败 / 空 / 有数据），颜色全取 CSS 变量，深浅主题自动跟随。
 *
 * 前六张是库概览（导入 / 章节 / 作者 / 进度 / 体积 / 章节长度），中间四张是「推荐与埋点」
 * （事件趋势 / 兴趣画像 / 热门书 / 漏斗），最后两张是同一区块的分布（点击理由 / 分类点击）
 * ——全部复用同一套 ChartFrame 与读数浮层，不另起一套。
 */

export type ChartId =
  | "trend"
  | "authors"
  | "buckets"
  | "reading"
  | "storage"
  | "chapterlen"
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
  const tip = useChartTip();
  return (
    <ChartFrame id="trend" title="近 14 天导入量" note={model.note} isEmpty={model.empty} {...state}>
      <div className="bars">
        {/* 整列都是读数目标：0 值那天没有柱子，但「这天没有导入」同样要能读出来 */}
        {model.data.bars.map((bar) => (
          <div className="bar-col" key={bar.label} {...tip.bind(bar.title)}>
            {/* 0 值不画柱：CSS 的 min-height 会把它渲染成一条 2px 横线，连续多天没导入时纯噪音 */}
            {bar.value > 0 ? <span className="bar-fill" style={{ height: `${bar.heightPct}%` }} /> : null}
            <span className="bar-lbl">{bar.labelVisible ? bar.label : ""}</span>
          </div>
        ))}
      </div>
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 2 章节数分布（直方图） ── */
export function ChapterBucketsChart({ buckets, ...state }: { buckets: Bucket[] } & ChartState) {
  const model = buildChapterBuckets(buckets);
  const tip = useChartTip();
  return (
    <ChartFrame id="buckets" title="章节数分布" note={model.note} isEmpty={model.empty} {...state}>
      <div className="bars">
        {model.data.bars.map((bar) => (
          <div className="bar-col" key={bar.label} {...tip.bind(bar.title)}>
            <span className="bar-val">{bar.value}</span>
            {bar.value > 0 ? <span className="bar-fill" style={{ height: `${bar.heightPct}%` }} /> : null}
            <span className="bar-lbl">{bar.label}</span>
          </div>
        ))}
      </div>
      <ChartTipLayer tip={tip.tip} />
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
  const tip = useChartTip();
  return (
    <ChartFrame id="authors" title="字数 Top 10 作者" note={model.note} isEmpty={model.empty} {...state}>
      <div className="hbars">
        {model.data.rows.map((row) => (
          <div className="hbar-row" key={row.name} {...tip.bind(row.title)}>
            <span className="hbar-name">{row.name}</span>
            <span className="hbar-track">
              <span className="hbar-fill" style={{ width: `${row.widthPct}%` }} />
            </span>
            <span className="hbar-val">{row.display}</span>
          </div>
        ))}
      </div>
      <ChartTipLayer tip={tip.tip} />
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
  const tip = useChartTip();

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
                {...tip.bind(segment.title)}
              />
            ) : null,
          )}
        </svg>
        <ChartTipLayer tip={tip.tip} />
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
  const tip = useChartTip();
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
              {...tip.bind(part.title)}
            />
          ))}
        </div>
        <ChartTipLayer tip={tip.tip} />
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
  const tip = useChartTip();
  return (
    <ChartFrame id="chapterlen" title="章节长度分布" note={model.note} isEmpty={model.empty} {...state}>
      <div className="cols">
        {model.data.cols.map((col) => (
          <div className="col-item" key={col.label} {...tip.bind(col.title)}>
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
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 7 近 N 天埋点事件（三色柱） ── */
export function EventTrendChart({ daily, ...state }: { daily: RecommendStatsDay[] } & ChartState) {
  const model = buildEventTrend(daily);
  const tip = useChartTip();
  return (
    <ChartFrame
      id="events"
      title="近 14 天埋点事件"
      note={model.note}
      detail="三色柱各自按峰值归一，浮层给的是当天真实次数。"
      isEmpty={model.empty}
      {...state}
    >
      <div className="etrend-wrap">
        <div className="etrend">
          {model.data.days.map((day) => (
            // 整列都是读数目标：哪一根为 0 也要能读出「这天没有」
            <div className="etrend-col" key={day.date} {...tip.bind(day.title)}>
              <span className="etrend-bars">
                {day.impressions > 0 ? (
                  <i className="etrend-bar is-impression" style={{ height: `${day.impressionPct}%` }} />
                ) : null}
                {day.clicks > 0 ? (
                  <i className="etrend-bar is-click" style={{ height: `${day.clickPct}%` }} />
                ) : null}
                {day.reads > 0 ? <i className="etrend-bar is-read" style={{ height: `${day.readPct}%` }} /> : null}
              </span>
              <span className="bar-lbl">{day.labelVisible ? day.label : ""}</span>
            </div>
          ))}
        </div>
        <ChartTipLayer tip={tip.tip} />
        <div className="etrend-legend">
          <span className="legend-row">
            <span className="legend-dot is-impression" />
            曝光 {compact(model.data.impressions)}
          </span>
          <span className="legend-row">
            <span className="legend-dot is-click" />
            点击 {compact(model.data.clicks)}（{model.data.ctr}）
          </span>
          <span className="legend-row">
            <span className="legend-dot is-read" />
            阅读 {compact(model.data.reads)} · {compact(model.data.readMinutes)} 分钟
          </span>
        </div>
      </div>
    </ChartFrame>
  );
}

/* ── 8 兴趣画像（分类 / 作者 两组横条） ── */
export function InterestBarsChart({ interests, ...state }: { interests: RecommendInterest[] } & ChartState) {
  const model = buildInterestBars(interests);
  const tip = useChartTip();
  return (
    <ChartFrame id="interest" title="兴趣画像" note={model.note} isEmpty={model.empty} {...state}>
      <div className="ibars">
        {model.data.groups.map((group) => (
          <div
            className="ibars-group"
            key={group.dim}
            data-dim={group.dim}
            data-testid={`ibars-${group.dim}`}
          >
            <span className="ibars-label">{group.label}</span>
            {group.rows.map((row) => (
              <div className="hbar-row" key={`${group.dim}-${row.name}`} {...tip.bind(row.title)}>
                <span className="hbar-name">{row.name}</span>
                <span className="hbar-track">
                  <span className="hbar-fill" style={{ width: `${row.widthPct}%` }} />
                </span>
                <span className="hbar-val">{row.display}</span>
              </div>
            ))}
          </div>
        ))}
      </div>
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 9 热门书 Top 10（横条，读过 2 分 / 点击 1 分） ── */
export function HotBooksChart({ books, ...state }: { books: RecommendHotBook[] } & ChartState) {
  const model = buildHotBooks(books);
  const tip = useChartTip();
  return (
    <ChartFrame id="hot" title="热门书 Top 10" note={model.note} isEmpty={model.empty} {...state}>
      <div className="hbars hot-books">
        {model.data.rows.map((row) => (
          <div className="hbar-row" key={row.name} {...tip.bind(row.title)}>
            <span className="hbar-name">{row.name}</span>
            <span className="hbar-track">
              <span className="hbar-fill" style={{ width: `${row.widthPct}%` }} />
            </span>
            <span className="hbar-val">{row.display}</span>
          </div>
        ))}
      </div>
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 10 曝光 → 点击 → 阅读 → 收藏（漏斗） ── */
export function FunnelChart({ summary, ...state }: { summary: RecommendStatsSummary } & ChartState) {
  const model = buildFunnel(summary);
  const tip = useChartTip();
  return (
    <ChartFrame
      id="funnel"
      title="曝光到收藏"
      note={model.note}
      detail="条长按曝光归一，右侧是相对上一级的转化率。"
      isEmpty={model.empty}
      {...state}
    >
      <div className="funnel-wrap">
        <div className="funnel">
          {model.data.steps.map((step) => (
            <div className="funnel-row" key={step.key} {...tip.bind(step.title)}>
              <span className="funnel-label">{step.label}</span>
              <span className="funnel-track">
                <span className={`funnel-fill is-${step.key}`} style={{ width: `${step.widthPct}%` }} />
              </span>
              <span className="funnel-value num">{compact(step.value)}</span>
              <span className="funnel-rate">{step.stepRate}</span>
            </div>
          ))}
        </div>
        <ChartTipLayer tip={tip.tip} />
      </div>
    </ChartFrame>
  );
}

/* ── 11 点击理由分布（横向条，理由长 → 名字列给宽 + 浮层看全文） ── */
export function ReasonMixChart({ rows, ...state }: { rows: RecommendReasonMix[] } & ChartState) {
  const model = buildReasonMix(rows);
  const tip = useChartTip();
  return (
    <ChartFrame
      id="reasonmix"
      title="点击理由分布"
      note={model.note}
      detail="条长按点击量；浮层给这条理由的曝光与点击率。"
      isEmpty={model.empty}
      {...state}
    >
      <div className="hbars reason-bars">
        {model.data.rows.map((row) => (
          <div className="hbar-row" key={row.name} {...tip.bind(row.title)}>
            <span className="hbar-name">{row.name}</span>
            <span className="hbar-track">
              <span className="hbar-fill" style={{ width: `${row.widthPct}%` }} />
            </span>
            <span className="hbar-val">{row.display}</span>
          </div>
        ))}
      </div>
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 12 分类点击分布（横向条） ── */
export function CategoryClicksChart({ rows, ...state }: { rows: RecommendCategoryClicks[] } & ChartState) {
  const model = buildCategoryClicks(rows);
  const tip = useChartTip();
  return (
    <ChartFrame
      id="categoryclicks"
      title="分类点击分布"
      note={model.note}
      detail="只看大分类：点进来的是哪个类型，一眼看得出你最近在追什么。"
      isEmpty={model.empty}
      {...state}
    >
      <div className="hbars reason-bars">
        {model.data.rows.map((row) => (
          <div className="hbar-row" key={row.name} {...tip.bind(row.title)}>
            <span className="hbar-name">{row.name}</span>
            <span className="hbar-track">
              <span className="hbar-fill" style={{ width: `${row.widthPct}%` }} />
            </span>
            <span className="hbar-val">{row.display}</span>
          </div>
        ))}
      </div>
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

