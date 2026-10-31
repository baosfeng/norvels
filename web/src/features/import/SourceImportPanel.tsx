import { Fragment, useRef, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { SourceImportIssue, SourceImportResult } from "../../api/types";
import {
  applicableCount,
  countIssues,
  countsSummary,
  statusMeta,
  typeLabel,
  type ImportCounts,
} from "./sourceImport";
import { readFileText } from "./sourceProfiles";

/** 文件上限：后端请求体 2 MiB，这里按任务口径只收 1 MiB 的可读文本。 */
const MAX_IMPORT_FILE_BYTES = 1 << 20;

/** 输入区说明：四种形态都只是「把字符串放进 raw」，取回与识别都在后端。 */
export const IMPORT_HINT =
  "粘贴 Legado 书源 JSON，或放分享链接（legado:// / yuedu:// / http(s)）——链接由后端取回，前端只提交原始文本。";

/** 书源导入区：粘贴/选文件 → 预览 → 应用，全程在同一张卡片里展开。 */
export function SourceImportPanel({ onImported }: { onImported: () => void }) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [raw, setRaw] = useState("");
  const [sourceName, setSourceName] = useState("");
  const [busy, setBusy] = useState<"" | "preview" | "apply">("");
  const [result, setResult] = useState<SourceImportResult | null>(null);
  const [errorText, setErrorText] = useState("");
  const [note, setNote] = useState("");

  const counts = result ? countIssues(result.results) : null;
  const applicable = result ? applicableCount(result.results) : 0;

  function reset() {
    setResult(null);
    setErrorText("");
    setNote("");
  }

  async function pickFile(file: File | undefined) {
    if (!file) return;
    if (file.size > MAX_IMPORT_FILE_BYTES) {
      setErrorText("文件超过 1 MB，改用粘贴内容或链接吧（太大的 JSON 不好在页面里检查）。");
      return;
    }
    try {
      setRaw(await readFileText(file));
      setSourceName(file.name);
      reset();
    } catch {
      setErrorText("这个文件读不出来，确认是 UTF-8 编码的 JSON 再试。");
    }
  }

  /** 明显是 JSON 文本时先在本地判一下语法：坏 JSON 不发请求，行内说明。 */
  function jsonSyntaxProblem(text: string): string {
    const trimmed = text.trim();
    if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return "";
    try {
      JSON.parse(trimmed);
      return "";
    } catch {
      return "这不是合法 JSON：检查一下括号、引号与逗号。";
    }
  }

  async function preview() {
    if (busy) return;
    const text = raw.trim();
    if (!text) {
      setErrorText("先粘贴 JSON、选择文件，或放一个分享链接。");
      return;
    }
    const syntax = jsonSyntaxProblem(text);
    if (syntax) {
      setErrorText(syntax);
      setResult(null);
      return;
    }
    setBusy("preview");
    setErrorText("");
    setNote("");
    try {
      setResult(await api.importSourceProfiles(text, false));
    } catch (err) {
      setResult(null);
      setErrorText(errorMessage(err, "预览失败"));
    } finally {
      setBusy("");
    }
  }

  async function apply() {
    if (busy || applicable === 0) return;
    setBusy("apply");
    setErrorText("");
    try {
      const applied = await api.importSourceProfiles(raw.trim(), true);
      setResult(applied);
      setNote(appliedNote(applied));
      onImported();
    } catch (err) {
      setErrorText(errorMessage(err, "导入失败"));
    } finally {
      setBusy("");
    }
  }

  return (
    <div
      data-testid="source-import-panel"
      style={{
        display: "grid",
        gap: "var(--gap-card)",
        padding: "var(--gap-card)",
        border: "1px dashed var(--border)",
        borderRadius: "var(--radius)",
      }}
    >
      <p className="meta">{IMPORT_HINT}</p>
      <div className="field">
        <label htmlFor="source-import-raw">书源 JSON 或链接</label>
        <textarea
          className="input is-mono"
          id="source-import-raw"
          rows={6}
          value={raw}
          spellCheck={false}
          placeholder={'{\n  "bookSourceUrl": "https://example.invalid/api",\n  "bookSourceName": "示例书源"\n}'}
          data-testid="source-import-raw"
          onChange={(event) => {
            setRaw(event.target.value);
            setSourceName("");
            reset();
          }}
        />
      </div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <button className="btn" type="button" onClick={() => fileRef.current?.click()}>
          选择 .json 文件
        </button>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          hidden
          data-testid="source-import-file"
          onChange={(event) => {
            void pickFile(event.target.files?.[0]);
            event.target.value = "";
          }}
        />
        <button className="btn" type="button" data-testid="source-import-preview" disabled={busy !== ""} onClick={() => void preview()}>
          {busy === "preview" ? "正在预览…" : "预览"}
        </button>
        {sourceName ? (
          <span className="meta" data-testid="source-import-file-name">
            来自文件：{sourceName}
          </span>
        ) : null}
      </div>
      {errorText ? (
        <p className="inline-note is-error" role="alert" data-testid="source-import-error">
          {errorText}
        </p>
      ) : null}
      {note ? (
        <p className="meta" role="status" data-testid="source-import-note">
          {note}
        </p>
      ) : null}

      {result && counts ? (
        <div style={{ display: "grid", gap: "var(--gap-card)" }}>
          <CountSummary counts={counts} via={result.via} />
          <ImportResultTable results={result.results} />
          {result.skipped && result.skipped.length > 0 ? <SkippedBlock items={result.skipped} /> : null}
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            <button
              className="btn btn-primary"
              type="button"
              data-testid="source-import-apply"
              disabled={busy !== "" || applicable === 0}
              onClick={() => void apply()}
            >
              {busy === "apply" ? "正在导入…" : `导入 ${applicable} 条可用的`}
            </button>
            {applicable === 0 ? (
              <span className="meta">没有可用条目，改好 JSON 再预览。</span>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

/** 计数摘要：可用 / 部分可用 / 不可用，另附后端判定的输入形态。 */
function CountSummary({ counts, via }: { counts: ImportCounts; via?: string }) {
  return (
    <p className="meta" data-testid="source-import-counts">
      {countsSummary(counts)}
      {via ? ` · 输入形态：${viaLabel(via)}` : ""}
      {counts.blocked > 0 ? " · 未通过的不进入应用" : ""}
    </p>
  );
}

/**
 * 预览结果表：主行只放「序号 / 名称 / 类型 / 状态」，降级字段、不支持字段与原因
 * 折到紧邻的明细行里跨列展示。
 *
 * 这么拆是因为导入区在左栏（420px）：七列硬排会把状态列压成一字一行。
 * 信息一条没少，宽度却可读——窄栏里「主行 + 明细行」比横向滚动更好用。
 */
function ImportResultTable({ results }: { results: SourceImportIssue[] }) {
  return (
    <div className="import-errors" style={{ padding: 0 }}>
      <table className="table" data-testid="source-import-results">
        <thead>
          <tr>
            <th className="num">#</th>
            <th>名称</th>
            <th>类型</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          {results.map((item) => (
            <Fragment key={item.index}>
              <tr data-testid="source-import-result-row">
                <td className="num">{item.index + 1}</td>
                <td>{item.name || item.id || "未命名"}</td>
                <td>{typeLabel(item.type)}</td>
                <td>
                  <StatusCell status={item.status} />
                </td>
              </tr>
              <tr data-testid="source-import-result-detail">
                <td />
                <td colSpan={3} className="meta">
                  {detailText(item)}
                </td>
              </tr>
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** 明细行文案：状态原因 + 降级字段 + 不支持字段，缺哪个就不提哪个。 */
function detailText(item: SourceImportIssue): string {
  const meta = statusMeta(item.status);
  const parts: string[] = [];
  parts.push(meta.hint || "—");
  if (item.degradedFields.length > 0) parts.push(`降级字段：${item.degradedFields.join("、")}`);
  if (item.unsupportedFields.length > 0) parts.push(`不支持字段：${item.unsupportedFields.join("、")}`);
  if (item.reason) parts.push(item.reason);
  return parts.join(" · ");
}

/** 状态单元格：只放颜色 + 文案，为什么在明细行里说。 */
function StatusCell({ status }: { status: string }) {
  const meta = statusMeta(status);
  return (
    <span style={{ color: meta.color, fontWeight: 600 }} data-testid="source-import-status">
      {meta.label}
    </span>
  );
}

/** 新语义下被跳过的条目（后端返回 skipped 时才出现）。 */
function SkippedBlock({ items }: { items: NonNullable<SourceImportResult["skipped"]> }) {
  return (
    <details data-testid="source-import-skipped">
      <summary className="meta">{items.length} 条不可用已跳过</summary>
      <ul>
        {items.map((item) => (
          <li key={item.index}>
            <code>{item.name || item.id || `#${item.index + 1}`}</code> · {statusMeta(item.status).label} ·{" "}
            {item.reason || "—"}
          </li>
        ))}
      </ul>
    </details>
  );
}

/** 输入形态的中文名。 */
function viaLabel(via: string): string {
  switch (via) {
    case "text":
      return "粘贴/文件文本";
    case "url":
      return "http(s) 链接取回";
    case "legado":
      return "legado:// 取回";
    case "yuedu":
      return "yuedu:// 取回";
    default:
      return via;
  }
}

/** 应用成功的一句话：导入 N 条、覆盖 M 条，新语义还带跳过数。 */
function appliedNote(result: SourceImportResult): string {
  const skipped = result.skipped?.length ?? 0;
  const tail = skipped > 0 ? `，跳过 ${skipped} 条不可用的` : "";
  return `已导入 ${result.applied} 条，覆盖 ${result.overwritten} 条${tail}。`;
}

/** 失败文案：后端 message 优先（带「第几条哪条错」），没给就按状态码说人话。 */
function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    if (error.message && error.kind === "http") return error.message;
    if (error.kind === "timeout") return "请求超时了，稍后再试。";
  }
  return `${fallback}，稍后再试。`;
}
