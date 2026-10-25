import { useState } from "react";
import { api } from "../../api/client";
import type { BookSummary, SerialStatus } from "../../api/types";
import { parseAuthors, parseTags } from "../../lib/metaDraft";

export type BatchKind = "author" | "tag" | "serial" | "delete";

/**
 * 批量操作弹层：设作者 / 设标签 / 设连载状态 / 删除。
 *
 * 实现方式是前端逐本下发既有的 PATCH / DELETE（本地库一本书一次请求，几十本也可接受）：
 * 不新增批量端点，就不新增一套「部分成功」的事务语义——失败的那几本会单独列出来可重试。
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
  const [done, setDone] = useState(0);
  const [failed, setFailed] = useState<BookSummary[]>([]);

  async function apply(targets: BookSummary[]) {
    if (running || targets.length === 0) return;
    setRunning(true);
    setFailed([]);
    const failures: BookSummary[] = [];
    let ok = 0;
    for (const book of targets) {
      try {
        await patchOne(kind, book.id, { authors: parseAuthors(authorText), tags: parseTags(tagText), serial });
        ok += 1;
        setDone((prev) => prev + 1);
      } catch {
        failures.push(book);
      }
    }
    setFailed(failures);
    setRunning(false);
    if (failures.length === 0) onDone(doneMessage(kind, ok, targets.length, { authorText, tagText, serial }));
    void ok;
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
              placeholder="留空表示清空这些书的作者"
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
              data-testid="admin-batch-serial"
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

        {running || done > 0 || failed.length > 0 ? (
          <p className="meta" role="status" data-testid="admin-batch-progress">
            已完成 <strong className="num">{done}</strong> / {books.length}
            {failed.length > 0 ? ` · ${failed.length} 本失败` : ""}
          </p>
        ) : null}

        {failed.length > 0 && !running ? (
          <div data-testid="admin-batch-failures">
            <ul className="rule-list">
              {failed.map((book) => (
                <li className="rule-row" key={book.id}>
                  {book.title}
                  <span className="meta">没能更新</span>
                </li>
              ))}
            </ul>
            <button className="btn btn-sm" type="button" data-testid="admin-batch-retry" onClick={() => void apply(failed)}>
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

/** 单本落地：按 kind 选字段（只有元信息走 PATCH，删除走 DELETE）。 */
async function patchOne(
  kind: BatchKind,
  bookId: number,
  values: { authors: string[]; tags: string[]; serial: Exclude<SerialStatus, ""> },
): Promise<void> {
  if (kind === "delete") {
    await api.deleteBook(bookId);
    return;
  }
  if (kind === "author") {
    await api.updateBook(bookId, { authors: values.authors });
    return;
  }
  if (kind === "tag") {
    await api.updateBook(bookId, { tags: values.tags });
    return;
  }
  await api.updateBook(bookId, { serialStatus: values.serial });
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
  switch (kind) {
    case "author":
      return `应用到 ${count} 本`;
    case "tag":
      return `应用到 ${count} 本`;
    case "serial":
      return `应用到 ${count} 本`;
    default:
      return `永久删除 ${count} 本`;
  }
}

function doneMessage(
  kind: BatchKind,
  ok: number,
  total: number,
  values: { authorText: string; tagText: string; serial: Exclude<SerialStatus, ""> },
): string {
  switch (kind) {
    case "author":
      return parseAuthors(values.authorText).length === 0
        ? `已清空 ${total} 本的作者。`
        : `${ok} 本的作者已更新。`;
    case "tag":
      return `${ok} 本的标签已更新。`;
    case "serial":
      return `${ok} 本的连载状态已设为${values.serial === "ongoing" ? "连载中" : "已完结"}。`;
    default:
      return `已删除 ${ok} 本书。`;
  }
}
