import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { showToast } from "../../lib/toastStore";
import { useDashboard, useMetrics, useRecommendStats } from "../../api/hooks";
import type { RecommendStatsSummary } from "../../api/types";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount, formatDate } from "../../lib/format";
import { buildRecommendMetrics } from "../../lib/charts";
import { HealthCard } from "./HealthCard";
import { ObservePanel } from "./ObservePanel";
import { ReadingHeatmapCard } from "./ReadingHeatmap";
import {
  CategoryClicksChart,
  ChapterBucketsChart,
  ChapterLengthChart,
  EventTrendChart,
  FunnelChart,
  HotBooksChart,
  ImportCumulativeChart,
  ImportTrendChart,
  InterestBarsChart,
  ReadingDistChart,
  ReadingTrendChart,
  ReasonMixChart,
  StorageChart,
  TopAuthorsChart,
  type ChartState,
} from "./Charts";

const TREND_DAYS = 14;
/** 最近在读列表只显示 5 行；聚合端点一次给 50 条（三态估算要用全部）。 */
const RECENT_LIMIT = 5;
const PROGRESS_LIMIT = 50;

/** 埋点接口读不到 / 还没数据时的空汇总：图拿到全 0 就走各自的空态，不用一堆 ?? 兜底。 */
const EMPTY_RECOMMEND_SUMMARY: RecommendStatsSummary = {
  days: TREND_DAYS,
  impressions: 0,
  clicks: 0,
  reads: 0,
  readSeconds: 0,
  shelfAdds: 0,
};

/** 一条埋点事件都没有：曝光 / 点击 / 阅读 / 收藏全为 0（后端冷启动就是这个形状）。 */
function hasNoEvents(summary: RecommendStatsSummary, dailyCount: number): boolean {
  return (
    dailyCount === 0 ||
    summary.impressions + summary.clicks + summary.reads + summary.shelfAdds === 0
  );
}

/**
 * 驾驶舱（PC 专属）。图与结论都由 lib/charts.ts 的同一份转换结果生成，不会各说各话。
 *
 * 取数分三类，一次刷新共 3 个请求：
 *   1. `useDashboard` —— 数字类 7 块（stats / health / 导入趋势 / 阅读时长 / 最近在读 /
 *      Top 作者 / 导入任务）由后端聚合端点返回，替代原先的 8 个独立请求；
 *   2. `useRecommendStats` —— 依赖独立推荐库，缺库是 503，刻意留在聚合端点外面；
 *   3. `useMetrics` —— 进程内快照，必须反映「当下」，也不进 5 秒缓存。
 *
 * 各块的缺失互不牵连：聚合端点在 warnings 里说明哪块没取到，前端让那一张图走空态
 * （stats 除外——它是首屏核心，缺了整体 5xx，这里就走整页错误态）。
 */
export function DashboardPage() {
  /** 数字类 7 块：一次请求、一次刷新 */
  const dashboard = useDashboard();
  /** 推荐与埋点：曝光 / 点击 / 阅读 / 收藏的窗口聚合（rec 库；未启用时后端 503） */
  const rec = useRecommendStats(TREND_DAYS);
  /** 接口耗时：进程内快照（路由分位 + 慢请求），不自动轮询，只在刷新时重取 */
  const metrics = useMetrics();
  const [refreshing, setRefreshing] = useState(false);
  /** 刷新完成的最短可见时长定时器：卸载时必须清掉，否则 jsdom 已拆环境后回调会抛 window is not defined。 */
  const refreshTimer = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (refreshTimer.current !== null) window.clearTimeout(refreshTimer.current);
    },
    [],
  );

  /** 刷新概览：3 个数据源各重取一次（原先是 10 个），卡片保持旧值直到新值回来。 */
  async function refreshOverview() {
    if (refreshing) return;
    setRefreshing(true);
    const started = Date.now();
    for (const source of [dashboard, rec, metrics]) {
      source.reload();
    }
    const wait = Math.max(0, 240 - (Date.now() - started));
    if (refreshTimer.current !== null) window.clearTimeout(refreshTimer.current);
    refreshTimer.current = window.setTimeout(() => {
      refreshTimer.current = null;
      setRefreshing(false);
      showToast("概览已刷新");
    }, wait);
  }

  const payload = dashboard.data;
  /** 每块各自兜底：某块缺失时只有用到它的那张图走空态，整页照常出。 */
  const statsData = payload?.stats ?? null;
  const healthData = payload?.health ?? null;
  const trendPoints = payload?.importTrend ?? [];
  const readingDays = payload?.readingStats?.days ?? [];
  const authors = payload?.topAuthors?.items ?? [];
  const jobItems = payload?.importJobs ?? [];
  /** 最近在读最多 50 条：列表取前 5 行，阅读三态估算吃全部 */
  const progressAll = payload?.readingProgress ?? [];
  const recentItems = progressAll.slice(0, RECENT_LIMIT);

  const loading = dashboard.loading && !payload;
  const error = dashboard.error;
  const empty = !loading && !error && (statsData?.books ?? 0) === 0;

  /** 图卡四态：默认跟随聚合端点，推荐与埋点那张区块用自己的数据源。 */
  const chartState = (
    source: { loading: boolean; error: unknown; reload: () => void } = dashboard,
    stale = false,
  ): ChartState => ({
    loading: source.loading,
    failed: Boolean(source.error),
    onRetry: source.reload,
    stale,
  });

  // 后端把统计改成查预计算表后，刷新窗口内会带 stale 标记：此时照常出图，只提示「数据更新中」
  const statsStale = Boolean(statsData?.stale);
  const healthStale = Boolean(healthData?.stale);

  const recSummary = rec.data?.summary ?? EMPTY_RECOMMEND_SUMMARY;
  const recDaily = rec.data?.daily ?? [];
  // 「今日」按 UTC 天匹配：后端 daily 的日期就是 UTC 的 YYYY-MM-DD，与徽标口径一致
  const recommendMetrics = buildRecommendMetrics(recSummary, recDaily, new Date().toISOString().slice(0, 10))
    .metrics;
  const recEmpty = hasNoEvents(recSummary, recDaily.length);

  return (
    <section data-testid="view:dashboard">
      <div className="page-head">
        <div>
          <p className="eyebrow">库总览</p>
          <h1>驾驶舱</h1>
          <p className="sub">书库状态、阅读进度与最近动态</p>
        </div>
        <button
          className="btn btn-sm"
          type="button"
          disabled={refreshing}
          onClick={() => void refreshOverview()}
        >
          {refreshing ? "正在刷新…" : "刷新概览"}
        </button>
      </div>

      {loading ? (
        <LoadingPanel title="正在读取书库概览" description="正在汇总书籍、章节与最近动态。" />
      ) : error ? (
        <ErrorPanel error={error} title="概览没能加载" onRetry={dashboard.reload} />
      ) : empty ? (
        <EmptyPanel
          title="书库还没有内容"
          description="把存放 txt 的目录导入之后，这里会显示书库规模、阅读进度和最近动态。"
          actions={
            <Link className="btn btn-primary" to="/import">
              去导入
            </Link>
          }
        />
      ) : (
        <div className="dash-layout">
          <div className="dash-main">
            <div className="dash-stats">
              <StatCard label="书籍总数" value={formatCount(statsData?.books ?? 0)} hint="本" />
              <StatCard label="章节总数" value={formatCount(statsData?.chapters ?? 0)} hint="章" />
              <StatCard label="作者" value={formatCount(statsData?.authors ?? 0)} hint="位" />
              <StatCard label="总字数" value={formatCount(statsData?.words ?? 0)} hint="字" />
            </div>

            <section className="dash-block">
              <div className="dash-block-head">
                <h2>最近在读</h2>
                <Link className="meta" to="/shelf" state={{ resetFilters: true }}>
                  全部书架 →
                </Link>
              </div>
              {dashboard.loading ? (
                <LoadingPanel title="正在读取阅读进度" description="稍等一下。" />
              ) : dashboard.error ? (
                <ErrorPanel error={dashboard.error} title="阅读进度没能加载" onRetry={dashboard.reload} />
              ) : recentItems.length === 0 ? (
                <EmptyPanel title="还没有阅读记录" description="从书架挑一本开始读，进度会自动记在这里。" />
              ) : (
                <div className="dash-list">
                  {recentItems.map((item) => (
                    <Link
                      className="dash-row"
                      key={item.bookId}
                      to={`/read/${item.bookId}/${item.chapterId}`}
                    >
                      <Cover title={item.bookTitle} showLabel={false} />
                      <div className="dash-row-main">
                        <b>{item.bookTitle}</b>
                        <span className="meta">{item.chapterTitle}</span>
                        <span className="dash-bar">
                          <i style={{ width: `${Math.round(item.percent * 100)}%` }} />
                        </span>
                      </div>
                      <div className="dash-row-side">
                        <b className="num">{Math.round(item.percent * 100)}%</b>
                        <span className="meta">{formatDate(item.updatedAt)}</span>
                      </div>
                    </Link>
                  ))}
                </div>
              )}
            </section>

            <section className="dash-block">
              <div className="dash-block-head">
                <h2>导入概况</h2>
                <Link className="meta" to="/import">
                  前往导入管理 →
                </Link>
              </div>
              {dashboard.loading ? (
                <LoadingPanel title="正在读取导入记录" description="稍等一下。" />
              ) : dashboard.error ? (
                <ErrorPanel error={dashboard.error} title="导入记录没能加载" onRetry={dashboard.reload} />
              ) : jobItems.length === 0 ? (
                <EmptyPanel title="还没有导入任务" description="去导入页选一个目录，开始第一次导入。" />
              ) : (
                <ImportSummary jobs={jobItems} />
              )}
            </section>


            {/* 推荐与埋点：数据来自发现页的真实浏览（曝光 / 点击 / 停留 / 阅读）。
                事件只有真的逛过才会产生，所以整块自己带空态，而不是画四张空图占两屏。 */}
            <section className="dash-block" data-testid="dashboard:recommend">
              <div className="dash-block-head">
                <h2>推荐与埋点</h2>
                <Link className="meta" to="/discover" data-testid="dashboard:discover-link">
                  去发现页 →
                </Link>
              </div>
              {rec.loading && !rec.data ? (
                <LoadingPanel
                  title="正在读取埋点数据"
                  description="正在汇总最近的曝光、点击与阅读。"
                  testId="recommend-loading"
                />
              ) : rec.error ? (
                <ErrorPanel error={rec.error} title="埋点数据没能加载" onRetry={rec.reload} testId="recommend-error" />
              ) : recEmpty ? (
                <EmptyPanel
                  title="还没有埋点数据"
                  description="去发现页逛一圈就会开始记录：曝光、点击、停留与阅读都会计入，这里随后会长出兴趣画像与热门书。"
                  testId="recommend-empty"
                  actions={
                    <Link className="btn btn-primary" to="/discover">
                      去发现页
                    </Link>
                  }
                />
              ) : (
                <>
                  <div className="rec-metrics" data-testid="recommend-metrics">
                    {recommendMetrics.map((metric) => (
                      <div className="stat-card" key={metric.key} data-testid={`metric-${metric.key}`}>
                        <span className="stat-label">{metric.label}</span>
                        <b className="stat-num num">{metric.value}</b>
                        <span className="stat-hint">{metric.hint}</span>
                      </div>
                    ))}
                  </div>
                  <div className="chart-grid rec-charts" data-testid="recommend-charts">
                    <EventTrendChart daily={recDaily} {...chartState(rec)} />
                    <InterestBarsChart interests={rec.data?.interests ?? []} {...chartState(rec)} />
                    <HotBooksChart books={rec.data?.hotBooks ?? []} {...chartState(rec)} />
                    <FunnelChart summary={recSummary} {...chartState(rec)} />
                    {/* 两张分布图：老后端没有这两块数据时各自走空态，不影响上面四张 */}
                    <ReasonMixChart rows={rec.data?.reasonMix ?? []} {...chartState(rec)} />
                    <CategoryClicksChart rows={rec.data?.categoryClicks ?? []} {...chartState(rec)} />
                  </div>
                </>
              )}
            </section>

            {/* 接口耗时：唯一一块「运维视角」的数据，慢在哪要能直接在驾驶舱看到 */}
            <section className="dash-block" data-testid="dashboard:metrics">
              <div className="dash-block-head">
                <h2>接口耗时</h2>
                <button
                  className="btn btn-sm"
                  type="button"
                  disabled={metrics.loading}
                  onClick={metrics.reload}
                  data-testid="metrics-refresh"
                >
                  {metrics.loading ? "读取中…" : "刷新耗时"}
                </button>
              </div>
              <ObservePanel
                snapshot={metrics.data}
                loading={metrics.loading}
                error={metrics.error}
                onRetry={metrics.reload}
              />
            </section>

            {/* 九张图两列排：阅读时长占整行（四项数值 2×2 + 90 天日历要横向空间），
                其余两两配对——同为 14 天时间轴的放一行、同为分布的放一行，读起来成组。 */}
            <div className="chart-grid" data-testid="dashboard:charts">
              <ReadingHeatmapCard
                stats={payload?.readingStats ?? null}
                loading={dashboard.loading}
                failed={Boolean(dashboard.error)}
                onRetry={dashboard.reload}
              />
              <ReadingTrendChart days={readingDays} {...chartState()} />
              <ImportTrendChart points={trendPoints} {...chartState()} />
              <ImportCumulativeChart points={trendPoints} {...chartState()} />
              <TopAuthorsChart
                authors={authors.map((a) => ({ name: a.name, wordCount: a.wordCount }))}
                totalWords={statsData?.words ?? 0}
                {...chartState()}
              />
              <ChapterBucketsChart buckets={healthData?.chapterBuckets ?? []} {...chartState(dashboard, healthStale)} />
              <ReadingDistChart
                progress={progressAll}
                totalBooks={statsData?.books ?? 0}
                limitReached={progressAll.length >= PROGRESS_LIMIT}
                // 后端已给聚合值时以它为准（不依赖 50 条上限），否则回退到估算
                buckets={healthData?.readingBuckets ?? null}
                {...chartState()}
              />
              <StorageChart stats={statsData} {...chartState(dashboard, statsStale)} />
              <ChapterLengthChart
                buckets={healthData?.chapterLengthBuckets ?? []}
                {...chartState(dashboard, healthStale)}
              />
            </div>
          </div>

          <aside className="dash-side">
            <HealthCard
              report={healthData}
              loading={dashboard.loading}
              error={dashboard.error}
              onRetry={dashboard.reload}
              lastImportAt={statsData?.lastImportAt ?? null}
            />
          </aside>
        </div>
      )}
    </section>
  );
}

function StatCard({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="stat-card">
      <span className="stat-label">{label}</span>
      <b className="stat-num num">{value}</b>
      <span className="stat-hint">{hint}</span>
    </div>
  );
}

function ImportSummary({
  jobs,
}: {
  jobs: {
    id: number;
    status: string;
    total: number;
    imported: number;
    skipped: number;
    failed: number;
    sourceDir: string;
    startedAt: string | null;
  }[];
}) {
  const latest = jobs[0];
  return (
    <div className="dash-recent">
      <div>
        <span className="meta">上次导入</span>
        <b>{formatDate(latest?.startedAt ?? null)}</b>
      </div>
      <div>
        <span className="meta">导入</span>
        <b className="num">{latest?.imported ?? 0} 本</b>
      </div>
      <div>
        <span className="meta">跳过</span>
        <b className="num">{latest?.skipped ?? 0} 本</b>
      </div>
      <div>
        <span className="meta">未成功</span>
        <b className="num">{latest?.failed ?? 0} 本</b>
      </div>
    </div>
  );
}