import { useEffect, useRef, useState } from "react";
import { useImportJobs } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { DryRunReport, ImportJob } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { ChapterRulesPanel } from "./ChapterRulesPanel";
import { DryRunReportPanel } from "./DryRunReport";
import { FailedFilesPanel } from "./FailedFilesPanel";
import { ImportHistory } from "./ImportHistory";
import { ImportProgressCard } from "./ImportProgressCard";
import { ImportSuccessPanel } from "./ImportSuccessPanel";
import { TagRulesPanel } from "./TagRulesPanel";
import { UploadPanel } from "./UploadPanel";
import { useTimedNote } from "./useTimedNote";

/** 运行中任务的轮询间隔：进度与计数靠这一次拉取更新。 */
const POLL_MS = 2000;
/** 同时处理文件数的合法区间（契约 C-05）。 */
const WORKERS_MIN = 1;
const WORKERS_MAX = 8;

/** 终态（不再变化）：见到它就停轮询，并刷新一次任务列表。 */
function isTerminal(status: ImportJob["status"]): boolean {
  return status === "completed" || status === "completed_with_errors" || status === "failed" || status === "canceled";
}

/**
 * 导入管理（PC 专属，H5 不渲染也不承认这个路由）。
 * 左栏是三个入口（路径 / 上传 / 两套规则），右栏是可预检结果、进度、失败清单与历史。
 */
export function ImportPage() {
  const { data, loading, error, reload } = useImportJobs();
  const [dir, setDir] = useState("");
  const [recursive, setRecursive] = useState(true);
  const [force, setForce] = useState(false);
  const [update, setUpdate] = useState(false);
  const [workers, setWorkers] = useState("4");
  const [busy, setBusy] = useState(false);
  const started = useTimedNote();
  const [canceling, setCanceling] = useState<number | null>(null);
  const [canceledNote, setCanceledNote] = useState("");
  const [live, setLive] = useState<ImportJob | null>(null);
  const [dryBusy, setDryBusy] = useState(false);
  const [dryReport, setDryReport] = useState<DryRunReport | null>(null);
  const [dryError, setDryError] = useState<ApiError | null>(null);
  const [dryNote, setDryNote] = useState("");
  const [dryDir, setDryDir] = useState("");
  const [dryElapsed, setDryElapsed] = useState(0);
  const dryAbort = useRef<AbortController | null>(null);
  const dryButton = useRef<HTMLButtonElement>(null);

  const jobs = data?.items ?? [];
  const running = jobs.find((j) => j.status === "running");
  const runningId = running?.id ?? null;
  // 进度卡只采信同一个任务的轮询快照，否则列表换单后还显示上一单的数字
  const current = running && live?.id === running.id ? live : running;
  const finished = jobs.filter((j) => j.status !== "running");
  const activeJob = running ?? jobs[0] ?? null;
  // 以 txt / epub / zip 结尾就按单文件处理：递归选项对它没有意义
  const isSingleFile = /\.(txt|epub|zip)$/i.test(dir.trim());
  const workersNum = Number(workers);
  const workersInvalid = workers.trim() !== "" && (!Number.isFinite(workersNum) || workersNum < WORKERS_MIN || workersNum > WORKERS_MAX);

  usePolling(runningId, setLive, setCanceling, setCanceledNote, reload);

  // 预检的已跑秒数：取消时照实显示，不假装瞬间完成（契约 C-21）
  useEffect(() => {
    if (!dryBusy) return;
    queueMicrotask(() => setDryElapsed(0));
    const timer = window.setInterval(() => setDryElapsed((n) => n + 1), 1000);
    return () => window.clearInterval(timer);
  }, [dryBusy]);

  const workersOf = () => {
    if (workers.trim() === "") return 1;
    const n = Math.round(workersNum);
    return Math.min(WORKERS_MAX, Math.max(WORKERS_MIN, n));
  };

  async function start() {
    if (busy) return;
    const target = dir.trim();
    if (!target) {
      started.show("先填一个目录或 txt 文件路径。");
      return;
    }
    setBusy(true);
    started.clear();
    try {
      await api.startImport({ dir: target, recursive, force, update, workers: workersOf() });
      started.show("任务已开始，进度会在下方的进度卡里更新。");
      reload();
    } catch (err) {
      started.show(
        err instanceof Error && "status" in err && (err as { status?: number }).status === 409
          ? "已经有一个导入任务在跑了，等它结束再开新的。"
          : "任务没能开始，检查一下路径是否存在。",
      );
    } finally {
      setBusy(false);
    }
  }

  /** 只预检：不建任务、不占名额；60 秒上限由这一次请求自己的超时兜住。 */
  async function runDryRun() {
    const target = dir.trim();
    if (!target) {
      setDryNote("先填一个目录或 txt 文件路径，再预检。");
      return;
    }
    const controller = new AbortController();
    dryAbort.current = controller;
    setDryBusy(true);
    setDryNote("");
    setDryError(null);
    setDryReport(null);
    setDryDir(target);
    try {
      const report = await api.dryRunImport(
        { dir: target, recursive, force, update, workers: workersOf() },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) return;
      setDryReport(report);
    } catch (err) {
      if (controller.signal.aborted) return;
      setDryError(err instanceof ApiError ? err : new ApiError("network", err instanceof Error ? err.message : "预检失败"));
    } finally {
      if (!controller.signal.aborted) setDryBusy(false);
      dryAbort.current = null;
    }
  }

  /** 取消这次预检：报告收起、回到未预检状态，焦点回「只预检」。 */
  function cancelDryRun() {
    dryAbort.current?.abort();
    dryAbort.current = null;
    setDryBusy(false);
    setDryError(null);
    setDryReport(null);
    setDryDir("");
    setDryNote("");
    dryButton.current?.focus();
  }

  async function cancelJob(job: ImportJob) {
    setCanceling(job.id);
    setCanceledNote("");
    try {
      await api.cancelImport(job.id);
    } catch (err) {
      setCanceling(null);
      setCanceledNote(cancelErrorText(err));
    }
  }

  return (
    <section className="ix-ops" data-testid="view:import">
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
          <form
            className="import-form"
            onSubmit={(event) => {
              event.preventDefault();
              void start();
            }}
          >
            <h2>新建导入任务</h2>
            <div className="field">
              <label htmlFor="dir">目录或单个文件路径</label>
              <input
                className="input"
                id="dir"
                value={dir}
                placeholder="例如 文稿/小说，或 文稿/小说/示例书简.epub"
                data-od-id="C-01"
                onChange={(e) => setDir(e.target.value)}
              />
              {dir.trim() === "" ? <p className="inline-note">先填一个目录或 txt 文件路径。</p> : null}
              {isSingleFile ? (
                <p className="inline-note">填到 .txt / .epub / .zip 就按单文件处理。</p>
              ) : null}
            </div>
            <div className="check-row">
              <input
                id="rec"
                type="checkbox"
                checked={recursive && !isSingleFile}
                disabled={isSingleFile}
                data-od-id="C-02"
                onChange={(e) => setRecursive(e.target.checked)}
              />
              <label htmlFor="rec">递归扫描子目录{isSingleFile ? "（单个文件用不上）" : ""}</label>
            </div>
            <div className="check-row">
              <input
                id="force"
                type="checkbox"
                checked={force}
                data-od-id="C-03"
                onChange={(e) => {
                  setForce(e.target.checked);
                  if (e.target.checked) setUpdate(false);
                }}
              />
              <label htmlFor="force">重新导入已入库的文件</label>
            </div>
            {force ? <p className="inline-note">重新导入会覆盖已有章节。</p> : null}
            <div className="check-row">
              <input
                id="update"
                type="checkbox"
                checked={update}
                data-od-id="C-04"
                onChange={(e) => {
                  setUpdate(e.target.checked);
                  if (e.target.checked) setForce(false);
                }}
              />
              <label htmlFor="update">追更：同一本书只追加新章节（保留阅读进度）</label>
            </div>
            {update ? <p className="inline-note is-fade">只追加新章节，已有章节与阅读进度保留；章节对不上时整本拒绝，不覆盖。</p> : null}
            <div className="field">
              <label htmlFor="workers">同时处理的文件数</label>
              <input
                className={`input${workersInvalid ? " is-invalid" : ""}`}
                id="workers"
                type="number"
                min={WORKERS_MIN}
                max={WORKERS_MAX}
                value={workers}
                data-od-id="C-05"
                onChange={(e) => setWorkers(e.target.value)}
              />
              {workersInvalid ? (
                <p className="inline-note" data-testid="import-workers-hint">
                  同时处理 1–8 个，超出按 8 算
                </p>
              ) : null}
            </div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <button className="btn btn-primary" type="submit" disabled={busy} data-od-id="C-06">
                {busy ? "正在开始…" : "开始导入"}
              </button>
              <button
                className="btn"
                type="button"
                ref={dryButton}
                data-testid="import-dry-run"
                disabled={busy || dryBusy}
                data-od-id="C-07"
                onClick={() => void runDryRun()}
              >
                {dryBusy ? "正在预检…" : "只预检"}
              </button>
            </div>
            <p className="meta">同一时间只能运行一个导入任务；只预检不建任务、不占名额，随时可点。</p>
            {started.note ? (
              <p className="meta" role="status">
                {started.note}
              </p>
            ) : null}
            {dryNote ? (
              <p className="meta" role="status" data-testid="dry-run-note">
                {dryNote}
              </p>
            ) : null}
          </form>
          <UploadPanel onSubmitted={reload} disabled={Boolean(running)} />
          <ChapterRulesPanel />
          <TagRulesPanel />
        </div>

        <div>
          {dryBusy ? (
            <>
              <LoadingPanel
                title="正在预检"
                description="扫描目录并试解析文件，最多 60 秒；不写库、不建任务。"
                testId="dry-run-loading"
              />
              <div className="dry-run-actions">
                <span className="meta">已跑 {dryElapsed} 秒</span>
                <button className="btn btn-ghost" type="button" data-testid="dry-run-cancel" data-od-id="C-21" onClick={cancelDryRun}>
                  取消这次预检
                </button>
              </div>
            </>
          ) : null}
          {dryError ? (
            <ErrorPanel error={dryError} title="预检没能完成" onRetry={() => void runDryRun()} testId="dry-run-error" />
          ) : null}
          {dryError && isDryTimeout(dryError) ? (
            <p className="inline-note" data-testid="dry-run-timeout">
              预检超过 60 秒会中断，可缩小目录范围重试
            </p>
          ) : null}
          {dryReport ? <DryRunReportPanel report={dryReport} dir={dryDir} /> : null}

          {loading ? (
            <LoadingPanel title="正在读取任务" description="正在取回最近的导入记录。" />
          ) : error ? (
            <ErrorPanel error={error} title="任务列表没能加载" onRetry={reload} />
          ) : current ? (
            <ImportProgressCard job={current} canceling={canceling === current.id} onCancel={(job) => void cancelJob(job)} />
          ) : (
            <EmptyPanel title="当前没有进行中的任务" description="拖入文件或填好左边的路径，开始一次导入。" />
          )}

          {canceledNote ? (
            <p className="meta" role="status" data-testid="import-cancel-note">
              {canceledNote}
            </p>
          ) : null}

          {activeJob && !running && isTerminal(activeJob.status) ? <ImportSuccessPanel job={activeJob} /> : null}

          {activeJob ? <FailedFilesPanel key={activeJob.id} job={activeJob} onRetried={() => reload()} /> : null}

          <ImportHistory jobs={finished} />
        </div>
      </div>
    </section>
  );
}

/** 运行中任务的进度轮询：进页面先拉一次，之后每 2 秒一次，见到终态就停并刷新列表。 */
function usePolling(
  runningId: number | null,
  setLive: (job: ImportJob | null) => void,
  setCanceling: (id: number | null) => void,
  setCanceledNote: (text: string) => void,
  reload: () => void,
) {
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
  }, [runningId, setLive, setCanceling, setCanceledNote, reload]);
}

/** 预检超时（服务端 503 与客户端超时同口径）：提示缩小目录范围。 */
function isDryTimeout(error: ApiError): boolean {
  return error.kind === "timeout" || error.status === 503;
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
