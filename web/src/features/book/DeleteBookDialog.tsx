import { useState } from "react";
import { ApiError, api } from "../../api/client";

/**
 * 删除书籍的二次确认弹层：必须把书名一字不差抄进输入框，确认按钮才可用。
 * 删除不可撤销（章节 / 封面 / 阅读进度一起没），所以不用「再点一次」这种容易误触的方式。
 */
export function DeleteBookDialog({
  bookId,
  title,
  onClose,
  onDeleted,
}: {
  bookId: number;
  title: string;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [errorText, setErrorText] = useState("");
  const matched = typed.trim() === title.trim() && title.trim() !== "";

  async function confirm() {
    if (busy || !matched) return;
    setBusy(true);
    setErrorText("");
    try {
      await api.deleteBook(bookId);
      onDeleted();
    } catch (err) {
      setErrorText(deleteErrorText(err));
      setBusy(false);
    }
  }

  return (
    <div
      className="confirm-backdrop"
      role="presentation"
      onClick={() => {
        if (!busy) onClose();
      }}
    >
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-title"
        data-testid="delete-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="delete-title">删除《{title}》</h2>
        <p className="meta">
          这本书的章节、封面与阅读进度会一并删除，删掉之后不能恢复。确认无误请把书名抄进下面。
        </p>
        <div className="field">
          <label htmlFor="delete-confirm-input">输入书名「{title}」以确认</label>
          <input
            className="input"
            id="delete-confirm-input"
            autoComplete="off"
            value={typed}
            placeholder={title}
            onChange={(event) => setTyped(event.target.value)}
          />
        </div>
        <div className="confirm-actions">
          <button
            className="btn btn-danger"
            type="button"
            disabled={!matched || busy}
            onClick={confirm}
            data-testid="delete-confirm"
          >
            {busy ? "正在删除…" : "确认删除"}
          </button>
          <button className="btn btn-ghost" type="button" onClick={onClose} disabled={busy}>
            取消
          </button>
        </div>
        {!matched ? (
          <p className="meta" data-testid="delete-hint">
            书名还没对上，确认按钮先不可用。
          </p>
        ) : null}
        {errorText ? (
          <p className="meta" role="alert" data-testid="delete-error">
            {errorText}
          </p>
        ) : null}
      </div>
    </div>
  );
}

/** 删除失败的文案：按状态码翻成人话，不把后端错误直接展示。 */
function deleteErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "这本书已经不在了，刷新页面试试。";
    if (error.status === 409) return "这本书正在被导入任务使用，等任务结束再删。";
    if (error.kind === "timeout") return "删除请求超时了，稍后再试。";
  }
  return "删除没能完成，稍后再试。";
}
