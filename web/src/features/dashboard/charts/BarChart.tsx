import { clampNumber, clipText, domainTopOf, estimateTextWidth } from "../../../lib/chartScale";
import type { ChartTipBindings } from "../useChartTip";
import { CaptionRow, type LegendItem } from "./svgLayout";
import { useChartWidth } from "./useChartWidth";

/**
 * 横条图（纯 SVG）：作者榜 / 热门书 / 点击理由 / 分类点击 / 漏斗共用。
 *
 * 三条口径：
 *   - 值写在条尾：条长随数值变化，值贴着条的末端，不再固定在最右一列（数字与条对不上时最难读）；
 *   - 名字列定宽 + 按像素宽度截断补「…」：作者名 / 书名 / 理由长到几十字也不挤掉条；
 *   - 行均分图体高度：10 行铺满 132px（每行约 13px、条高 8px），不再沉在卡片下半截。
 */

export interface BarRow {
  key: string;
  /** 左侧名字（作者 / 书名 / 分类 / 漏斗级名） */
  name: string;
  value: number;
  /** 条尾的数字文案（已格式化） */
  display: string;
  /** 悬停读数 */
  title: string;
  /** 最右一列（漏斗的级间转化率） */
  trailing?: string;
  color?: string;
}

export interface BarChartProps {
  rows: BarRow[];
  height?: number;
  caption?: string;
  legend?: LegendItem[];
  /** 名字列宽；省略时按图宽给（22%，钳 48–110） */
  nameWidth?: number;
  /** 默认条色（行上没给 color 时用） */
  color?: string;
  /** 高亮某一行的条色（榜单第一名） */
  peakIndex?: number;
  peakColor?: string;
  tipBind: (label: string) => ChartTipBindings;
  testId?: string;
  ariaLabel: string;
}

const MAX_NAME_FONT = 11;
const MAX_VALUE_FONT = 10;
const DEFAULT_COLOR = "color-mix(in oklab, var(--accent), var(--surface) 45%)";
const TRACK = "var(--surface-warm)";

export function BarChart({
  rows,
  height = 132,
  caption,
  legend,
  nameWidth,
  color = DEFAULT_COLOR,
  peakIndex,
  peakColor = "var(--accent)",
  tipBind,
  testId,
  ariaLabel,
}: BarChartProps) {
  const [ref, width] = useChartWidth();
  const names = nameWidth ?? clampNumber(Math.round(width * 0.22), 48, 110);
  const top = caption || legend?.length ? 13 : 2;
  // 行距按行数铺满可用高度：行数少时把行距拉开、条加高，最后一行贴到图体底部
  // （原先行距封顶 28px + 顶部对齐，3 行的卡下方会空掉 55%）；行数多时行距自然收紧。
  const available = Math.max(0, height - top - 2);
  const count = rows.length;
  const roughStep = count > 0 ? available / count : 0;
  const barH = clampNumber(roughStep * 0.55, 9, 20);
  /**
   * ≥3 行：首行贴顶、末行贴底，把可用高度铺满（行距拉开，不在下方堆一片空白）。
   * ≤2 行：整组垂直线居中（上下留白均分）——单行时「标题在顶、条贴底」比居中更难看，
   * 而且那点空白是数据量决定的，不该靠布局硬撑。
   */
  const pinned = count >= 3;
  /** 贴顶贴底时 n−1 个间隔均分剩余高度；居中时用固定的舒适行距 */
  const step = pinned && count > 1 ? (available - barH) / (count - 1) : 0;
  const centeredGap = 14;
  const blockH = count * barH + Math.max(0, count - 1) * centeredGap;
  const blockTop = top + Math.max(0, (available - blockH) / 2);
  /** 字号跟着行距走：10 行时行距 11.7px，10.5px 的字会挤成一团（实测作者榜读不出行界）。 */
  const nameFont = clampNumber(Math.min(roughStep, 28) * 0.75, 8, MAX_NAME_FONT);
  const valueFont = clampNumber(Math.min(roughStep, 28) * 0.72, 8, MAX_VALUE_FONT);
  /** 第 index 行的中心：贴边时自下而上（末行贴底），居中时整块垂直居中 */
  const centerYOf = (index: number) =>
    pinned
      ? height - 2 - barH / 2 - (count - 1 - index) * step
      : blockTop + barH / 2 + index * (barH + centeredGap);
  const maxValue = rows.reduce((acc, row) => (Number.isFinite(row.value) && row.value > acc ? row.value : acc), 0);
  const domainTop = domainTopOf(maxValue);
  const displayReserve =
    rows.reduce((acc, row) => Math.max(acc, estimateTextWidth(row.display, valueFont)), 0) + 6;
  const trailingReserve =
    rows.reduce((acc, row) => Math.max(acc, estimateTextWidth(row.trailing ?? "", valueFont)), 0) +
    (rows.some((row) => row.trailing) ? 8 : 0);
  const plotLeft = names + 6;
  const plotRight = Math.max(plotLeft + 24, width - 2 - trailingReserve);
  const barAreaW = Math.max(8, plotRight - plotLeft - displayReserve);

  /** 一行的可点区域高度：贴边时是行距，居中时是条高 + 半间隙 */
  const bandH = pinned ? Math.max(barH, step) : barH + centeredGap;
  /** 条尾的值：放不下时收进条内右对齐，任何情况下都不出图。 */
  const valueX = (end: number) => plotLeft + end + 4;
  const valueInside = (end: number) => plotLeft + end + 4 + displayReserve > plotRight;

  return (
    <div className="chart-fit" ref={ref} data-testid={testId}>
      <svg className="c-svg" width={width} height={height} role="img" aria-label={ariaLabel}>
        <CaptionRow caption={caption} legend={legend} />
        {rows.map((row, index) => {
          const centerY = centerYOf(index);
          const value = Number.isFinite(row.value) && row.value > 0 ? row.value : 0;
          const end = count > 0 ? (value / domainTop) * barAreaW : 0;
          const isPeak = peakIndex === index;
          const fill = isPeak ? peakColor : (row.color ?? color);
          const inside = valueInside(end) && end > displayReserve;
          return (
            <g key={row.key}>
              <text className="c-barname" x={0} y={centerY + nameFont * 0.35} fontSize={nameFont}>
                {clipText(row.name, names, nameFont)}
              </text>
              <rect
                className="c-track"
                x={plotLeft}
                y={centerY - barH / 2}
                width={barAreaW}
                height={barH}
                rx={Math.min(4, barH / 2)}
                fill={TRACK}
              />
              {value > 0 ? (
                <rect
                  className="c-bar"
                  x={plotLeft}
                  y={centerY - barH / 2}
                  width={Math.max(2, end)}
                  height={barH}
                  rx={Math.min(4, barH / 2)}
                  fill={fill}
                />
              ) : (
                <rect
                  className="c-bar is-zero"
                  data-zero-baseline="1"
                  x={plotLeft}
                  y={centerY - barH / 2}
                  width={1}
                  height={barH}
                  fill={fill}
                />
              )}
              <text
                className={`c-value${inside ? " is-inside" : ""}`}
                x={inside ? plotLeft + end - 4 : valueX(end)}
                y={centerY + valueFont * 0.35}
                fontSize={valueFont}
                textAnchor={inside ? "end" : "start"}
              >
                {row.display}
              </text>
              {row.trailing ? (
                <text
                  className="c-tick"
                  x={width - 2}
                  y={centerY + valueFont * 0.35}
                  fontSize={valueFont}
                  textAnchor="end"
                >
                  {row.trailing}
                </text>
              ) : null}
              <rect
                className="c-hit"
                x={0}
                y={Math.max(0, centerY - bandH / 2)}
                width={width}
                height={Math.max(1, Math.min(height - Math.max(0, centerY - bandH / 2), bandH))}
                {...tipBind(row.title)}
              />
            </g>
          );
        })}
      </svg>
    </div>
  );
}
