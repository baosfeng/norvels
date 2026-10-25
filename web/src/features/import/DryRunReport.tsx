import type { DryRunReport } from "../../api/types";
import { EmptyPanel } from "../../components/StatePanel";

/**
 * 只预检的只读报告：四类计数 + 数量口径 + 样例表 + 错误清单。
 * 预检不写库、不建任务，所以这里没有任何写操作入口。
 */
export function DryRunReportPanel({ report, dir }: { report: DryRunReport; dir: string }) {
  const files = report.files;
  const errors = report.errors ?? [];

  return (
    <div className="dry-run" data-testid="dry-run-report">
      <div className="head">
        <h2>预检结果</h2>
        <span className="meta" style={{ wordBreak: "break-all" }}>
          {dir}
        </span>
      </div>
      <p className="meta">预检不写库、不建任务，也不会挡住正在跑的导入；判定口径与真正导入一致。</p>
      {report.truncated ? (
        <p className="meta" data-testid="dry-run-truncated">
          目录条目太多，只预检了前 200 个文件；真正导入不受这个上限限制。
        </p>
      ) : null}

      <div className="counters">
        <div>
          <strong className="num" data-testid="dry-run-will-import">
            {files.willImport}
          </strong>
          <span>将导入</span>
        </div>
        <div>
          <strong className="num" data-testid="dry-run-duplicate">
            {files.duplicate}
          </strong>
          <span>重复</span>
        </div>
        <div>
          <strong className="num" data-testid="dry-run-unsupported">
            {files.unsupported}
          </strong>
          <span>不支持</span>
        </div>
        <div>
          <strong className="num" data-testid="dry-run-failed">
            {files.failed}
          </strong>
          <span>解析失败</span>
        </div>
        <div>
          <strong className="num" data-testid="dry-run-total">
            {files.total}
          </strong>
          <span>共扫描</span>
        </div>
      </div>
      <p className="meta" data-testid="dry-run-books">
        预计新书 {report.books.new} 本 · 重复 {report.books.duplicate} 本 · 合集拆出 {report.books.split} 本
      </p>

      {files.total === 0 ? (
        <EmptyPanel
          title="这个路径下没找到可导入的文件"
          description="确认路径存在，扩展名是 txt / epub / zip；zip 里的条目也会被扫到。"
          testId="dry-run-empty"
        />
      ) : (
        <>
          {report.sample.length > 0 ? (
            <div className="import-errors">
              <table className="table" data-testid="dry-run-sample">
                <thead>
                  <tr>
                    <th>文件</th>
                    <th>书名</th>
                    <th className="num">章</th>
                    <th className="num">字数</th>
                    <th>结论</th>
                  </tr>
                </thead>
                <tbody>
                  {report.sample.map((item) => (
                    <tr key={item.source}>
                      <td style={{ wordBreak: "break-all" }}>{item.source}</td>
                      <td>{item.title || "—"}</td>
                      <td className="num">{item.chapters}</td>
                      <td className="num">{item.words}</td>
                      <td>{item.duplicate ? "重复，会跳过" : item.reason || "将导入"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <p className="meta">样例最多 20 条，按扫描顺序；完整结果以计数为准。</p>
            </div>
          ) : null}

          {errors.length > 0 ? (
            <div className="import-errors" data-testid="dry-run-errors">
              <h3>没能处理的文件</h3>
              <ul>
                {errors.map((item) => (
                  <li key={item.source}>
                    <code>{item.source}</code> · {item.reason}
                  </li>
                ))}
              </ul>
              <p className="meta">错误最多列 50 条。</p>
            </div>
          ) : null}
        </>
      )}
    </div>
  );
}
