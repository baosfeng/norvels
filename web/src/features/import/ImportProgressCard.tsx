import { useRef, useState } from "react";
import type { ImportJob } from "../../api/types";
import { formatDate } from "../../lib/format";

/** 进度百分比：总数未知（0）时不算进度，避免除以 0。 */
export function progressOf(done: number, total: number): number {
  if (!total || total <= 0) return 0;
  return Math.min(100, Math.round((done / total) * 100));
}

/**
 * 进行中任务的进度卡：计数 + 进度条 + 取消（二次确认）+ 失败明细折叠条。
 *
 * 取消是「不可撤销且代价高」之外的一类——任务已产出的数据不丢，但要重新跑，
 * 所以按契约走二次确认，默认焦点落在「继续跑」，避免一次回车就把任务停了。
 */
export function ImportProgressCard({
  job,
  canceling,
  onCancel,
}: {
  job: ImportJob;
  canceling: boolean;
  onCancel: (job: ImportJob) => void;
}) {
  const [confirming, setConfirming] = useState(false);
  const [showErrors, setShowErrors] = useState(false);
  const cancelRef = useRef<HTMLButtonElement>(null);

  function closeConfirm() {
    setConfirming(false);
    // 焦点回进度卡上的「取消任务」：弹层是从这里打开的，关掉要回到原处
    cancelRef.current?.focus();
  }

  return (
    <div className="import-progress" data-testid="import-run" tabIndex={-1}>
      <div className="top">
        <span className="pill is-live">处理中</span>
        <h2>任务 #{job.id}</h2>
        <span className="meta" style={{ marginLeft: "auto" }}>
          开始于 {formatDate(job.startedAt)}
        </span>
        {canceling ? (
          <span className="meta" data-testid="import-cancel-pending">
            正在取消…
          </span>
        ) : (
          <button
            className="btn btn-ghost btn-danger"
            type="button"
            ref={cancelRef}
            onClick={() => setConfirming(true)}
            data-testid="import-cancel"
            data-od-id="C-22"
          >
            取消任务
          </button>
        )}
      </div>
      <div className="progress">
        <span
          data-testid="import-progress-bar"
          style={{ width: `${progressOf(job.imported + job.skipped + job.failed, job.total)}%` }}
        />
      </div>
      <div className="counters">
        <div>
          <strong className="num" data-testid="import-progress-done">
            {job.imported + job.skipped + job.failed}
          </strong>
          <span>/ {job.total} 文件</span>
        </div>
        <div>
          <strong className="num">{job.imported}</strong>
          <span>已导入</span>
        </div>
        <div>
          <strong className="num">{job.skipped}</strong>
          <span>已跳过</span>
        </div>
        <div>
          <strong className="num">{job.failed}</strong>
          <span>失败</span>
        </div>
      </div>
      <p className="meta" style={{ wordBreak: "break-all" }}>
        {job.message || `正在处理：${job.sourceDir}`}
      </p>
      {job.failed > 0 ? (
        <button
          className="failure-toggle"
          type="button"
          aria-expanded={showErrors}
          data-od-id="C-23"
          onClick={() => setShowErrors((v) => !v)}
        >
          <span>{job.failed} 个文件未能导入</span>
          <span>{showErrors ? "收起" : "查看"}</span>
        </button>
      ) : null}
      {showErrors && job.failed > 0 ? (
        <div className="import-errors is-scroll" data-testid="import-errors">
          <table className="table">
            <thead>
              <tr>
                <th>文件</th>
                <th>原因</th>
              </tr>
            </thead>
            <tbody>
              {(job.errors ?? []).map((e) => (
                <tr key={e.filePath}>
                  <td style={{ wordBreak: "break-all" }}>{e.filePath}</td>
                  <td>{e.reason}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {confirming ? (
        <div className="confirm-backdrop" role="presentation" onClick={closeConfirm}>
          <div
            className="confirm-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="cancel-job-title"
            data-testid="import-cancel-confirm"
            onClick={(event) => event.stopPropagation()}
          >
            <h2 id="cancel-job-title">取消任务 #{job.id}？</h2>
            <p className="meta">
              任务会停在当前进度，已经导入的书保留在书架；想继续可以重新开一次任务。
            </p>
            <div className="confirm-actions">
              <button className="btn btn-primary" type="button" autoFocus onClick={closeConfirm} data-testid="import-cancel-stay">
                继续跑
              </button>
              <button
                className="btn btn-danger"
                type="button"
                data-testid="import-cancel-confirm-btn"
                onClick={() => {
                  setConfirming(false);
                  onCancel(job);
                }}
              >
                确认取消
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
