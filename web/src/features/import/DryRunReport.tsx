import { useState } from "react";
import type { DryRunReport } from "../../api/types";
import { EmptyPanel } from "../../components/StatePanel";

/**
 * 只预检的只读报告：计数 + 样例表 + 告警折叠条。
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

      <WarningFold items={errors} />

      {files.total === 0 ? (
        <EmptyPanel
          title="这个路径下没找到可导入的文件"
          description="确认路径存在，扩展名是 txt / epub / zip；zip 里的条目也会被扫到。"
          testId="dry-run-empty"
        />
      ) : report.sample.length > 0 ? (
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
    </div>
  );
}

/** 告警明细的条数上限：超过就在列表内滚动，别把整页撑长（契约 C-20 边界）。 */
const WARN_SCROLL_AT = 20;

/**
 * 预检告警折叠条：没有告警时整条不出现（不占版面、也不留空壳）。
 * 展开/收起只影响展示，告警计数与页面滚动位置都不变。
 */
function WarningFold({ items }: { items: { source: string; reason: string }[] }) {
  const [open, setOpen] = useState(false);
  if (items.length === 0) return null;
  return (
    <div className="warn-fold" data-testid="dry-run-warnings">
      <button
        className="failure-toggle"
        type="button"
        aria-expanded={open}
        data-od-id="C-20"
        onClick={() => setOpen((value) => !value)}
      >
        <span>{items.length} 条告警</span>
        <span>{open ? "收起" : "查看"}</span>
      </button>
      {open ? (
        <ul
          className={`warn-list${items.length > WARN_SCROLL_AT ? " is-scroll" : ""}`}
          data-testid="dry-run-errors"
        >
          {items.map((item) => (
            <li key={item.source}>
              <code>{item.source}</code> · {item.reason}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
