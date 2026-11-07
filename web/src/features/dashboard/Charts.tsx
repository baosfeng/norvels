import { useState } from "react";
import {
  buildChapterBuckets,
  buildChapterLength,
  buildImportCumulative,
  buildImportTrend,
  buildReadingDist,
  buildReadingTrend,
  buildStorage,
  buildTopAuthors,
  compact,
  RADIUS,
  storageRatioNote,
} from "../../lib/charts";
import type { Bucket, DaySeconds, ImportTrendPoint, ReadingProgress, Stats } from "../../api/types";
import { BarChart } from "./charts/BarChart";
import { CHART_BODY_H, ChartFrame, type ChartState } from "./charts/ChartFrame";
import { ColumnChart } from "./charts/ColumnChart";
import { LineChart } from "./charts/LineChart";
import { StackBar } from "./charts/StackBar";
import { ChartTipLayer } from "./ChartTip";
import { useChartTip } from "./useChartTip";

/**
 * 库概览的七张图：纯 SVG 手写，不引图表库（ADR-0001 §6）。
 * 几何全部走 lib/chartScale.ts + charts/ 内核，卡片只负责「取哪份数、说哪句话」。
 *
 * 每张图结构一致：标题 → 一句实算结论 → 图体。结论与图体由同一份转换结果生成，不会各说各话。
 * 四态由 ChartFrame 统一处理（读取中 / 失败 / 空 / 有数据），颜色全取 CSS 变量，深浅主题自动跟随。
 */

export { CHART_BODY_H, ChartFrame, type ChartId, type ChartState } from "./charts/ChartFrame";
export {
  CategoryClicksChart,
  EventTrendChart,
  FunnelChart,
  HotBooksChart,
  InterestBarsChart,
  ReasonMixChart,
} from "./RecCharts";

const TICK = (value: number) => (Math.abs(value) >= 10000 ? compact(value) : String(Math.round(value)));

/* ── 1 近 14 天导入量（竖条） ── */
export function ImportTrendChart({ points, ...state }: { points: ImportTrendPoint[] } & ChartState) {
  const model = buildImportTrend(points);
  const tip = useChartTip();
  const bars = model.data.bars;
  return (
    <ChartFrame id="trend" title="近 14 天导入量" note={model.note} isEmpty={model.empty} {...state}>
      <ColumnChart
        labels={bars.map((bar) => bar.label)}
        series={[{ key: "books", label: "导入本数", color: "var(--accent)", values: bars.map((bar) => bar.value) }]}
        titles={bars.map((bar) => bar.title)}
        height={CHART_BODY_H}
        caption="导入本数 · 本"
        formatValue={TICK}
        tipBind={tip.bind}
        testId="chart:trend:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 2 导入累计曲线（折线 + 面积） ── */
export function ImportCumulativeChart({ points, ...state }: { points: ImportTrendPoint[] } & ChartState) {
  const model = buildImportCumulative(points);
  const tip = useChartTip();
  const rows = model.data.points;
  return (
    <ChartFrame id="cumulative" title="导入累计" note={model.note} isEmpty={model.empty} {...state}>
      <LineChart
        labels={rows.map((row) => row.label)}
        series={[{ key: "cumulative", label: "累计本数", color: "var(--accent)", values: rows.map((row) => row.value) }]}
        titles={rows.map((row) => row.title)}
        height={CHART_BODY_H}
        caption="累计本数 · 本"
        formatValue={TICK}
        tipBind={tip.bind}
        testId="chart:cumulative:figure"
        ariaLabel={model.note}
      />
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
      <BarChart
        rows={model.data.rows.map((row) => ({
          key: row.name,
          name: row.name,
          value: row.value,
          display: row.display,
          title: row.title,
        }))}
        height={CHART_BODY_H}
        caption="字数 · 字"
        nameWidth={88}
        peakIndex={0}
        tipBind={tip.bind}
        testId="chart:authors:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 4 章节数分布（竖条，真实刻度） ── */
export function ChapterBucketsChart({ buckets, ...state }: { buckets: Bucket[] } & ChartState) {
  const model = buildChapterBuckets(buckets);
  const tip = useChartTip();
  const bars = model.data.bars;
  const peakIndex = bars.findIndex((bar) => bar.label === model.data.peak);
  return (
    <ChartFrame id="buckets" title="章节数分布" note={model.note} isEmpty={model.empty} {...state}>
      <ColumnChart
        labels={bars.map((bar) => bar.label)}
        series={[{ key: "books", label: "本数", color: "var(--accent)", values: bars.map((bar) => bar.value) }]}
        titles={bars.map((bar) => bar.title)}
        height={CHART_BODY_H}
        caption="本数 · 本"
        showValues
        peakIndex={peakIndex >= 0 ? peakIndex : undefined}
        formatValue={TICK}
        tipBind={tip.bind}
        testId="chart:buckets:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 5 阅读进度分布（环形，图例可过滤） ── */
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

  /** 图例过滤：只改这张图的显隐，不改任何数字，也不发请求。 */
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
        <svg className="donut" width={RADIUS * 2 + 16} height={RADIUS * 2 + 16} viewBox="0 0 120 120" role="img" aria-label={note}>
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
        <div className="donut-legend">
          {model.data.segments.map((segment) => (
            <button
              className={`legend-row legend-btn${hidden.has(segment.key) ? " is-off" : ""}`}
              key={segment.key}
              type="button"
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
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 6 库体积构成（堆叠条，真实占比 + 铺满图体） ── */
export function StorageChart({ stats, ...state }: { stats: Stats | null } & ChartState) {
  const model = buildStorage(stats);
  const tip = useChartTip();
  const ratio = storageRatioNote(model.data);
  /** 结论 + 压缩率合成一行：同一张卡里两行说明会把卡片撑高，也与相邻卡不再等高。 */
  const note = ratio ? `${model.note}；${ratio}` : model.note;
  return (
    <ChartFrame id="storage" title="库体积构成" note={note} isEmpty={model.empty} {...state}>
      <StackBar
        parts={model.data.parts
          .filter((part) => part.bytes > 0)
          .map((part) => ({
            key: part.key,
            label: part.label,
            value: part.bytes,
            display: part.display,
            pctText: part.pctText,
            color: part.color,
            labelColor: part.labelColor,
            title: part.title,
          }))}
        height={CHART_BODY_H}
        legend={model.data.parts
          .filter((part) => part.bytes > 0)
          .map((part) => ({ key: part.key, label: `${part.label} ${part.pctText}`, color: part.color }))}
        tipBind={tip.bind}
        testId="chart:storage:figure"
        ariaLabel={note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 8 近 14 天阅读时长（折线 + 面积）：与热力图同一份 /api/stats/reading 的 days ── */
export function ReadingTrendChart({ days, ...state }: { days: DaySeconds[] } & ChartState) {
  const model = buildReadingTrend(days, 14);
  const tip = useChartTip();
  const points = model.data.points;
  return (
    <ChartFrame id="reading-trend" title="近 14 天阅读时长" note={model.note} isEmpty={model.empty} {...state}>
      <LineChart
        labels={points.map((point) => point.label)}
        series={[{ key: "minutes", label: "阅读时长", color: "var(--accent)", values: points.map((point) => point.value) }]}
        titles={points.map((point) => point.title)}
        height={CHART_BODY_H}
        caption="阅读时长 · 分钟"
        area
        formatValue={(value) => String(Math.round(value))}
        tipBind={tip.bind}
        testId="chart:reading-trend:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 7 章节长度分布（竖条，真实刻度） ── */
export function ChapterLengthChart({ buckets, ...state }: { buckets: Bucket[] } & ChartState) {
  const model = buildChapterLength(buckets);
  const tip = useChartTip();
  const cols = model.data.cols;
  const peakIndex = cols.findIndex((col) => col.label === model.data.peak);
  return (
    <ChartFrame id="chapterlen" title="章节长度分布" note={model.note} isEmpty={model.empty} {...state}>
      <ColumnChart
        labels={cols.map((col) => col.label)}
        series={[{ key: "chapters", label: "章数", color: "color-mix(in oklab, var(--accent), var(--surface) 45%)", values: cols.map((col) => col.value) }]}
        titles={cols.map((col) => col.title)}
        height={CHART_BODY_H}
        caption="章数 · 章"
        formatValue={TICK}
        peakIndex={peakIndex >= 0 ? peakIndex : undefined}
        tipBind={tip.bind}
        testId="chart:chapterlen:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}
