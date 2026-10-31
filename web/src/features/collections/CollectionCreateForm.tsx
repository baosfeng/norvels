import { useState, type FormEvent } from "react";
import { api } from "../../api/client";
import { showToast } from "../../lib/toastStore";
import { collectionErrorText } from "./collectionErrors";

/**
 * 书单页的新建入口：书单没有「草稿」概念，提交即生效。
 * 成功后清空输入并让页面重取列表；失败把后端说法翻成人话，不静默。
 */
export function CollectionCreateForm({ onCreated }: { onCreated: () => void }) {
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    const name = draft.trim();
    if (!name || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.createCollection(name);
      setDraft("");
      onCreated();
      showToast(`已新建书单「${name}」`);
    } catch (err) {
      setError(collectionErrorText(err, "新建"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="collections-create" data-testid="collections-create-form" onSubmit={submit}>
      <div className="collections-create-row">
        <input
          className="input"
          id="collection-new"
          value={draft}
          placeholder="例如「今年想读」"
          aria-label="新建书单的名字"
          data-testid="collections-new-input"
          disabled={busy}
          maxLength={64}
          onChange={(event) => setDraft(event.target.value)}
        />
        <button
          className="btn btn-primary"
          type="submit"
          data-testid="collections-create"
          disabled={busy || draft.trim() === ""}
        >
          新建
        </button>
      </div>
      {error ? (
        <p className="inline-note is-error" role="alert" data-testid="collections-create-error">
          {error}
        </p>
      ) : null}
    </form>
  );
}
