import { useState } from "react";
import { api, ApiError } from "../../api/client";
import { useCollections } from "../../api/hooks";
import type { Collection } from "../../api/types";
import { showToast } from "../../lib/toastStore";

/**
 * 书架筛选区的「书单」筛选 + 管理入口（新建 / 改名 / 删除）。
 * 书单是扁平收藏夹：一本书可属多个书单，这里只管书单本身，不管成员（成员在详情页加）。
 * 所有写操作成功后 reload 列表；删除的正好是当前筛选值时通知父级清空选中，避免筛出空页。
 */
export function CollectionFilter({
  value,
  onChange,
}: {
  value: number | null;
  onChange: (id: number | null) => void;
}) {
  const list = useCollections();
  const [open, setOpen] = useState(false);
  const items = list.data?.items ?? [];

  return (
    <div className="chips" role="group" aria-label="书单筛选" data-testid="filter-collection">
      <select
        className="select"
        value={value ?? ""}
        data-testid="filter-collection-select"
        data-od-id="C-25"
        onChange={(event) => onChange(event.target.value ? Number(event.target.value) : null)}
      >
        <option value="">全部书单</option>
        {items.map((item) => (
          <option key={item.id} value={item.id}>
            {item.name}（{item.bookCount}）
          </option>
        ))}
      </select>
      <button
        className="btn btn-ghost"
        type="button"
        data-testid="collection-manage-open"
        onClick={() => setOpen(true)}
      >
        管理书单
      </button>
      {open ? (
        <CollectionManagerDialog
          items={items}
          onClose={() => setOpen(false)}
          onChanged={() => list.reload()}
          onDeleted={(id) => {
            if (value === id) onChange(null);
            list.reload();
          }}
        />
      ) : null}
    </div>
  );
}

/** 管理弹层：新建 / 改名 / 删除，全部即时生效（书单没有「草稿」概念）。 */
function CollectionManagerDialog({
  items,
  onClose,
  onChanged,
  onDeleted,
}: {
  items: Collection[];
  onClose: () => void;
  onChanged: () => void;
  onDeleted: (id: number) => void;
}) {
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [renaming, setRenaming] = useState<{ id: number; name: string } | null>(null);
  const [confirmingId, setConfirmingId] = useState<number | null>(null);

  async function create() {
    const name = draft.trim();
    if (!name || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.createCollection(name);
      setDraft("");
      onChanged();
      showToast(`已新建书单「${name}」`);
    } catch (err) {
      setError(collectionErrorText(err, "新建"));
    } finally {
      setBusy(false);
    }
  }

  async function rename(id: number) {
    const name = (renaming?.name ?? "").trim();
    if (!name || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.renameCollection(id, name);
      setRenaming(null);
      onChanged();
      showToast("书单名已更新");
    } catch (err) {
      setError(collectionErrorText(err, "改名"));
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: number) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api.deleteCollection(id);
      setConfirmingId(null);
      onDeleted(id);
      showToast("书单已删除（书本身还在书架上）");
    } catch (err) {
      setError(collectionErrorText(err, "删除"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="confirm-backdrop" data-testid="collection-manager" onClick={onClose}>
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="collection-manager-title"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="collection-manager-title">管理书单</h2>
        <p className="meta">书单是扁平收藏夹，一本书可以同时放在多个书单里。</p>

        <div className="field">
          <label htmlFor="collection-new">新建书单</label>
          <input
            className="input"
            id="collection-new"
            value={draft}
            placeholder="例如「今年想读」"
            data-testid="collection-new-input"
            disabled={busy}
            onChange={(event) => setDraft(event.target.value)}
          />
        </div>
        <button
          className="btn btn-primary"
          type="button"
          data-testid="collection-create"
          disabled={busy || draft.trim() === ""}
          onClick={() => void create()}
        >
          新建
        </button>

        {items.length === 0 ? (
          <p className="meta" data-testid="collection-manager-empty">
            还没有书单。新建一个之后，就能在书架上按书单筛选，在详情页把书放进去。
          </p>
        ) : (
          <ul className="drawer-list" data-testid="collection-manager-list">
            {items.map((item) => {
              const editing = renaming?.id === item.id;
              return (
                <li className="check-row" key={item.id} data-testid={`collection-row-${item.id}`}>
                  {editing ? (
                    <input
                      className="input"
                      aria-label={`书单「${item.name}」的新名字`}
                      value={renaming.name}
                      data-testid={`collection-rename-input-${item.id}`}
                      disabled={busy}
                      onChange={(event) => setRenaming({ id: item.id, name: event.target.value })}
                    />
                  ) : (
                    <label>
                      <span>{item.name}</span>
                      <span className="meta">{item.bookCount} 本</span>
                    </label>
                  )}
                  <span className="chips">
                    {editing ? (
                      <>
                        <button
                          className="btn"
                          type="button"
                          data-testid={`collection-rename-save-${item.id}`}
                          disabled={busy}
                          onClick={() => void rename(item.id)}
                        >
                          保存
                        </button>
                        <button className="btn btn-ghost" type="button" onClick={() => setRenaming(null)}>
                          取消
                        </button>
                      </>
                    ) : confirmingId === item.id ? (
                      <>
                        <button
                          className="btn btn-danger"
                          type="button"
                          data-testid={`collection-delete-confirm-${item.id}`}
                          disabled={busy}
                          onClick={() => void remove(item.id)}
                        >
                          确认删除
                        </button>
                        <button className="btn btn-ghost" type="button" onClick={() => setConfirmingId(null)}>
                          取消
                        </button>
                      </>
                    ) : (
                      <>
                        <button
                          className="btn btn-ghost"
                          type="button"
                          data-testid={`collection-rename-${item.id}`}
                          onClick={() => setRenaming({ id: item.id, name: item.name })}
                        >
                          改名
                        </button>
                        <button
                          className="btn btn-ghost"
                          type="button"
                          data-testid={`collection-delete-${item.id}`}
                          onClick={() => setConfirmingId(item.id)}
                        >
                          删除
                        </button>
                      </>
                    )}
                  </span>
                </li>
              );
            })}
          </ul>
        )}

        {error ? (
          <p className="meta" role="alert" data-testid="collection-manager-error">
            {error}
          </p>
        ) : null}

        <div className="confirm-actions">
          <button className="btn btn-primary" type="button" data-testid="collection-manager-close" onClick={onClose}>
            完成
          </button>
        </div>
      </div>
    </div>
  );
}

/** 书单写操作失败的文案：重名单独说人话，其余按状态码兜底。 */
function collectionErrorText(error: unknown, action: string): string {
  if (error instanceof ApiError) {
    if (error.status === 409) return "已经有同名书单了，换个名字。";
    if (error.status === 404) return "这个书单已经不在了，刷新看看。";
    if (error.status === 400) return "书单名不符合要求（不能为空）。";
    if (error.kind === "timeout") return `${action}超时了，稍后再试。`;
  }
  return `${action}没能完成，稍后再试。`;
}
