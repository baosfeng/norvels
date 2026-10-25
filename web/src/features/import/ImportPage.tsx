import { useState, type FormEvent } from "react";
import { useImportJobs } from "../../api/hooks";
import { api } from "../../api/client";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatDate } from "../../lib/format";

/**
 * 导入管理（PC 专属，H5 不渲染也不承认这个路由）。
 * 表单 / 进度 / 失败明细 / 任务历史四块，当前任务由进度卡独占、历史只列已结束任务。
 */
export function ImportPage() {
  const { data, loading, error, reload } = useImportJobs();
  const [dir, setDir] = useState("");
  const [recursive, setRecursive] = useState(true);
  const [force, setForce] = useState(false);
  const [workers, setWorkers] = useState(4);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [showErrors, setShowErrors] = useState(false);

  const jobs = data?.items ?? [];
  const running = jobs.find((j) => j.status === "running");
  const finished = jobs.filter((j) => j.status !== "running");

  async function start(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!dir.trim()) {
      setMessage("先填一个目录路径。");
      return;
    }
    setBusy(true);
    setMessage("");
    try {
      await api.startImport({ dir: dir.trim(), recursive, force, workers });
      setMessage("任务已开始，进度会在下方的进度卡里更新。");
      reload();
    } catch (err) {
      setMessage(
        err instanceof Error && "status" in err && (err as { status?: number }).status === 409
          ? "已经有一个导入任务在跑了，等它结束再开新的。"
          : "任务没能开始，检查一下目录路径是否存在。",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <section data-testid="view:import" data-od-id="import">
      <div className="page-head">
        <div>
          <p className="eyebrow">导入管理</p>
          <h1>把 txt 目录变成书架</h1>
          <p className="sub">自动识别目录下的 txt 文件，同一文件不会重复导入</p>
        </div>
        <span className="pill">{running ? "1 个进行中的任务" : "0 个排队任务"}</span>
      </div>

      <div className="import-layout">
        <form className="import-form" onSubmit={start}>
          <h2>新建导入任务</h2>
          <div className="field">
            <label htmlFor="dir">目录路径</label>
            <input
              className="input"
              id="dir"
              value={dir}
              onChange={(e) => setDir(e.target.value)}
              placeholder="例如 文稿/小说"
            />
          </div>
          <div className="check-row">
            <input id="rec" type="checkbox" checked={recursive} onChange={(e) => setRecursive(e.target.checked)} />
            <label htmlFor="rec">递归扫描子目录</label>
          </div>
          <div className="check-row">
            <input id="force" type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
            <label htmlFor="force">重新导入已入库的文件</label>
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
          <button className="btn btn-primary" type="submit" disabled={busy}>
            {busy ? "正在开始…" : "开始导入"}
          </button>
          <p className="meta">同一时间只能运行一个导入任务。</p>
          {message ? <p className="meta" role="status">{message}</p> : null}
        </form>

        <div>
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
            <EmptyPanel title="当前没有进行中的任务" description="填好左边的目录路径，开始一次导入。" />
          )}

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
                    <th>目录</th>
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
                        <span className={`pill${job.status === "failed" || job.failed > 0 ? " is-warn" : ""}`}>
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

function statusLabel(status: string, failed: number): string {
  if (status === "completed") return failed > 0 ? "部分未成功" : "已完成";
  if (status === "failed") return "中断";
  if (status === "canceled") return "已取消";
  return "排队中";
}