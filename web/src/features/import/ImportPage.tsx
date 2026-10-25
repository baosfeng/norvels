import { useEffect, useState, type FormEvent } from "react";
import { useImportJobs } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { DryRunReport, ImportJob } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatDate } from "../../lib/format";
import { ChapterRulesPanel } from "./ChapterRulesPanel";
import { DryRunReportPanel } from "./DryRunReport";
import { FailedFilesPanel } from "./FailedFilesPanel";
import { TagRulesPanel } from "./TagRulesPanel";
import { UploadPanel } from "./UploadPanel";

/**
 * 导入管理（PC 专属，H5 不渲染也不承认这个路由）。
 * 表单 / 进度 / 失败明细 / 任务历史四块，当前任务由进度卡独占、历史只列已结束任务。
 */
export function ImportPage() {
  const { data, loading, error, reload } = useImportJobs();
  const [dir, setDir] = useState("");
  const [recursive, setRecursive] = useState(true);
  const [force, setForce] = useState(false);
  const [update, setUpdate] = useState(false);
  const [workers, setWorkers] = useState(4);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [showErrors, setShowErrors] = useState(false);
  // 正在取消的任务 id（按钮变为「正在取消…」），以及取消结果的人话提示
  const [canceling, setCanceling] = useState<number | null>(null);
  const [canceledNote, setCanceledNote] = useState("");
  // 取消发出后要轮询的任务 id：等到 canceled 才停，不等列表刷新碰运气
  const [watchId, setWatchId] = useState<number | null>(null);
  // 只预检：报告是同步返回的只读结果，不建任务
  const [dryBusy, setDryBusy] = useState(false);
  const [dryReport, setDryReport] = useState<DryRunReport | null>(null);
  const [dryError, setDryError] = useState<ApiError | null>(null);
  const [dryDir, setDryDir] = useState("");

  const jobs = data?.items ?? [];
  const running = jobs.find((j) => j.status === "running");
  const finished = jobs.filter((j) => j.status !== "running");
  // 失败清单看最新任务：正在跑的优先，否则最近一条历史
  const activeJob = running ?? jobs[0] ?? null;
  // 以 txt / epub / zip 结尾就按单文件处理：递归选项对它没有意义，勾上也不会改变结果。
  const isSingleFile = /\.(txt|epub|zip)$/i.test(dir.trim());

  // 取消后的轮询：每 2 秒问一次任务状态，见到 canceled / 已结束就停并刷新列表
  useEffect(() => {
    if (watchId === null) return;
    let active = true;
    const tick = async () => {
      try {
        const job = await api.importJob(watchId);
        if (!active) return;
        if (job.status === "canceled") {
          setCanceling(null);
          setCanceledNote(`任务 #${watchId} 已取消，已导入的 ${job.imported} 本书保留在书架。`);
          setWatchId(null);
          reload();
        } else if (job.status === "completed" || job.status === "completed_with_errors" || job.status === "failed") {
          setCanceling(null);
          setWatchId(null);
          reload();
        }
      } catch {
        /* 单次轮询失败不中断：下一轮再试，卸载时整体清掉 */
      }
    };
    void tick();
    const timer = window.setInterval(() => void tick(), 2000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [watchId, reload]);

  async function cancelJob(job: ImportJob) {
    setCanceling(job.id);
    setCanceledNote("");
    try {
      await api.cancelImport(job.id);
      setWatchId(job.id);
    } catch (err) {
      setCanceling(null);
      setCanceledNote(cancelErrorText(err));
    }
  }

  async function start(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!dir.trim()) {
      setMessage("先填一个目录或 txt 文件路径。");
      return;
    }
    setBusy(true);
    setMessage("");
    try {
      await api.startImport({ dir: dir.trim(), recursive, force, update, workers });
      setMessage("任务已开始，进度会在下方的进度卡里更新。");
      reload();
    } catch (err) {
      setMessage(
        err instanceof Error && "status" in err && (err as { status?: number }).status === 409
          ? "已经有一个导入任务在跑了，等它结束再开新的。"
          : "任务没能开始，检查一下路径是否存在。",
      );
    } finally {
      setBusy(false);
    }
  }

  /** 只预检（dry-run）：不建任务、不占锁；服务端最长 60 秒，超时回 503。 */
  async function runDryRun() {
    const target = dir.trim();
    if (!target) {
      setMessage("先填一个目录或 txt 文件路径，再预检。");
      return;
    }
    setDryBusy(true);
    setDryError(null);
    setDryReport(null);
    setDryDir(target);
    try {
      setDryReport(await api.dryRunImport({ dir: target, recursive, force, update, workers }));
    } catch (err) {
      setDryError(err instanceof ApiError ? err : new ApiError("network", err instanceof Error ? err.message : "预检失败"));
    } finally {
      setDryBusy(false);
    }
  }

  return (
    <section data-testid="view:import" data-od-id="import">
      <div className="page-head">
        <div>
          <p className="eyebrow">导入管理</p>
          <h1>把书变成书架</h1>
          <p className="sub">拖入或选择 txt / epub / zip 直接导入，也可以填目录批量扫描；同一文件不会重复导入</p>
        </div>
        <span className="pill">{running ? "1 个进行中的任务" : "0 个排队任务"}</span>
      </div>

      <div className="import-layout">
        <div style={{ display: "grid", gap: 16 }}>
        <form className="import-form" onSubmit={start}>
          <h2>新建导入任务</h2>
          <div className="field">
            <label htmlFor="dir">目录或单个文件路径</label>
            <input
              className="input"
              id="dir"
              value={dir}
              onChange={(e) => setDir(e.target.value)}
              placeholder="例如 文稿/小说，或 文稿/小说/示例书简.epub"
            />
          </div>
          <div className="check-row">
            <input
              id="rec"
              type="checkbox"
              checked={recursive && !isSingleFile}
              disabled={isSingleFile}
              onChange={(e) => setRecursive(e.target.checked)}
            />
            <label htmlFor="rec">递归扫描子目录{isSingleFile ? "（单个文件用不上）" : ""}</label>
          </div>
          <div className="check-row">
            <input id="force" type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
            <label htmlFor="force">重新导入已入库的文件</label>
          </div>
          <div className="check-row">
            <input id="update" type="checkbox" checked={update} onChange={(e) => setUpdate(e.target.checked)} />
            <label htmlFor="update">追更：同一本书只追加新章节（保留阅读进度）</label>
          </div>
          <div className="field">
            <label htmlFor="workers">同时处理的文件数</label>
            <input
              className="input"
              id="workers"
              type="number"
              min={1}
              max={8}
              value={workers}
              onChange={(e) => setWorkers(Number(e.target.value) || 1)}
            />
          </div>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <button className="btn btn-primary" type="submit" disabled={busy}>
              {busy ? "正在开始…" : "开始导入"}
            </button>
            <button
              className="btn"
              type="button"
              data-testid="import-dry-run"
              disabled={busy || dryBusy}
              onClick={() => void runDryRun()}
            >
              {dryBusy ? "正在预检…" : "只预检"}
            </button>
          </div>
          <p className="meta">同一时间只能运行一个导入任务；只预检不建任务、不占锁，随时可点。</p>
          {message ? <p className="meta" role="status">{message}</p> : null}
        </form>
        <UploadPanel onSubmitted={reload} disabled={Boolean(running)} />
        <ChapterRulesPanel />
        <TagRulesPanel />
        </div>

        <div>
          {dryBusy ? (
            <LoadingPanel
              title="正在预检"
              description="扫描目录并试解析文件，最多 60 秒；不写库、不建任务。"
              testId="dry-run-loading"
            />
          ) : null}
          {dryError ? (
            <ErrorPanel error={dryError} title="预检没能完成" onRetry={() => void runDryRun()} testId="dry-run-error" />
          ) : null}
          {dryReport ? <DryRunReportPanel report={dryReport} dir={dryDir} /> : null}

          {loading ? (
            <LoadingPanel title="正在读取任务" description="正在取回最近的导入记录。" />
          ) : error ? (
            <ErrorPanel error={error} title="任务列表没能加载" onRetry={reload} />
          ) : running ? (
            <div className="import-progress" data-testid="import-run">
              <div className="top">
                <span className="pill is-live">处理中</span>
                <h2>任务 #{running.id}</h2>
                <span className="meta" style={{ marginLeft: "auto" }}>
                  开始于 {formatDate(running.startedAt)}
                </span>
                {canceling === running.id ? (
                  <span className="meta" data-testid="import-cancel-pending">
                    正在取消…
                  </span>
                ) : (
                  <button
                    className="btn btn-ghost btn-danger"
                    type="button"
                    onClick={() => void cancelJob(running)}
                    data-testid="import-cancel"
                  >
                    取消任务
                  </button>
                )}
              </div>
              <div className="progress">
                <span style={{ width: `${progressOf(running.imported + running.skipped + running.failed, running.total)}%` }} />
              </div>
              <div className="counters">
                <div>
                  <strong className="num">{running.imported + running.skipped + running.failed}</strong>
                  <span>/ {running.total} 文件</span>
                </div>
                <div>
                  <strong className="num">{running.imported}</strong>
                  <span>已导入</span>
                </div>
                <div>
                  <strong className="num">{running.skipped}</strong>
                  <span>已跳过</span>
                </div>
                <div>
                  <strong className="num">{running.failed}</strong>
                  <span>失败</span>
                </div>
              </div>
              <p className="meta" style={{ wordBreak: "break-all" }}>
                {running.message || `正在处理：${running.sourceDir}`}
              </p>
              {running.failed > 0 ? (
                <button
                  className="failure-toggle"
                  type="button"
                  aria-expanded={showErrors}
                  onClick={() => setShowErrors((v) => !v)}
                >
                  <span>{running.failed} 个文件未能导入</span>
                  <span>{showErrors ? "收起" : "查看"}</span>
                </button>
              ) : null}
              {showErrors && running.failed > 0 ? (
                <div className="import-errors">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>文件</th>
                        <th>原因</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(running.errors ?? []).map((e) => (
                        <tr key={e.filePath}>
                          <td>{e.filePath}</td>
                          <td>{e.reason}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : null}
            </div>
          ) : (
            <EmptyPanel title="当前没有进行中的任务" description="拖入文件或填好左边的路径，开始一次导入。" />
          )}

          {canceledNote ? (
            <p className="meta" role="status" data-testid="import-cancel-note">
              {canceledNote}
            </p>
          ) : null}

          {activeJob ? (
            <FailedFilesPanel key={activeJob.id} job={activeJob} onRetried={() => reload()} />
          ) : null}

          <div className="import-jobs">
            <div className="head">
              <h2>任务历史</h2>
              <span className="meta">已结束的任务 · 当前任务见上方进度卡</span>
            </div>
            {finished.length === 0 ? (
              <p className="meta" style={{ padding: 16 }}>
                还没有结束的任务。
              </p>
            ) : (
              <table className="table">
                <thead>
                  <tr>
                    <th>任务</th>
                    <th>来源</th>
                    <th className="num">导入</th>
                    <th className="num">跳过</th>
                    <th className="num">失败</th>
                    <th>状态</th>
                    <th>开始</th>
                  </tr>
                </thead>
                <tbody>
                  {finished.map((job) => (
                    <tr key={job.id}>
                      <td>#{job.id}</td>
                      <td>{job.sourceDir}</td>
                      <td className="num">{job.imported}</td>
                      <td className="num">{job.skipped}</td>
                      <td className="num">{job.failed}</td>
                      <td>
                        <span
                          className={`pill${
                            job.status === "failed" || job.status === "completed_with_errors" || job.failed > 0
                              ? " is-warn"
                              : ""
                          }`}
                        >
                          {statusLabel(job.status, job.failed)}
                        </span>
                      </td>
                      <td className="meta">{formatDate(job.startedAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}

function progressOf(done: number, total: number): number {
  if (!total || total <= 0) return 0;
  return Math.min(100, Math.round((done / total) * 100));
}

/** 取消失败的文案：任务消失 / 已结束 / 其它，都说成人话。 */
function cancelErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "这个任务已经不在了，刷新页面试试。";
    if (error.status === 409) return "任务已经结束了，不用再取消。";
    if (error.kind === "timeout") return "取消请求超时了，稍后再试。";
  }
  return "取消没能生效，稍后再试。";
}

function statusLabel(status: string, failed: number): string {
  // 部分失败是「完成了但有失败文件」，不能显示成排队中
  if (status === "completed_with_errors") return "完成（部分失败）";
  if (status === "completed") return failed > 0 ? "部分未成功" : "已完成";
  if (status === "failed") return "中断";
  if (status === "canceled") return "已取消";
  return "排队中";
}