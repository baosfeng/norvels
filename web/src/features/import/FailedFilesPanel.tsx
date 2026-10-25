import { useState } from "react";
import { ApiError, api } from "../../api/client";
import type { ImportJob } from "../../api/types";
import { useFailedFiles } from "../../api/hooks";
import { ErrorPanel } from "../../components/StatePanel";

/**
 * 失败文件清单（契约 §2.2 的 /files?status=failed）：每行可单独重试，顶部可一键重试全部。
 * 重试生成新任务、原任务记录不变；没有失败项时整块不渲染，不占版面。
 */
export function FailedFilesPanel({
  job,
  onRetried,
}: {
  job: ImportJob;
  onRetried: (jobId: number, count: number) => void;
}) {
  const files = useFailedFiles(job.id);
  // 正在重试的源（"__all__" 表示全部）；同一时刻只发一个重试请求
  const [pending, setPending] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const list = files.data?.files ?? [];
  const failedCount = job.failedCount ?? job.failed;

  async function retry(sources?: string[]) {
    if (pending) return;
    setPending(sources ? sources[0] : "__all__");
    setNote("");
    try {
      const result = await api.retryImport(job.id, sources);
      const count = sources?.length ?? list.length;
      setNote(
        `已新建重试任务 #${result.jobId}，处理 ${count} 个失败文件；原任务记录不变，进度见上方任务卡。`,
      );
      onRetried(result.jobId, count);
    } catch (err) {
      setNote(retryErrorText(err));
    } finally {
      setPending(null);
    }
  }

  if (files.loading) {
    return (
      <div className="import-form" data-testid="failed-files">
        <p className="meta" data-testid="failed-files-loading">
          正在读取失败文件…
        </p>
      </div>
    );
  }
  if (files.error) {
    return (
      <div className="import-form" data-testid="failed-files">
        <ErrorPanel error={files.error} title="失败清单没能加载" onRetry={files.reload} testId="failed-files-error" />
      </div>
    );
  }
  if (list.length === 0) return null;

  return (
    <div className="import-form" data-testid="failed-files">
      <div className="jobs-head">
        <h2>没能导入的文件</h2>
        <span className="meta num">
          {failedCount} 个 · 任务 #{job.id}
        </span>
      </div>
      <button
        className="btn"
        type="button"
        data-testid="retry-all-failed"
        disabled={pending !== null}
        onClick={() => void retry()}
      >
        {pending === "__all__" ? "正在重试…" : "重试全部失败"}
      </button>
      <div className="import-errors">
        <table className="table">
          <thead>
            <tr>
              <th>文件</th>
              <th>原因</th>
              <th>重试</th>
            </tr>
          </thead>
          <tbody>
            {list.map((file) => (
              <tr key={file.id} data-testid={`failed-file-${file.id}`}>
                <td style={{ wordBreak: "break-all" }}>{file.displayPath || file.source}</td>
                <td>{file.reason || "未知原因"}</td>
                <td>
                  <button
                    className="btn btn-ghost"
                    type="button"
                    aria-label={`重试 ${file.displayPath || file.source}`}
                    disabled={pending !== null}
                    onClick={() => void retry([file.source])}
                  >
                    {pending === file.source ? "正在重试…" : "重试"}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {note ? (
        <p className="meta" role="status" data-testid="retry-note">
          {note}
        </p>
      ) : null}
    </div>
  );
}

/** 重试失败的文案：任务不在 / 还在跑 / 没有可重试项 / 超时，都说成人话。 */
function retryErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "这个任务已经不在了，刷新页面试试。";
    if (error.status === 409) return "任务还在跑，等它结束再重试失败项。";
    if (error.status === 400) return "这批文件不在该任务里，或者已经没有可重试的失败项。";
    if (error.kind === "timeout") return "重试请求超时了，稍后再试。";
  }
  return "重试没能开始，稍后再试。";
}
