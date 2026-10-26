import { Link } from "react-router-dom";
import type { ImportJob } from "../../api/types";
import { formatCount } from "../../lib/format";

/**
 * 导入成功提示：任务结束后给一个去书籍管理的出口。
 * 全部失败时不给出口（点过去也看不到新书），改成让人去看失败明细。
 */
export function ImportSuccessPanel({ job }: { job: ImportJob }) {
  const imported = job.imported ?? 0;
  if (imported === 0) {
    if (job.failed === 0) return null;
    return (
      <div className="import-form" data-testid="import-success-empty">
        <h2>任务 #{job.id} 结束了</h2>
        <p className="meta">这次没有书入库，检查失败明细里的原因。</p>
      </div>
    );
  }
  return (
    <div className="import-form" data-testid="import-success">
      <h2>任务 #{job.id} 结束了</h2>
      <p className="meta">
        这次入库 {formatCount(imported)} 本书{job.failed > 0 ? `，另有 ${formatCount(job.failed)} 个文件没成功` : ""}。
      </p>
      <Link className="btn" to="/admin" data-od-id="C-25">
        去书籍管理
      </Link>
    </div>
  );
}
