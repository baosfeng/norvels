import { clampNumber, clipText, domainTopOf } from "../../lib/chartScale";
import {
  buildCategoryClicks,
  buildEventTrend,
  buildFunnel,
  buildHotBooks,
  buildInterestBars,
  buildReasonMix,
  compact,
  type InterestGroup,
} from "../../lib/charts";
import type {
  RecommendCategoryClicks,
  RecommendHotBook,
  RecommendInterest,
  RecommendReasonMix,
  RecommendStatsDay,
  RecommendStatsSummary,
} from "../../api/types";
import { BarChart } from "./charts/BarChart";
import { CHART_BODY_H, ChartFrame, type ChartState } from "./charts/ChartFrame";
import { FacetColumns } from "./charts/FacetColumns";
import { useChartWidth } from "./charts/useChartWidth";
import { ChartTipLayer } from "./ChartTip";
import { useChartTip, type ChartTipBindings } from "./useChartTip";

/**
 * 「推荐与埋点」六张图：纯 SVG，几何走 lib/chartScale.ts。
 *
 * 与库概览七张图同一套 ChartFrame / 图体高度 / 读数浮层，不再各写一套。
 * 量纲不同的系列一律分面（事件趋势上下两面），同量纲的横向比（兴趣画像组内归一、漏斗按曝光归一）。
 */

const IMPRESSION = "color-mix(in oklab, var(--accent), var(--surface) 58%)";
const CLICK = "var(--accent)";
const READ = "var(--success-text)";
const SHELF = "color-mix(in oklab, var(--success-text), var(--surface) 35%)";

/* ── 1 近 14 天埋点事件（上下两个 mini 面） ── */
export function EventTrendChart({ daily, ...state }: { daily: RecommendStatsDay[] } & ChartState) {
  const model = buildEventTrend(daily);
  const tip = useChartTip();
  const days = model.data.days;
  return (
    <ChartFrame id="events" title="近 14 天埋点事件" note={model.note} isEmpty={model.empty} {...state}>
      <FacetColumns
        labels={days.map((day) => day.label)}
        facets={[
          {
            key: "counts",
            unit: "次",
            series: [
              { key: "impressions", label: "曝光", color: IMPRESSION, values: days.map((day) => day.impressions) },
              { key: "clicks", label: "点击", color: CLICK, values: days.map((day) => day.clicks) },
            ],
            formatValue: (value) => compact(value),
          },
          {
            key: "minutes",
            unit: "分钟",
            series: [{ key: "minutes", label: "阅读", color: READ, values: days.map((day) => day.minutes) }],
            formatValue: (value) => compact(value),
          },
        ]}
        titles={days.map((day) => day.title)}
        height={CHART_BODY_H}
        tipBind={tip.bind}
        testId="chart:events:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 2 兴趣画像（五组横条，组内归一） ── */

/**
 * 一组横条：组标题 + 组内 top5，行均分图体高度。
 *
 * 每组一个 SVG：五组并排时列宽由 CSS 网格决定，组内归一（各组权重不在同一尺度），
 * 组标题说清维度，条长只在组内可比。
 */
function GroupBars({ group, tipBind }: { group: InterestGroup; tipBind: (label: string) => ChartTipBindings }) {
  const [ref, width] = useChartWidth(160);
  const rows = group.rows;
  const labelH = 13;
  /** 行距封顶 26px：某一维只有 1–2 行时不再把条拉散到 132px 高，组内顶部对齐 */
  const rowStep = Math.min(26, (CHART_BODY_H - labelH) / Math.max(1, rows.length));
  const nameW = clampNumber(Math.round(width * 0.36), 26, 56);
  const valueW = 30;
  const barX = nameW + 4;
  const barAreaW = Math.max(10, width - barX - valueW - 2);
  const domainTop = domainTopOf(rows.reduce((acc, row) => Math.max(acc, row.value), 0));
  const barH = clampNumber(rowStep * 0.5, 4, 9);
  return (
    <div className="chart-fit" ref={ref} data-dim={group.dim} data-testid={`ibars-${group.dim}`}>
      <svg className="c-svg" width={width} height={CHART_BODY_H} role="img" aria-label={group.label}>
        <text className="c-caption" x={0} y={9}>
          {group.label}
        </text>
        {rows.map((row, index) => {
          const centerY = labelH + index * rowStep + rowStep / 2;
          const end = (row.value / domainTop) * barAreaW;
          return (
            <g key={`${group.dim}-${row.name}`}>
              <text className="c-barname" x={0} y={centerY + 3} fontSize={9.5}>
                {clipText(row.name, nameW, 9.5)}
              </text>
              <rect className="c-track" x={barX} y={centerY - barH / 2} width={barAreaW} height={barH} rx={2} fill="var(--surface-warm)" />
              <rect
                className="c-bar"
                x={barX}
                y={centerY - barH / 2}
                width={Math.max(2, end)}
                height={barH}
                rx={2}
                fill="var(--accent)"
              />
              <text className="c-value" x={width - 2} y={centerY + 3} fontSize={9} textAnchor="end">
                {row.display}
              </text>
              <rect
                className="c-hit"
                x={0}
                y={labelH + index * rowStep}
                width={width}
                height={Math.max(1, rowStep)}
                {...tipBind(row.title)}
              />
            </g>
          );
        })}
      </svg>
    </div>
  );
}

export function InterestBarsChart({ interests, ...state }: { interests: RecommendInterest[] } & ChartState) {
  const model = buildInterestBars(interests);
  const tip = useChartTip();
  return (
    <ChartFrame id="interest" title="兴趣画像" note={model.note} isEmpty={model.empty} {...state}>
      <div className="ibars">
        {model.data.groups.map((group) => (
          <GroupBars key={group.dim} group={group} tipBind={tip.bind} />
        ))}
      </div>
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 3 热门书 Top 10（横条，读过 2 分 / 点击 1 分） ── */
export function HotBooksChart({ books, ...state }: { books: RecommendHotBook[] } & ChartState) {
  const model = buildHotBooks(books);
  const tip = useChartTip();
  return (
    <ChartFrame id="hot" title="热门书 Top 10" note={model.note} isEmpty={model.empty} {...state}>
      <BarChart
        rows={model.data.rows.map((row, index) => ({
          key: `hot-${index}`,
          name: row.name,
          value: row.value,
          display: row.display,
          title: row.title,
        }))}
        height={CHART_BODY_H}
        caption="读过 2 分 / 点击 1 分"
        nameWidth={96}
        peakIndex={0}
        tipBind={tip.bind}
        testId="chart:hot:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 4 曝光 → 点击 → 阅读 → 收藏（漏斗） ── */
export function FunnelChart({ summary, ...state }: { summary: RecommendStatsSummary } & ChartState) {
  const model = buildFunnel(summary);
  const tip = useChartTip();
  const colors: Record<string, string> = { impression: IMPRESSION, click: CLICK, read: READ, shelf: SHELF };
  return (
    <ChartFrame
      id="funnel"
      title="曝光到收藏"
      note={`${model.note}；条长按曝光归一`}
      isEmpty={model.empty}
      {...state}
    >
      <BarChart
        rows={model.data.steps.map((step) => ({
          key: step.key,
          name: step.label,
          value: step.value,
          display: compact(step.value),
          trailing: step.stepRate,
          color: colors[step.key],
          title: step.title,
        }))}
        height={CHART_BODY_H}
        caption="条长按曝光归一 · 次"
        nameWidth={34}
        tipBind={tip.bind}
        testId="chart:funnel:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 5 点击理由分布（横条） ── */
export function ReasonMixChart({ rows, ...state }: { rows: RecommendReasonMix[] } & ChartState) {
  const model = buildReasonMix(rows);
  const tip = useChartTip();
  return (
    <ChartFrame
      id="reasonmix"
      title="点击理由分布"
      note={`${model.note}；条长按点击量`}
      isEmpty={model.empty}
      {...state}
    >
      <BarChart
        rows={model.data.rows.map((row, index) => ({
          key: `reason-${index}`,
          name: row.name,
          value: row.value,
          display: row.display,
          title: row.title,
        }))}
        height={CHART_BODY_H}
        caption="点击 · 次"
        nameWidth={72}
        peakIndex={0}
        tipBind={tip.bind}
        testId="chart:reasonmix:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}

/* ── 6 分类点击分布（横条） ── */
export function CategoryClicksChart({ rows, ...state }: { rows: RecommendCategoryClicks[] } & ChartState) {
  const model = buildCategoryClicks(rows);
  const tip = useChartTip();
  return (
    <ChartFrame
      id="categoryclicks"
      title="分类点击分布"
      note={model.note}
      isEmpty={model.empty}
      {...state}
    >
      <BarChart
        rows={model.data.rows.map((row, index) => ({
          key: `category-${index}`,
          name: row.name,
          value: row.value,
          display: row.display,
          title: row.title,
        }))}
        height={CHART_BODY_H}
        caption="点击 · 次"
        nameWidth={82}
        peakIndex={0}
        tipBind={tip.bind}
        testId="chart:categoryclicks:figure"
        ariaLabel={model.note}
      />
      <ChartTipLayer tip={tip.tip} />
    </ChartFrame>
  );
}
