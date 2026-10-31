import { useRef, useState } from "react";
import { useCapabilities } from "../../api/hooks";
import { api } from "../../api/client";
import type { UploadRejected, UploadResult } from "../../api/types";
import { formatBytes } from "../../lib/format";
import {
  LIMITS_UNKNOWN_HINT,
  extLabel,
  innerExtLabel,
  uploadLimitsOf,
  type UploadLimits,
} from "./limits";
import { formatOf, rejectionHeadline, rejectionText, screenFiles, uploadErrorText } from "./uploadScreen";

/**
 * 文件上传区：选择或拖入受支持的图书文件，逐条报告接收与被拒的原因。
 * 四态由 phase + picked + result 决定：空 / 上传中 / 失败 / 有结果。
 *
 * 上限与格式白名单一律来自 capabilities（读不到时不做本地预检、交给服务端），
 * 本文件不再出现任何写死的数字或格式清单。
 */

type Rejection = UploadRejected;
type Phase = "idle" | "uploading" | "done" | "error";

/** 压缩包说明：包内条目种类从同一份扩展名派生（去掉 zip 本身）。 */
function zipHint(limits: UploadLimits): string {
  const inner = innerExtLabel(limits.extensions);
  return inner
    ? `压缩包会解包后逐条导入：包内 ${inner} 各成一条，结果按「包名.zip:内部路径」列出。`
    : "压缩包会解包后逐条导入：包内每个受支持的条目各成一条，结果按「包名.zip:内部路径」列出。";
}

/** 上传区顶部的说明：数字全部来自下发的上限，缺哪个就不提哪个。 */
function uploadMeta(limits: UploadLimits): string {
  const parts: string[] = [];
  if (limits.maxFiles !== null) parts.push(`一次最多 ${limits.maxFiles} 个`);
  if (limits.maxFileBytes !== null) parts.push(`单本 ${formatBytes(limits.maxFileBytes)} 以内`);
  if (limits.maxTotalBytes !== null) parts.push(`一批 ${formatBytes(limits.maxTotalBytes)} 以内`);
  if (parts.length === 0) return "上传大小与格式以服务端校验为准。";
  const formats = extLabel(limits.extensions);
  return formats ? `${parts.join("，")}；目前支持 ${formats}。` : `${parts.join("，")}。`;
}

export function UploadPanel({ onSubmitted, disabled = false }: { onSubmitted: () => void; disabled?: boolean }) {
  const caps = useCapabilities();
  const limits = uploadLimitsOf(caps.data);
  const [picked, setPicked] = useState<File[]>([]);
  const [note, setNote] = useState("");
  const [dragging, setDragging] = useState(false);
  const [force, setForce] = useState(false);
  const [update, setUpdate] = useState(false);
  const [phase, setPhase] = useState<Phase>("idle");
  const [result, setResult] = useState<UploadResult | null>(null);
  const [rejected, setRejected] = useState<Rejection[]>([]);
  const [errorText, setErrorText] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const screened = screenFiles(picked, limits);
  const uploading = phase === "uploading";
  const canSubmit = picked.length > 0 && !uploading && !disabled;
  const hasZip = picked.some((file) => formatOf(file.name) === "zip");

  function resetOutcome() {
    setResult(null);
    setRejected([]);
    setErrorText("");
    setPhase("idle");
  }

  function addFiles(incoming: FileList | null) {
    const list = incoming ? Array.from(incoming) : [];
    if (list.length === 0) return;
    const merged = [...picked, ...list];
    const cap = limits.maxFiles;
    setPicked(cap === null ? merged : merged.slice(0, cap));
    setNote(
      cap !== null && merged.length > cap
        ? `一次最多 ${cap} 个文件，多出的 ${merged.length - cap} 个没有加进来`
        : "",
    );
    resetOutcome();
  }

  function clear() {
    setPicked([]);
    setNote("");
    resetOutcome();
  }

  async function submit() {
    if (!canSubmit) return;
    if (screened.accepted.length === 0) {
      setRejected(screened.rejected);
      setErrorText(rejectionHeadline(screened.rejected, limits));
      setPhase("error");
      return;
    }
    setPhase("uploading");
    setResult(null);
    setRejected([]);
    setErrorText("");
    try {
      const data = await api.uploadImport(screened.accepted, { force, update });
      setResult(data);
      setRejected([...screened.rejected, ...(data.rejected ?? [])]);
      setPhase("done");
      setPicked([]);
      setNote("");
      onSubmitted();
    } catch (err) {
      setRejected(screened.rejected);
      setErrorText(uploadErrorText(err, limits));
      setPhase("error");
    }
  }

  return (
    <div className="import-form" data-testid="upload-panel">
      <h2>上传文件</h2>
      <p className="meta">{uploadMeta(limits)}</p>
      {caps.error ? (
        <p className="inline-note" data-testid="upload-limits-unknown">
          {LIMITS_UNKNOWN_HINT}
        </p>
      ) : null}
      <DropZone
        dragging={dragging}
        disabled={uploading || disabled}
        formats={extLabel(limits.extensions)}
        onPick={() => inputRef.current?.click()}
        onDragState={setDragging}
        onDrop={addFiles}
      />
      <input
        ref={inputRef}
        type="file"
        hidden
        multiple
        accept={limits.extensions.length > 0 ? limits.extensions.map((ext) => `.${ext}`).join(",") : undefined}
        data-testid="upload-input"
        onChange={(event) => {
          addFiles(event.target.files);
          event.target.value = "";
        }}
      />
      {picked.length > 0 ? (
        <PickedTable files={picked} />
      ) : (
        <p className="meta" data-testid="upload-empty">
          还没有选文件。拖进来，或者点上面的区域挑文件。
        </p>
      )}
      {hasZip ? (
        <p className="meta" data-testid="upload-zip-hint">
          {zipHint(limits)}
        </p>
      ) : null}
      {note ? (
        <p className="meta" role="status">
          {note}
        </p>
      ) : null}
      <div className="check-row">
        <input id="upload-force" type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
        <label htmlFor="upload-force">已入库的书也重新导入</label>
      </div>
      <div className="check-row">
        <input id="upload-update" type="checkbox" checked={update} onChange={(e) => setUpdate(e.target.checked)} />
        <label htmlFor="upload-update">追更（只追加新章节）</label>
      </div>
      {update ? (
        <p className="meta" data-testid="upload-update-hint">
          同一本书再次导入时只追加新章节，已有章节与阅读进度保留；章节对不上时后端会拒绝，不会覆盖。
        </p>
      ) : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <button
          className="btn btn-primary"
          type="button"
          onClick={() => void submit()}
          disabled={!canSubmit}
          data-testid="upload-submit"
          data-od-id="C-09"
        >
          {uploading ? "正在上传…" : picked.length > 0 ? `上传文件（${screened.accepted.length}）` : "上传文件"}
        </button>
        {picked.length > 0 || result ? (
          <button className="btn btn-ghost" type="button" onClick={clear} disabled={uploading}>
            清空
          </button>
        ) : null}
      </div>
      {disabled ? (
        <p className="inline-note" data-testid="upload-reason">
          已经有一个导入任务在跑，等它结束再上传。
        </p>
      ) : picked.length === 0 ? (
        <p className="inline-note" data-testid="upload-reason">
          先选文件，才能上传。
        </p>
      ) : null}
      {uploading ? (
        <p className="meta" role="status" data-testid="upload-loading">
          正在上传，文件多的时候会久一点，先别关掉页面。
        </p>
      ) : null}
      {phase === "error" ? (
        <p className="meta" role="alert" data-testid="upload-error" style={{ color: "var(--danger, #b3261e)" }}>
          {errorText}
        </p>
      ) : null}
      {result ? <ResultSummary result={result} /> : null}
      {rejected.length > 0 ? <RejectedTable items={rejected} limits={limits} testId="upload-rejected" /> : null}
    </div>
  );
}

function DropZone({
  dragging,
  disabled,
  formats,
  onPick,
  onDragState,
  onDrop,
}: {
  dragging: boolean;
  disabled: boolean;
  formats: string;
  onPick: () => void;
  onDragState: (value: boolean) => void;
  onDrop: (files: FileList | null) => void;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      aria-label="选择或拖入文件"
      data-testid="upload-dropzone"
      onClick={onPick}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onPick();
        }
      }}
      onDragOver={(event) => {
        event.preventDefault();
        onDragState(true);
      }}
      onDragLeave={() => onDragState(false)}
      onDrop={(event) => {
        event.preventDefault();
        onDragState(false);
        onDrop(event.dataTransfer?.files ?? null);
      }}
      className={`upload-dropzone${dragging ? " is-dragging" : ""}${disabled ? " is-disabled" : ""}`}
      data-od-id="C-08"
      aria-disabled={disabled}
    >
      <strong style={{ fontSize: 13 }}>拖文件到这里，或点击选择</strong>
      <span className="meta">{formats ? `支持 ${formats}，可一次选多个` : "支持的格式与大小以服务端为准"}</span>
    </div>
  );
}

function PickedTable({ files }: { files: File[] }) {
  return (
    <table className="table" data-testid="upload-picked">
      <thead>
        <tr>
          <th>文件</th>
          <th>大小</th>
        </tr>
      </thead>
      <tbody>
        {files.map((file) => (
          <tr key={`${file.name}:${file.size}:${file.lastModified}`}>
            <td style={{ wordBreak: "break-all" }}>{file.name}</td>
            <td>{formatBytes(file.size)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/**
 * 上传结果：逐个列已接收条目。压缩包里的条目 name 是「包名.zip:内部路径」，
 * 原样列出即可区分它来自哪个包、包内哪一层。
 */
function ResultSummary({ result }: { result: UploadResult }) {
  const accepted = result.accepted ?? [];
  return (
    <div data-testid="upload-accepted" role="status" style={{ display: "grid", gap: 4 }}>
      <p className="meta">已提交 {accepted.length} 个文件，任务 #{result.jobId} 已排队，进度看右侧进度卡。</p>
      {accepted.length > 0 ? (
        <table className="table" data-testid="upload-accepted-table">
          <thead>
            <tr>
              <th>已接收</th>
              <th>格式</th>
            </tr>
          </thead>
          <tbody>
            {accepted.map((item, index) => (
              <tr key={`${item.name}:${index}`}>
                <td style={{ wordBreak: "break-all" }}>{item.name}</td>
                <td>{item.format}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
}

function RejectedTable({ items, limits, testId }: { items: Rejection[]; limits: UploadLimits; testId: string }) {
  return (
    <div className="import-errors" data-testid={testId} role="alert" style={{ padding: 0 }}>
      <table className="table">
        <thead>
          <tr>
            <th>文件</th>
            <th>没导入的原因</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item, index) => (
            <tr key={`${item.name}:${item.code}:${index}`}>
              <td style={{ wordBreak: "break-all" }}>{item.name}</td>
              <td>{rejectionText(item, limits)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
