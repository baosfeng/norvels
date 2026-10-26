import type { ImportJob } from "../../api/types";
import { formatDate } from "../../lib/format";

/** 任务历史：只列已结束的任务，当前任务由进度卡独占（避免同一单在两处各说一套数字）。 */
export function ImportHistory({ jobs }: { jobs: ImportJob[] }) {
  return (
    <div className="import-jobs">
      <div className="head">
        <h2>任务历史</h2>
        <span className="meta">已结束的任务 · 当前任务见上方进度卡</span>
      </div>
      {jobs.length === 0 ? (
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
            {jobs.map((job) => (
              <tr key={job.id}>
                <td>#{job.id}</td>
                <td>{job.sourceDir}</td>
                <td className="num">{job.imported}</td>
                <td className="num">{job.skipped}</td>
                <td className="num">{job.failed}</td>
                <td>
                  <span className={`pill${job.status === "failed" || job.status === "completed_with_errors" || job.failed > 0 ? " is-warn" : ""}`}>
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
  );
}

/** 部分失败是「完成了但有失败文件」，不能显示成排队中。 */
export function statusLabel(status: string, failed: number): string {
  if (status === "completed_with_errors") return "完成（部分失败）";
  if (status === "completed") return failed > 0 ? "部分未成功" : "已完成";
  if (status === "failed") return "中断";
  if (status === "canceled") return "已取消";
  return "排队中";
}
