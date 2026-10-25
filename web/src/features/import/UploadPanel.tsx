import { useRef, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { UploadRejected, UploadResult } from "../../api/types";
import { formatBytes } from "../../lib/format";

/** 与后端契约一致的上限：选完文件就挡掉，不让用户等一次注定失败的上传。 */
const MAX_FILES = 20;
const MAX_FILE_BYTES = 64 * 1024 * 1024;
const MAX_TOTAL_BYTES = 256 * 1024 * 1024;
const SUPPORTED_FORMATS = ["txt", "epub", "zip"] as const;

/** 选中压缩包时的说明：包内条目逐条导入，结果名带「包名.zip:内部路径」前缀。 */
const ZIP_HINT = "压缩包会解包后逐条导入：包内 txt/epub 各成一条，结果按「包名.zip:内部路径」列出。";

type Rejection = UploadRejected;
type Phase = "idle" | "uploading" | "done" | "error";

/** 扩展名小写化（含点前的文件名无所谓，只看最后一个点之后）。 */
function formatOf(name: string): string {
  const dot = name.lastIndexOf(".");
  return dot < 0 ? "" : name.slice(dot + 1).toLowerCase();
}

/**
 * 本地预检：不支持的格式与超限文件逐个说明，同批合法文件继续上传。
 * 后端仍会再校验一次（它才是权威），这里只是让失败更早、更好懂。
 */
function screenFiles(files: File[]): { accepted: File[]; rejected: Rejection[] } {
  const accepted: File[] = [];
  const rejected: Rejection[] = [];
  let total = 0;
  for (const file of files) {
    const format = formatOf(file.name);
    if (!(SUPPORTED_FORMATS as readonly string[]).includes(format)) {
      rejected.push({
        name: file.name,
        code: "unsupported_format",
        message: `暂不支持 ${format ? `.${format}` : "这个文件"}，目前支持 TXT、EPUB、ZIP`,
      });
      continue;
    }
    if (file.size > MAX_FILE_BYTES) {
      rejected.push({ name: file.name, code: "payload_too_large", message: `「${file.name}」超过 64 MB 上限` });
      continue;
    }
    total += file.size;
    if (total > MAX_TOTAL_BYTES) {
      rejected.push({ name: file.name, code: "payload_too_large", message: "这一批文件超过 256 MB 上限，分批再传" });
      continue;
    }
    accepted.push(file);
  }
  return { accepted, rejected };
}

/** 拒绝原因的人话：后端给了 message 就用它，没给按 code 兜底。 */
function rejectionText(item: Rejection): string {
  if (item.message) return item.message;
  switch (item.code) {
    case "unsupported_format":
      return "暂不支持这个文件，目前支持 TXT、EPUB、ZIP";
    case "invalid_zip":
      return "这个压缩包打不开，确认是标准 zip 后再试";
    case "empty_zip":
      return "压缩包里没有可导入的 txt 或 epub";
    case "zip_too_large":
      return "压缩包内容超过上限（最多 1000 条、解压后 512 MB）";
    case "payload_too_large":
      return "文件超过 64 MB 上限";
    case "too_many_files":
      return "一次最多 20 个文件";
    case "encrypted_epub":
      return "EPUB 已加密，暂时无法导入";
    default:
      return "这个文件没能加入导入任务，换个文件再试";
  }
}

/** 整次上传失败的文案：按状态码与 code 分支，不把后端术语直接甩给用户。 */
function uploadErrorText(error: unknown): string {
  if (!(error instanceof ApiError)) return "上传没能完成，稍后再试";
  if (error.kind === "timeout") return "上传超时了，检查网络后再试";
  if (error.status === 409) return "已经有一个导入任务在跑，等它结束再上传";
  if (error.status === 413 || error.code === "payload_too_large") return "文件太大：单本 64 MB、一批 256 MB 以内";
  if (error.code === "too_many_files") return "一次最多 20 个文件";
  if (error.code === "encrypted_epub") return "EPUB 已加密，暂时无法导入";
  if (error.code === "invalid_zip") return "压缩包打不开，确认是标准 zip 后再试";
  if (error.code === "empty_zip") return "压缩包里没有可导入的 txt 或 epub";
  if (error.code === "zip_too_large") return "压缩包内容超过上限（最多 1000 条、解压后 512 MB）";
  if (error.code === "unsupported_format") return "这批文件都不支持，目前支持 TXT、EPUB、ZIP";
  return "上传没能完成，稍后再试";
}

/**
 * 文件上传区：选择或拖入 txt / epub / zip，逐条报告接收与被拒的原因。
 * 四态由 phase + picked + result 决定：空 / 上传中 / 失败 / 有结果。
 */
export function UploadPanel({ onSubmitted, disabled = false }: { onSubmitted: () => void; disabled?: boolean }) {
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

  const screened = screenFiles(picked);
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
    setPicked(merged.slice(0, MAX_FILES));
    setNote(merged.length > MAX_FILES ? `一次最多 20 个文件，多出的 ${merged.length - MAX_FILES} 个没有加进来` : "");
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
      setErrorText("这批文件都不支持，目前支持 TXT、EPUB、ZIP");
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
      setErrorText(uploadErrorText(err));
      setPhase("error");
    }
  }

  return (
    <div className="import-form" data-testid="upload-panel">
      <h2>上传文件</h2>
      <p className="meta">一次最多 20 个，单本 64 MB 以内，一批 256 MB 以内；目前支持 TXT、EPUB 与 ZIP 压缩包。</p>
      <DropZone
        dragging={dragging}
        disabled={uploading || disabled}
        onPick={() => inputRef.current?.click()}
        onDragState={setDragging}
        onDrop={addFiles}
      />
      <input
        ref={inputRef}
        type="file"
        hidden
        multiple
        accept=".txt,.epub,.zip,text/plain,application/epub+zip,application/zip"
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
          {ZIP_HINT}
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
        <button className="btn btn-primary" type="button" onClick={submit} disabled={!canSubmit} data-testid="upload-submit">
          {uploading ? "正在上传…" : picked.length > 0 ? `上传文件（${screened.accepted.length}）` : "上传文件"}
        </button>
        {picked.length > 0 || result ? (
          <button className="btn btn-ghost" type="button" onClick={clear} disabled={uploading}>
            清空
          </button>
        ) : null}
      </div>
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
      {rejected.length > 0 ? <RejectedTable items={rejected} testId="upload-rejected" /> : null}
    </div>
  );
}

function DropZone({
  dragging,
  disabled,
  onPick,
  onDragState,
  onDrop,
}: {
  dragging: boolean;
  disabled: boolean;
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
      style={{
        display: "grid",
        placeItems: "center",
        gap: 4,
        padding: "22px 16px",
        border: `1.5px dashed ${dragging ? "var(--accent, #c26b3f)" : "var(--border)"}`,
        borderRadius: "var(--radius)",
        background: dragging ? "var(--surface-warm)" : "transparent",
        cursor: disabled ? "not-allowed" : "pointer",
        textAlign: "center",
      }}
    >
      <strong style={{ fontSize: 13 }}>拖文件到这里，或点击选择</strong>
      <span className="meta">支持 TXT、EPUB、ZIP，可一次选多个</span>
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

function RejectedTable({ items, testId }: { items: Rejection[]; testId: string }) {
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
              <td>{rejectionText(item)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}