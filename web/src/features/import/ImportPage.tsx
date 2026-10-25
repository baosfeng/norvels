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

/** 运行中任务的轮询间隔：后端没有推送通道，进度条与计数靠这一次拉取更新。 */
const POLL_MS = 2000;

/** 终态（不再变化）：见到它就停轮询，并刷新一次任务列表。 */
function isTerminal(status: ImportJob["status"]): boolean {
  return status === "completed" || status === "completed_with_errors" || status === "failed" || status === "canceled";
}

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
  /** 轮询拿到的最新任务快照（同一个 id 才认）：列表里的那份只在进页面时取一次 */
  const [live, setLive] = useState<ImportJob | null>(null);
  // 只预检：报告是同步返回的只读结果，不建任务
  const [dryBusy, setDryBusy] = useState(false);
  const [dryReport, setDryReport] = useState<DryRunReport | null>(null);
  const [dryError, setDryError] = useState<ApiError | null>(null);
  const [dryDir, setDryDir] = useState("");

  const jobs = data?.items ?? [];
  const running = jobs.find((j) => j.status === "running");
  const runningId = running?.id ?? null;
  // 进度卡读这份：轮询快照与列表里的 running 是同一个任务才采信快照，
  // 否则列表已经换了任务、还显示上一单的进度
  const current = running && live?.id === running.id ? live : running;
  const finished = jobs.filter((j) => j.status !== "running");
  // 失败清单看最新任务：正在跑的优先，否则最近一条历史
  const activeJob = running ?? jobs[0] ?? null;
  // 以 txt / epub / zip 结尾就按单文件处理：递归选项对它没有意义，勾上也不会改变结果。
  const isSingleFile = /\.(txt|epub|zip)$/i.test(dir.trim());

  // 运行中任务的进度轮询：进页面先拉一次，之后每 2 秒一次，见到终态就停并刷新列表。
  // 启动条件 = 列表里存在 running 任务；停止条件 = 终态 / 任务从列表消失（runningId 变 null）/ 卸载。
  useEffect(() => {
    if (runningId === null) return;
    let active = true;
    let stopped = false;
    let timer = 0;
    const stop = () => {
      stopped = true;
      window.clearInterval(timer);
    };
    const tick = async () => {
      if (!active || stopped) return;
      try {
        const job = await api.importJob(runningId);
        if (!active || stopped) return;
        if (isTerminal(job.status)) {
          stop();
          setLive(null);
          setCanceling(null);
          if (job.status === "canceled") {
            setCanceledNote(`任务 #${runningId} 已取消，已导入的 ${job.imported} 本书保留在书架。`);
          }
          reload();
          return;
        }
        setLive(job);
      } catch {
        /* 单次轮询失败不中断：下一轮再试，卸载时整体清掉 */
      }
    };
    void tick();
    timer = window.setInterval(() => void tick(), POLL_MS);
    return () => {
      active = false;
      stop();
    };
  }, [runningId, reload]);

  async function cancelJob(job: ImportJob) {
    setCanceling(job.id);
    setCanceledNote("");
    try {
      // 结果由进度轮询发现（见到 canceled 才给提示），不等列表刷新碰运气
      await api.cancelImport(job.id);
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
          ) : current ? (
            <div className="import-progress" data-testid="import-run">
              <div className="top">
                <span className="pill is-live">处理中</span>
                <h2>任务 #{current.id}</h2>
                <span className="meta" style={{ marginLeft: "auto" }}>
                  开始于 {formatDate(current.startedAt)}
                </span>
                {canceling === current.id ? (
                  <span className="meta" data-testid="import-cancel-pending">
                    正在取消…
                  </span>
                ) : (
                  <button
                    className="btn btn-ghost btn-danger"
                    type="button"
                    onClick={() => void cancelJob(current)}
                    data-testid="import-cancel"
                  >
                    取消任务
                  </button>
                )}
              </div>
              <div className="progress">
                <span
                  data-testid="import-progress-bar"
                  style={{ width: `${progressOf(current.imported + current.skipped + current.failed, current.total)}%` }}
                />
              </div>
              <div className="counters">
                <div>
                  <strong className="num" data-testid="import-progress-done">
                    {current.imported + current.skipped + current.failed}
                  </strong>
                  <span>/ {current.total} 文件</span>
                </div>
                <div>
                  <strong className="num">{current.imported}</strong>
                  <span>已导入</span>
                </div>
                <div>
                  <strong className="num">{current.skipped}</strong>
                  <span>已跳过</span>
                </div>
                <div>
                  <strong className="num">{current.failed}</strong>
                  <span>失败</span>
                </div>
              </div>
              <p className="meta" style={{ wordBreak: "break-all" }}>
                {current.message || `正在处理：${current.sourceDir}`}
              </p>
              {current.failed > 0 ? (
                <button
                  className="failure-toggle"
                  type="button"
                  aria-expanded={showErrors}
                  onClick={() => setShowErrors((v) => !v)}
                >
                  <span>{current.failed} 个文件未能导入</span>
                  <span>{showErrors ? "收起" : "查看"}</span>
                </button>
              ) : null}
              {showErrors && current.failed > 0 ? (
                <div className="import-errors">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>文件</th>
                        <th>原因</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(current.errors ?? []).map((e) => (
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