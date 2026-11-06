import { useMemo } from "react";
import type { ApiError } from "../../api/client";
import type { MetricsSnapshot, RouteTiming, SlowRequest } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { compact } from "../../lib/charts";
import { formatDuration } from "../../lib/format";

/** 表格只放「最慢的前 10 条」：这是判断瓶颈的入口，不是全量清单（全量走 /api/metrics）。 */
const TOP_ROUTES = 10;
/** 慢请求列表默认展示最近 20 条（后端环形缓冲容量 200）。 */
const SLOW_ROWS = 20;

/** 毫秒读数：小于 10ms 保留一位小数，大于等于 1s 换成秒，避免一列里数字长度跳来跳去。 */
function ms(v: number): string {
  if (v >= 1000) return `${(v / 1000).toFixed(2)}s`;
  return `${v < 10 ? v.toFixed(1) : Math.round(v)}ms`;
}

/** 按时钟展示慢请求发生时间（at 是 RFC3339，跨时区按浏览器本地时区显示）。 */
function clock(at: string): string {
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return at;
  return d.toLocaleTimeString("zh-CN", { hour12: false });
}

/** 按 p95 降序取前 N 条：次数多但快的接口不是瓶颈，先看尾部延迟。 */
function topByP95(routes: RouteTiming[]): RouteTiming[] {
  return [...routes]
    .filter((route) => route.count > 0)
    .sort((a, b) => b.p95Ms - a.p95Ms || b.count - a.count)
    .slice(0, TOP_ROUTES);
}

/** 运行时一行摘要：连接池只列池名与占用，细节留在接口里。 */
function runtimeLine(snapshot: MetricsSnapshot): string {
  const rt = snapshot.runtime;
  const pools = rt.pools.map((pool) => `${pool.name} ${pool.inUse}/${pool.open}`).join(" · ");
  const base = `${rt.goVersion} · goroutines ${rt.goroutines} · 堆 ${rt.heapMB.toFixed(1)}MB · GC ${rt.numGC} 次 · 已运行 ${formatDuration(snapshot.uptimeSec)}`;
  return pools ? `${base} · 连接池 ${pools}` : base;
}

/**
 * 驾驶舱「接口耗时」区块：路由级分位表 + 最近慢请求。
 *
 * 数据全部来自进程内快照（GET /api/metrics），重启清零——这是「刚刚发生了什么」的口径，
 * 不是历史趋势；要看历史趋势请接外部时序库（本块刻意不画趋势图，避免两套口径）。
 */
export function ObservePanel({
  snapshot,
  loading,
  error,
  onRetry,
}: {
  snapshot: MetricsSnapshot | null;
  loading: boolean;
  error: ApiError | null;
  onRetry: () => void;
}) {
  const rows = useMemo(() => topByP95(snapshot?.http ?? []), [snapshot]);
  const slow = useMemo(() => (snapshot?.slowRequests ?? []).slice(0, SLOW_ROWS), [snapshot]);
  const peak = rows.reduce((max, row) => Math.max(max, row.p95Ms), 0);

  if (loading && !snapshot) {
    return <LoadingPanel title="正在读取接口耗时" description="读取进程内的路由分位与慢请求记录。" testId="metrics-loading" />;
  }
  if (error) {
    return <ErrorPanel error={error} title="接口耗时没能加载" onRetry={onRetry} testId="metrics-error" />;
  }
  if (rows.length === 0) {
    return (
      <EmptyPanel
        title="还没有接口调用记录"
        description="进程刚启动，或者服务端用 NORVELS_METRICS=off 关掉了采集；调用几个接口后再刷新。"
        testId="metrics-empty"
      />
    );
  }

  return (
    <div data-testid="dashboard:metrics-body">
      <p className="meta" data-testid="metrics-runtime">
        {snapshot ? runtimeLine(snapshot) : ""}
      </p>

      <div className="table-wrap">
        <table className="table" data-testid="metrics-routes">
          <caption className="meta">按 p95 降序，前 {TOP_ROUTES} 条路由（模板聚合，重启清零）</caption>
          <thead>
            <tr>
              <th>路由</th>
              <th className="num">次数</th>
              <th className="num">平均</th>
              <th className="num">p50</th>
              <th className="num">p95</th>
              <th className="num">p99</th>
              <th className="num">最大</th>
              <th className="num">错误</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.route} data-testid={`metrics-route-${row.route}`}>
                <td>
                  <code>{row.route}</code>
                </td>
                <td className="num">{compact(row.count)}</td>
                <td className="num">{ms(row.avgMs)}</td>
                <td className="num">{ms(row.p50Ms)}</td>
                <td className="num">
                  {/* 条形与读数同一行：p95 既要能横向比较，也要能直接读出数字 */}
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 8, justifyContent: "flex-end" }}>
                    <span className="hbar-track" style={{ flex: "0 0 72px", width: 72 }}>
                      <span
                        className="hbar-fill"
                        style={{ width: `${peak > 0 ? Math.round((row.p95Ms / peak) * 100) : 0}%` }}
                      />
                    </span>
                    <span>{ms(row.p95Ms)}</span>
                  </span>
                </td>
                <td className="num">{ms(row.p99Ms)}</td>
                <td className="num">{ms(row.maxMs)}</td>
                <td className="num">
                  {row.errors + row.clientErrors > 0 ? `${row.errors + row.clientErrors}` : "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="table-wrap" style={{ marginTop: "var(--space-4)" }}>
        <table className="table" data-testid="metrics-slow">
          <caption className="meta">最近慢请求（最新在前，最多 {SLOW_ROWS} 条）</caption>
          <thead>
            <tr>
              <th>时间</th>
              <th>请求</th>
              <th className="num">状态</th>
              <th className="num">耗时</th>
              <th className="num">SQL</th>
            </tr>
          </thead>
          <tbody>
            {slow.length === 0 ? (
              <tr>
                <td colSpan={5} className="meta" data-testid="metrics-slow-empty">
                  阈值内没有慢请求（默认 500ms，可用 NORVELS_SLOW_MS 调整）。
                </td>
              </tr>
            ) : (
              slow.map((item: SlowRequest, idx) => (
                <tr key={`${item.at}-${idx}`} data-testid={`metrics-slow-${idx}`}>
                  <td className="num">{clock(item.at)}</td>
                  <td>
                    <code title={item.path}>{item.method} {item.path}</code>
                  </td>
                  <td className="num">{item.status}</td>
                  <td className="num">{ms(item.durMs)}</td>
                  <td className="num">
                    {item.sqlCount > 0 ? `${item.sqlCount} 次 / ${ms(item.sqlMs)}` : "—"}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
