import { useState } from "react";
import { ApiError, api } from "../../api/client";
import type { BookMetaPatch, BookSummary, SerialStatus } from "../../api/types";
import { parseAuthors, parseTags } from "../../lib/metaDraft";

export type BatchKind = "author" | "tag" | "serial" | "delete";

/** 批量改元信息一次最多 200 本（后端上限）：翻页累计的选择也不会撞上，但先挡一道。 */
const MAX_BATCH = 200;

/** 只用到 id 与 title 定位一本；失败重试时也从这里还原。 */
type Target = Pick<BookSummary, "id" | "title">;

type Failure = { id: number; title: string; message: string };

/**
 * 批量操作弹层：设作者 / 设标签 / 设连载状态 / 删除。
 *
 * 元信息三种改法都走 POST /api/books/batch **一次请求**：单事务逐本应用同一 patch，
 * 单本失败只进 failed（带后端人话），不整体回滚；失败清单可以逐条或整体重试。
 * 删除仍逐本 DELETE：批量端点不覆盖删除，删书本来就该一本一条。
 */
export function AdminBatchDialog({
  kind,
  books,
  onClose,
  onDone,
}: {
  kind: BatchKind;
  books: BookSummary[];
  onClose: () => void;
  onDone: (message: string) => void;
}) {
  const [authorText, setAuthorText] = useState("");
  const [tagText, setTagText] = useState("");
  const [serial, setSerial] = useState<Exclude<SerialStatus, "">>("ongoing");
  const [running, setRunning] = useState(false);
  /** 已成功改完的本数（含上一轮重试成功的） */
  const [done, setDone] = useState(0);
  const [failed, setFailed] = useState<Failure[]>([]);
  const [errorText, setErrorText] = useState("");

  const values = { authors: parseAuthors(authorText), tags: parseTags(tagText), serial };

  async function apply(targets: Target[], retry = false) {
    if (running || targets.length === 0) return;
    if (targets.length > MAX_BATCH) {
      setErrorText(`一次最多 ${MAX_BATCH} 本，分批再改。`);
      return;
    }
    // 作者清空在契约里是 400（至少一个作者）：与其让后端逐本报错，不如在本地先说清
    if (kind === "author" && values.authors.length === 0) {
      setErrorText("作者不能为空，至少填一个。");
      return;
    }
    setRunning(true);
    setErrorText("");
    if (!retry) setFailed([]);
    try {
      const { ok, failures } = kind === "delete" ? await removeAll(targets) : await patchAll(kind, targets, values);
      setDone((prev) => (retry ? prev + ok : ok));
      setFailed((prev) => mergeFailures(retry ? prev : [], targets, failures));
      if (failures.length === 0) onDone(doneMessage(kind, targets.length, values));
    } catch (error) {
      setErrorText(requestErrorText(error));
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="confirm-backdrop" role="presentation" onClick={() => !running && onClose()}>
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="batch-title"
        data-testid="admin-batch-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="batch-title">{titleOf(kind, books.length)}</h2>
        <p className="meta">
          已选 {books.length} 本：{books.slice(0, 3).map((book) => book.title).join("、")}
          {books.length > 3 ? ` 等 ${books.length} 本` : ""}
        </p>

        {kind === "author" ? (
          <div className="field">
            <label htmlFor="batch-author">作者（多位用「、」或逗号分隔）</label>
            <input
              className="input"
              id="batch-author"
              value={authorText}
              disabled={running}
              placeholder="至少填一个作者"
              data-testid="admin-batch-input"
              onChange={(event) => setAuthorText(event.target.value)}
            />
          </div>
        ) : null}

        {kind === "tag" ? (
          <div className="field">
            <label htmlFor="batch-tag">标签（会替换这些书现有的标签）</label>
            <input
              className="input"
              id="batch-tag"
              value={tagText}
              disabled={running}
              placeholder="例如 武侠, 群像；留空表示清空标签"
              data-testid="admin-batch-input"
              onChange={(event) => setTagText(event.target.value)}
            />
          </div>
        ) : null}

        {kind === "serial" ? (
          <div className="field">
            <label htmlFor="batch-serial">连载状态</label>
            <select
              className="select"
              id="batch-serial"
              value={serial}
              disabled={running}
              data-testid="admin-batch-serial-input"
              onChange={(event) => setSerial(event.target.value as Exclude<SerialStatus, "">)}
            >
              <option value="ongoing">连载中</option>
              <option value="completed">已完结</option>
            </select>
          </div>
        ) : null}

        {kind === "delete" ? (
          <p className="meta" data-testid="admin-batch-warning">
            这 {books.length} 本书的正文、封面与阅读进度会一起删除，无法恢复。
          </p>
        ) : null}

        {errorText ? (
          <p className="meta" role="alert" data-testid="admin-batch-error">
            {errorText}
          </p>
        ) : null}

        {running || done > 0 || failed.length > 0 ? (
          <p className="meta" role="status" data-testid="admin-batch-progress">
            已完成{" "}
            <strong className="num" data-testid="admin-batch-done">
              {done}
            </strong>{" "}
            / {books.length}
            {failed.length > 0 ? ` · ${failed.length} 本失败` : ""}
          </p>
        ) : null}

        {failed.length > 0 && !running ? (
          <div data-testid="admin-batch-failures">
            <ul className="rule-list">
              {failed.map((item) => (
                <li className="rule-row" key={item.id} data-testid={`admin-batch-failed-${item.id}`}>
                  <span>{item.title}</span>
                  <span className="meta">{item.message}</span>
                  <button
                    className="btn btn-sm"
                    type="button"
                    data-testid={`admin-batch-retry-${item.id}`}
                    onClick={() => void apply([{ id: item.id, title: item.title }], true)}
                  >
                    只重试这本
                  </button>
                </li>
              ))}
            </ul>
            <button className="btn btn-sm" type="button" data-testid="admin-batch-retry" onClick={() => void apply(failed, true)}>
              重试失败的 {failed.length} 本
            </button>
          </div>
        ) : null}

        <div className="confirm-actions">
          <button
            className={`btn ${kind === "delete" ? "btn-danger" : "btn-primary"}`}
            type="button"
            disabled={running}
            data-testid="admin-batch-confirm"
            onClick={() => void apply(books)}
          >
            {running ? "正在处理…" : confirmLabel(kind, books.length)}
          </button>
          <button className="btn btn-ghost" type="button" disabled={running} onClick={onClose}>
            取消
          </button>
        </div>
      </div>
    </div>
  );
}

/** 元信息：一次 batch 请求，失败项原样带回来（不整体回滚）。 */
async function patchAll(
  kind: BatchKind,
  targets: Target[],
  values: { authors: string[]; tags: string[]; serial: Exclude<SerialStatus, ""> },
): Promise<{ ok: number; failures: Failure[] }> {
  const result = await api.batchUpdateBooks({ ids: targets.map((target) => target.id), patch: patchOf(kind, values) });
  const titles = new Map(targets.map((target) => [target.id, target.title]));
  const failures = result.failed.map((item) => ({
    id: item.id,
    title: titles.get(item.id) ?? `#${item.id}`,
    message: item.message || "没能更新",
  }));
  return { ok: result.updated.length, failures };
}

/** 删除：逐本 DELETE，失败的那几本单独列出来可重试。 */
async function removeAll(targets: Target[]): Promise<{ ok: number; failures: Failure[] }> {
  const failures: Failure[] = [];
  let ok = 0;
  for (const target of targets) {
    try {
      await api.deleteBook(target.id);
      ok += 1;
    } catch (error) {
      failures.push({ id: target.id, title: target.title, message: deleteErrorText(error) });
    }
  }
  return { ok, failures };
}

/** 按 kind 组装 patch：三种改法共用一个端点，patch 是唯一区别。 */
function patchOf(
  kind: BatchKind,
  values: { authors: string[]; tags: string[]; serial: Exclude<SerialStatus, ""> },
): BookMetaPatch {
  if (kind === "author") return { authors: values.authors };
  if (kind === "tag") return { tags: values.tags };
  return { serialStatus: values.serial };
}

/** 重试一轮后：从旧失败清单里去掉这一轮试过的，再并上这一轮的失败。 */
function mergeFailures(prev: Failure[], attempted: Target[], next: Failure[]): Failure[] {
  const tried = new Set(attempted.map((target) => target.id));
  return [...prev.filter((item) => !tried.has(item.id)), ...next];
}

function titleOf(kind: BatchKind, count: number): string {
  switch (kind) {
    case "author":
      return `批量设置作者（${count} 本）`;
    case "tag":
      return `批量设置标签（${count} 本）`;
    case "serial":
      return `批量设置连载状态（${count} 本）`;
    default:
      return `删除这 ${count} 本书？`;
  }
}

function confirmLabel(kind: BatchKind, count: number): string {
  return kind === "delete" ? `永久删除 ${count} 本` : `应用到 ${count} 本`;
}

function doneMessage(
  kind: BatchKind,
  count: number,
  values: { authors: string[]; tags: string[]; serial: Exclude<SerialStatus, ""> },
): string {
  switch (kind) {
    case "author":
      return `${count} 本的作者已改为「${values.authors.join("、")}」。`;
    case "tag":
      return values.tags.length === 0
        ? `已清空 ${count} 本的标签。`
        : `${count} 本的标签已改为 ${values.tags.join("、")}。`;
    case "serial":
      return `${count} 本的连载状态已设为${values.serial === "ongoing" ? "连载中" : "已完结"}。`;
    default:
      return `已删除 ${count} 本书。`;
  }
}

/** 请求级失败（整批没发出去）：参数 / 超时 / 网络分开说。 */
function requestErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return error.message || "有些字段不符合要求，检查一下再试。";
    if (error.status === 404) return "有书刚被删掉了，关掉弹窗刷新列表再试。";
    if (error.kind === "timeout") return "批量更新超时了：已改好的书不会重复改，稍后再试一次。";
  }
  return "批量更新没能完成，稍后再试。";
}

/** 删书失败：404 是已经不在了（当成功处理更安全，这里仍如实列出）。 */
function deleteErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "已经不在了，刷新列表即可";
    if (error.kind === "timeout") return "删除超时，稍后再试";
  }
  return "没能删除，稍后重试";
}
