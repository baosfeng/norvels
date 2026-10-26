import { useState } from "react";
import { Link } from "react-router-dom";
import { showToast } from "../../lib/toastStore";
import {
  useHealthReport,
  useImportJobs,
  useImportTrend,
  useReadingProgress,
  useReadingProgressAll,
  useReadingStats,
  useStats,
  useTopAuthors,
} from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount, formatDate } from "../../lib/format";
import { ReadingHeatmapCard } from "./ReadingHeatmap";
import {
  ChapterBucketsChart,
  ChapterLengthChart,
  ImportTrendChart,
  ReadingDistChart,
  StorageChart,
  TopAuthorsChart,
  type ChartState,
} from "./Charts";

const TREND_DAYS = 14;
const PROGRESS_LIMIT = 50;

/**
 * 驾驶舱（PC 专属）。六张图全部来自真实接口，每张图自带一句实算结论。
 * 图与结论都由 lib/charts.ts 的同一份转换结果生成，不会各说各话。
 */
export function DashboardPage() {
  const stats = useStats();
  const progress = useReadingProgress();
  const progressAll = useReadingProgressAll();
  const jobs = useImportJobs();
  const health = useHealthReport();
  const trend = useImportTrend(TREND_DAYS);
  const topAuthors = useTopAuthors(10);
  /** 近 90 天阅读时长：今日 / 本周 / 累计 / 连续天数 + 日历热力图 */
  const reading = useReadingStats(90);
  const [refreshing, setRefreshing] = useState(false);

  /** 刷新概览（契约 C-29）：重取全部数据源，卡片保持旧值直到新值回来。 */
  async function refreshOverview() {
    if (refreshing) return;
    setRefreshing(true);
    const started = Date.now();
    for (const source of [stats, progress, progressAll, jobs, health, trend, topAuthors, reading]) {
      source.reload();
    }
    const wait = Math.max(0, 240 - (Date.now() - started));
    window.setTimeout(() => {
      setRefreshing(false);
      showToast("概览已刷新");
    }, wait);
  }

  const loading = stats.loading && !stats.data;
  const error = stats.error;
  const empty = !loading && !error && (stats.data?.books ?? 0) === 0;

  const chartState = (
    source: { loading: boolean; error: unknown; reload: () => void },
    stale = false,
  ): ChartState => ({
    loading: source.loading,
    failed: Boolean(source.error),
    onRetry: source.reload,
    stale,
  });

  // 后端把统计改成查预计算表后，刷新窗口内会带 stale 标记：此时照常出图，只提示「数据更新中」
  const statsStale = Boolean(stats.data?.stale);
  const healthStale = Boolean(health.data?.stale);

  return (
    <section data-testid="view:dashboard" data-od-id="dashboard">
      <div className="page-head">
        <div>
          <p className="eyebrow">库总览</p>
          <h1>驾驶舱</h1>
          <p className="sub">书库状态、阅读进度与最近动态</p>
        </div>
        <button
          className="btn btn-sm"
          type="button"
          data-od-id="C-29"
          disabled={refreshing}
          onClick={() => void refreshOverview()}
        >
          {refreshing ? "正在刷新…" : "刷新概览"}
        </button>
      </div>

      {loading ? (
        <LoadingPanel title="正在读取书库概览" description="正在汇总书籍、章节与最近动态。" />
      ) : error ? (
        <ErrorPanel error={error} title="概览没能加载" onRetry={stats.reload} />
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
            <div className="dash-stats" data-od-id="dashboard:stats">
              <StatCard label="书籍总数" value={formatCount(stats.data?.books ?? 0)} hint="本" />
              <StatCard label="章节总数" value={formatCount(stats.data?.chapters ?? 0)} hint="章" />
              <StatCard label="作者" value={formatCount(stats.data?.authors ?? 0)} hint="位" />
              <StatCard label="总字数" value={formatCount(stats.data?.words ?? 0)} hint="字" />
            </div>

            <section className="dash-block" data-od-id="dashboard:reading">
              <div className="dash-block-head">
                <h2>最近在读</h2>
                <Link className="meta" to="/shelf" data-od-id="C-23">
                  全部书架 →
                </Link>
              </div>
              {progress.loading ? (
                <LoadingPanel title="正在读取阅读进度" description="稍等一下。" />
              ) : progress.error ? (
                <ErrorPanel error={progress.error} title="阅读进度没能加载" onRetry={progress.reload} />
              ) : (progress.data?.items.length ?? 0) === 0 ? (
                <EmptyPanel title="还没有阅读记录" description="从书架挑一本开始读，进度会自动记在这里。" />
              ) : (
                <div className="dash-list">
                  {progress.data?.items.map((item) => (
                    <Link
                      className="dash-row"
                      key={item.bookId}
                      to={`/read/${item.bookId}/${item.chapterId}`}
                      data-od-id="C-25"
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

            <section className="dash-block" data-od-id="dashboard:import">
              <div className="dash-block-head">
                <h2>导入概况</h2>
                <Link className="meta" to="/import" data-od-id="C-24">
                  前往导入管理 →
                </Link>
              </div>
              {jobs.loading ? (
                <LoadingPanel title="正在读取导入记录" description="稍等一下。" />
              ) : jobs.error ? (
                <ErrorPanel error={jobs.error} title="导入记录没能加载" onRetry={jobs.reload} />
              ) : (jobs.data?.items.length ?? 0) === 0 ? (
                <EmptyPanel title="还没有导入任务" description="去导入页选一个目录，开始第一次导入。" />
              ) : (
                <ImportSummary jobs={jobs.data?.items ?? []} />
              )}
            </section>

            <div className="chart-grid" data-testid="dashboard:charts" data-od-id="dashboard:charts">
              <ReadingHeatmapCard stats={reading.data} {...chartState(reading)} />
              <ImportTrendChart points={trend.data ?? []} {...chartState(trend)} />
              <TopAuthorsChart
                authors={(topAuthors.data?.items ?? []).map((a) => ({ name: a.name, wordCount: a.wordCount }))}
                totalWords={stats.data?.words ?? 0}
                {...chartState(topAuthors)}
              />
              <ChapterBucketsChart buckets={health.data?.chapterBuckets ?? []} {...chartState(health, healthStale)} />
              <ReadingDistChart
                progress={progressAll.data?.items ?? []}
                totalBooks={stats.data?.books ?? 0}
                limitReached={(progressAll.data?.items.length ?? 0) >= PROGRESS_LIMIT}
                // 后端已给聚合值时以它为准（不依赖 50 条上限），否则回退到估算
                buckets={health.data?.readingBuckets ?? null}
                {...chartState(progressAll || health)}
              />
              <StorageChart stats={stats.data} {...chartState(stats, statsStale)} />
              <ChapterLengthChart buckets={health.data?.chapterLengthBuckets ?? []} {...chartState(health, healthStale)} />
            </div>
          </div>

          <aside className="dash-side" data-od-id="dashboard:health">
            <section className="dash-block" data-od-id="dashboard:health-card">
              <div className="dash-block-head">
                <h2>库健康状况</h2>
              </div>
              {health.loading ? (
                <LoadingPanel title="正在检查书库" description="稍等一下。" />
              ) : health.error ? (
                <ErrorPanel error={health.error} title="健康状况没能读取" onRetry={health.reload} />
              ) : health.data ? (
                <>
                  <div className="health-item">
                    <div className="health-top">
                      <b>章节完整</b>
                      <span className="pill is-live">良好</span>
                    </div>
                    <p className="health-num num">
                      {health.data.totalBooks - health.data.thinBooks} / {health.data.totalBooks} 本
                    </p>
                    <p className="meta">
                      {health.data.thinBooks > 0
                        ? `${health.data.thinBooks} 本书的章节数偏少，可能是原文缺少章节标记。`
                        : "所有书都拆出了完整目录。"}
                    </p>
                  </div>
                  <div className={`health-item${health.data.missingAuthor + health.data.missingIntro > 0 ? " is-warn" : ""}`}>
                    <div className="health-top">
                      <b>待补信息</b>
                      <span className="pill is-warn">需处理</span>
                    </div>
                    <p className="health-num num">
                      {health.data.missingAuthor} 本缺作者 · {health.data.missingIntro} 本缺简介
                    </p>
                    <p className="meta">补全后，在书架和搜索结果里更容易分辨。</p>
                  </div>
                  <div className="health-item">
                    <div className="health-top">
                      <b>超长章节</b>
                      <span className="pill">留意</span>
                    </div>
                    <p className="health-num num">{health.data.hugeChapters} 个</p>
                    <p className="meta">单章超过 3 万字，可能是章节分界没识别到。</p>
                  </div>
                  <div className="health-item">
                    <div className="health-top">
                      <b>最近更新</b>
                    </div>
                    <p className="health-num num">{formatDate(stats.data?.lastImportAt ?? null)}</p>
                    <p className="meta">库里最新一次导入的时间。</p>
                  </div>
                  {/* C-27 / C-28（新增）：健康卡给出下一步入口，按指出的问题去补 */}
                  <div className="health-actions">
                    <Link className="btn btn-sm" to="/admin" data-od-id="C-27">
                      去书籍管理
                    </Link>
                    <Link className="btn btn-sm btn-ghost" to="/import" data-od-id="C-28">
                      去导入
                    </Link>
                  </div>
                </>
              ) : null}
            </section>
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