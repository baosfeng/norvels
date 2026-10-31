import { useState } from "react";
import { api } from "../../api/client";
import type { Collection } from "../../api/types";
import { showToast } from "../../lib/toastStore";
import { collectionErrorText } from "./collectionErrors";

export type RowMode = "idle" | "renaming" | "confirming";

/**
 * 一行书单的就地操作：改名与删除同一时刻只展开一个，写成功后让页面重取列表。
 * 删除是两段式（点「删除」→ 确认态 → 确认才发请求），与以前弹层里的行为一致。
 */
export function useCollectionRow(item: Collection, onChanged: () => void) {
  const [mode, setMode] = useState<RowMode>("idle");
  const [draft, setDraft] = useState(item.name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  function startRename() {
    setDraft(item.name);
    setError("");
    setMode("renaming");
  }

  function cancel() {
    setDraft(item.name);
    setError("");
    setMode("idle");
  }

  async function saveName() {
    const name = draft.trim();
    if (!name || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.renameCollection(item.id, name);
      setMode("idle");
      onChanged();
      showToast("书单名已更新");
    } catch (err) {
      setError(collectionErrorText(err, "改名"));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api.deleteCollection(item.id);
      setMode("idle");
      onChanged();
      showToast("书单已删除（书本身还在书架上）");
    } catch (err) {
      setError(collectionErrorText(err, "删除"));
    } finally {
      setBusy(false);
    }
  }

  return { mode, setMode, draft, setDraft, busy, error, startRename, cancel, saveName, remove };
}
