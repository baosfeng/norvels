import { useEffect } from "react";
import type { ReadingStats } from "../../api/types";
import { formatDurationShort } from "../../lib/format";
import { buildHeatmap, type HeatmapDay } from "../../lib/seriesGroup";
import { ChartTipLayer } from "./ChartTip";
import { useChartTip } from "./useChartTip";

/** 热力图 5 档底色：0 档用弱边框色，1–4 档由 --accent 与 --surface 混出，深浅主题自动跟随。 */
const LEVEL_BG = [
  "var(--border-soft)",
  "color-mix(in oklab, var(--accent) 22%, var(--surface))",
  "color-mix(in oklab, var(--accent) 42%, var(--surface))",
  "color-mix(in oklab, var(--accent) 66%, var(--surface))",
  "var(--accent)",
];

// 格子边长固定 11px：七行 × 十三周的日历必须等宽，格子随数据量变大就会破坏「一周一列」的读法
const CELL = 11;
const GAP = 3;
const WEEKDAY_LABELS = ["一", "三", "五"];
/** 点开后的浮层固定显示 5 秒（原行为）。hover / 聚焦立即显示，不再等 120ms。 */
const PIN_MS = 5000;

/**
 * 阅读时长卡（驾驶舱）：今日 / 本周 / 累计 / 连续天数 + 近 90 天日历热力图。
 *
 * 读数浮层与六张图共用 ChartTip：hover 与键盘聚焦都立即出读数，
 * 浮层贴在格子上方并带小三角指回格子；点一下固定 5 秒，换格或 Esc 收起。
 * 原先要 hover 停 120ms 才浮出——11px 的小格子快速划过时几乎看不到，这是
 * 「移上去没反应」的根因之一，因此改成即时。
 */
export function ReadingHeatmapCard({
  stats,
  loading,
  failed,
  onRetry,
}: {
  stats: ReadingStats | null;
  loading: boolean;
  failed: boolean;
  onRetry: () => void;
}) {
  const model = buildHeatmap(stats?.days ?? []);
  const tip = useChartTip(PIN_MS);

  /** Esc 收起当前读数（格子上按 Esc 与在抽屉里按 Esc 是同一套习惯）。 */
  useEffect(() => {
    if (!tip.tip) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") tip.hide();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [tip]);

  /** 移开/失焦时收起读数，但已经点开固定住的那次不收起。 */
  function dismiss() {
    if (!tip.tip?.pinned) tip.hide();
  }

  const note = loading
    ? "正在读取…"
    : failed
      ? "阅读时长暂时读不到。"
      : model.empty
        ? "近 90 天还没有阅读记录。"
        : `近 ${model.days.length} 天有 ${model.activeDays} 天在读，单日最长 ${formatDurationShort(model.maxSeconds)}。 指到格子上看当天读了多久。`;

  return (
    <article className="chart-card" data-chart="reading-time" data-testid="chart:reading-time">
      <div className="chart-head">
        <h3>阅读时长</h3>
        <p className="chart-note" data-testid="chart:reading-time:note">
          {note}
        </p>
      </div>

      {loading ? (
        <div className="chart-empty" data-testid="chart:reading-time:loading">
          正在读取…
        </div>
      ) : failed ? (
        <div className="chart-empty" data-testid="chart:reading-time:error">
          <span>数据暂不可用</span>
          <button className="load-more" type="button" onClick={onRetry}>
            重试
          </button>
        </div>
      ) : (
        <>
          <div className="dash-recent" data-testid="chart:reading-time:figures">
            <Figure label="今日" testId="reading-time-today" value={stats?.todaySeconds ?? 0} />
            <Figure label="本周" testId="reading-time-week" value={stats?.weekSeconds ?? 0} />
            <Figure label="累计" testId="reading-time-total" value={stats?.totalSeconds ?? 0} />
            <Figure label="连续" testId="reading-time-streak" value={stats?.streak ?? 0} suffix="天" />
          </div>

          {model.empty ? (
            <div className="chart-empty" data-testid="chart:reading-time:empty">
              读几分钟就会在这里留下第一格
            </div>
          ) : (
            <div className="dash-list" style={{ gap: GAP, overflowX: "auto" }} data-testid="chart:reading-time:grid">
              <div style={{ display: "flex", gap: GAP, alignItems: "flex-start" }}>
                <div style={{ display: "flex", flexDirection: "column", gap: GAP }} aria-hidden="true">
                  {Array.from({ length: 7 }, (_, row) => (
                    <span key={row} className="meta" style={{ height: CELL, fontSize: 9, lineHeight: `${CELL}px` }}>
                      {row % 2 === 0 ? WEEKDAY_LABELS[row / 2] ?? "" : ""}
                    </span>
                  ))}
                </div>
                {model.columns.map((column, index) => (
                  <div key={index} style={{ display: "flex", flexDirection: "column", gap: GAP }}>
                    {column.map((day, row) =>
                      day ? (
                        <HeatCell
                          key={day.date}
                          day={day}
                          active={tip.tip?.label === day.label}
                          onEnter={(cell) => tip.show(cell, day.label)}
                          onLeave={dismiss}
                          onFocus={(cell) => tip.show(cell, day.label)}
                          onBlur={dismiss}
                          onPin={(cell) => tip.show(cell, day.label, true)}
                          onEscape={tip.hide}
                        />
                      ) : (
                        <span key={`blank-${index}-${row}`} aria-hidden="true" style={{ width: CELL, height: CELL }} />
                      ),
                    )}
                  </div>
                ))}
              </div>
              <div className="meta" style={{ display: "flex", gap: 6, alignItems: "center" }} data-testid="chart:reading-time:legend">
                <span>少</span>
                {LEVEL_BG.map((bg, level) => (
                  <span key={level} aria-hidden="true" style={{ width: CELL, height: CELL, background: bg, borderRadius: 2 }} />
                ))}
                <span>多</span>
              </div>
            </div>
          )}
        </>
      )}

      <ChartTipLayer tip={tip.tip} testId="heat-tip" />
    </article>
  );
}

function Figure({ label, testId, value, suffix }: { label: string; testId: string; value: number; suffix?: string }) {
  return (
    <div>
      <span className="meta">{label}</span>
      <b className="num" data-testid={testId}>
        {suffix ? `${value} ${suffix}` : formatDurationShort(value)}
      </b>
    </div>
  );
}

/** 一格一天：可点、可聚焦、可单独读屏（每格独立 aria）。 */
function HeatCell({
  day,
  active,
  onEnter,
  onLeave,
  onFocus,
  onBlur,
  onPin,
  onEscape,
}: {
  day: HeatmapDay;
  active: boolean;
  onEnter: (cell: HTMLElement) => void;
  onLeave: () => void;
  onFocus: (cell: HTMLElement) => void;
  onBlur: () => void;
  onPin: (cell: HTMLElement) => void;
  onEscape: () => void;
}) {
  return (
    <button
      type="button"
      className={`heat-cell${active ? " is-active" : ""}`}
      data-testid={`heat-cell-${day.date}`}
      data-level={day.level}
      aria-label={day.label}
      onMouseEnter={(event) => onEnter(event.currentTarget)}
      onMouseLeave={onLeave}
      onFocus={(event) => onFocus(event.currentTarget)}
      onBlur={onBlur}
      onClick={(event) => onPin(event.currentTarget)}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          onEscape();
        }
      }}
      style={{ width: CELL, height: CELL, borderRadius: 2, background: LEVEL_BG[day.level] }}
    />
  );
}