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
  /** 行距封顶 28px：只有 3 行时不再把三条撑满 132px（那样行与行之间空得像三张独立的卡），
   *  但行数多时仍然均分图体，10 行铺满。 */
  const rowStep = rows.length > 0 ? Math.min(28, (height - top - 2) / rows.length) : 0;
  const barH = clampNumber(rowStep * 0.52, 5, 14);
  /** 字号跟着行距走：10 行时行距 11.7px，10.5px 的字会挤成一团（实测作者榜读不出行界）。 */
  const nameFont = clampNumber(rowStep * 0.75, 8, MAX_NAME_FONT);
  const valueFont = clampNumber(rowStep * 0.72, 8, MAX_VALUE_FONT);
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

  /** 条尾的值：放不下时收进条内右对齐，任何情况下都不出图。 */
  const valueX = (end: number) => plotLeft + end + 4;
  const valueInside = (end: number) => plotLeft + end + 4 + displayReserve > plotRight;

  return (
    <div className="chart-fit" ref={ref} data-testid={testId}>
      <svg className="c-svg" width={width} height={height} role="img" aria-label={ariaLabel}>
        <CaptionRow caption={caption} legend={legend} />
        {rows.map((row, index) => {
          const centerY = top + index * rowStep + rowStep / 2;
          const value = Number.isFinite(row.value) && row.value > 0 ? row.value : 0;
          const end = rowStep > 0 ? (value / domainTop) * barAreaW : 0;
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
                y={top + index * rowStep}
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
