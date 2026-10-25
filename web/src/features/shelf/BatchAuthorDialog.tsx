import { useState } from "react";
import { api } from "../../api/client";
import type { BookSummary } from "../../api/types";
import { parseAuthorList } from "../../lib/authors";

/**
 * 批量设作者：纯前端循环 PATCH /api/books/{id}（authors 字段），不发新请求类型。
 * 逐本更新并实时显示「已完成 x / N」；失败项列出来，可只重试失败的那几本。
 * 作者留空 = 清空作者（按契约 §4 下发空数组）。
 */
export function BatchAuthorDialog({
  books,
  onClose,
  onSaved,
}: {
  books: BookSummary[];
  onClose: () => void;
  onSaved: (count: number) => void;
}) {
  const total = books.length;
  const [authorText, setAuthorText] = useState(() => sharedAuthor(books));
  const [running, setRunning] = useState(false);
  const [succeeded, setSucceeded] = useState<Set<number>>(new Set());
  const [failedIds, setFailedIds] = useState<Set<number>>(new Set());
  const [errorText, setErrorText] = useState("");

  const authors = parseAuthorList(authorText);
  const failed = books.filter((book) => failedIds.has(book.id));
  const done = succeeded.size;

  async function run(targets: BookSummary[]) {
    if (running || targets.length === 0) return;
    setRunning(true);
    setErrorText("");
    setFailedIds(new Set());
    const failedNow: number[] = [];
    for (const book of targets) {
      try {
        // 契约：authors 是全量替换；空数组 = 清空作者
        await api.updateBook(book.id, { authors });
        setSucceeded((prev) => new Set(prev).add(book.id));
      } catch {
        failedNow.push(book.id);
      }
    }
    setFailedIds(new Set(failedNow));
    setRunning(false);
    if (failedNow.length === 0) onSaved(done + targets.length);
    else setErrorText(`${failedNow.length} 本没能更新，可以只重试这几本。`);
  }

  return (
    <div className="confirm-backdrop" data-testid="batch-author-dialog" onClick={onClose}>
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="batch-author-title"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="batch-author-title">批量设置作者</h2>
        <p className="meta">
          选中 {total} 本：{books.slice(0, 3).map((book) => book.title).join("、")}
          {total > 3 ? ` 等 ${total} 本` : ""}
        </p>
        <div className="field">
          <label htmlFor="batch-authors">作者（多位用「、」或逗号分隔）</label>
          <input
            className="input"
            id="batch-authors"
            value={authorText}
            placeholder="留空表示清空这些书的作者"
            disabled={running}
            onChange={(event) => setAuthorText(event.target.value)}
          />
        </div>
        <p className="meta" data-testid="batch-author-hint">
          {authors.length === 0
            ? "作者留空：确认后会把这批书的作者清空。"
            : `将把这批书的作者统一设为 ${authors.join("、")}。`}
        </p>

        {running || done > 0 || failed.length > 0 ? (
          <p className="meta" role="status" data-testid="batch-author-progress">
            已完成 <strong className="num">{done}</strong> / {total}
            {failed.length > 0 ? ` · ${failed.length} 本失败` : ""}
          </p>
        ) : null}

        {failed.length > 0 && !running ? (
          <div className="import-errors" data-testid="batch-author-failures">
            <table className="table">
              <thead>
                <tr>
                  <th>书</th>
                  <th>结果</th>
                </tr>
              </thead>
              <tbody>
                {failed.map((book) => (
                  <tr key={book.id}>
                    <td>{book.title}</td>
                    <td>没能更新</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <button className="btn" type="button" data-testid="batch-author-retry" onClick={() => void run(failed)}>
              重试失败的 {failed.length} 本
            </button>
          </div>
        ) : null}

        {errorText ? (
          <p className="meta" role="alert" data-testid="batch-author-error">
            {errorText}
          </p>
        ) : null}

        <div className="confirm-actions">
          <button
            className="btn btn-primary"
            type="button"
            data-testid="batch-author-confirm"
            disabled={running}
            onClick={() => void run(books.filter((book) => !succeeded.has(book.id)))}
          >
            {running ? "正在更新…" : authors.length === 0 ? `清空 ${total} 本作者` : `设为 ${authors.join("、")}`}
          </button>
          <button className="btn btn-ghost" type="button" onClick={onClose} disabled={running}>
            取消
          </button>
        </div>
      </div>
    </div>
  );
}

/** 选中的书作者都相同时预填，否则留空由用户重填，避免误覆盖。 */
function sharedAuthor(books: BookSummary[]): string {
  const names = books.map((book) => (book.authorName ?? "").trim());
  const first = names[0] ?? "";
  if (!first || names.some((name) => name !== first)) return "";
  return first;
}
