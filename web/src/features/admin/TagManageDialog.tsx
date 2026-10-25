import { useState } from "react";
import { ApiError, api } from "../../api/client";
import type { TagSummary } from "../../api/types";

/**
 * 标签管理（新建 / 改名 / 删除）。
 *
 * 标签列表由父组件（书籍管理页的筛选条）持有，这里只负责一次动作一次 reload：
 * 两处各挂一份 useTags 会各自发一次请求，还可能出现两套不一致的书数。
 * 删除是「全库解绑」而不是删书——弹窗必须把这件事说清楚，否则没人敢点。
 */
export function TagManageDialog({
  tags,
  loading,
  error,
  onReload,
  onClose,
  onChanged,
}: {
  tags: TagSummary[];
  loading: boolean;
  error: ApiError | null;
  onReload: () => void;
  onClose: () => void;
  /** 每次动作成功后通知页面（顶部提示条用），文案与弹窗内的一致 */
  onChanged: (message: string) => void;
}) {
  const [name, setName] = useState("");
  /** 正在进行的动作：新建用 "create"，改名/删除用标签 id；null = 空闲 */
  const [busyWith, setBusyWith] = useState<"create" | number | null>(null);
  const [renaming, setRenaming] = useState<{ id: number; text: string } | null>(null);
  const [deleting, setDeleting] = useState<TagSummary | null>(null);
  const [note, setNote] = useState("");
  const [errorText, setErrorText] = useState("");
  const busy = busyWith !== null;

  function succeeded(message: string) {
    setNote(message);
    setErrorText("");
    onChanged(message);
    onReload();
  }

  async function create() {
    const value = name.trim();
    if (!value) {
      setErrorText("先填一个标签名。");
      return;
    }
    setBusyWith("create");
    setErrorText("");
    try {
      const tag = await api.createTag(value);
      setName("");
      succeeded(`标签「${tag.name}」已创建。`);
    } catch (err) {
      setErrorText(tagErrorText(err, "新建标签没能完成"));
    } finally {
      setBusyWith(null);
    }
  }

  async function rename(tag: TagSummary) {
    const text = (renaming?.text ?? "").trim();
    if (!text) {
      setErrorText("标签名不能为空。");
      return;
    }
    if (text === tag.name) {
      setRenaming(null);
      return;
    }
    setBusyWith(tag.id);
    setErrorText("");
    try {
      const next = await api.renameTag(tag.id, text);
      setRenaming(null);
      succeeded(`标签已改名为「${next.name}」。`);
    } catch (err) {
      setErrorText(tagErrorText(err, "改名没能完成"));
    } finally {
      setBusyWith(null);
    }
  }

  async function remove(tag: TagSummary) {
    setBusyWith(tag.id);
    setErrorText("");
    try {
      await api.deleteTag(tag.id);
      setDeleting(null);
      succeeded(`标签「${tag.name}」已删除，从 ${tag.bookCount} 本书解绑，书本身都还在。`);
    } catch (err) {
      setErrorText(tagErrorText(err, "删除标签没能完成"));
    } finally {
      setBusyWith(null);
    }
  }

  return (
    <div className="confirm-backdrop" role="presentation" onClick={() => !busy && onClose()}>
      <div
        className="confirm-dialog is-wide"
        role="dialog"
        aria-modal="true"
        aria-labelledby="tag-manage-title"
        data-testid="tag-manage-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="tag-manage-title">{deleting ? `删除标签「${deleting.name}」？` : "标签管理"}</h2>

        {errorText ? (
          <p className="meta" role="alert" data-testid="tag-manage-error">
            {errorText}
          </p>
        ) : null}
        {note ? (
          <p className="meta" role="status" data-testid="tag-manage-note">
            {note}
          </p>
        ) : null}

        {deleting ? (
          <>
            <p className="meta" data-testid="tag-delete-warning">
              会把这个标签从 <strong className="num">{deleting.bookCount}</strong> 本书上解绑，
              书、正文与阅读进度都不会被删除。
            </p>
            <div className="confirm-actions">
              <button
                className="btn btn-danger"
                type="button"
                autoFocus
                disabled={busy}
                data-testid="tag-delete-confirm"
                onClick={() => void remove(deleting)}
              >
                {busy ? "正在删除…" : "确认删除标签"}
              </button>
              <button
                className="btn btn-ghost"
                type="button"
                disabled={busy}
                data-testid="tag-delete-cancel"
                onClick={() => setDeleting(null)}
              >
                取消
              </button>
            </div>
          </>
        ) : (
          <>
            <p className="meta">
              标签是书的多对多标记：改名只改标记本身，删除只解绑、不删书。导入时的词表规则也会打这些标签。
            </p>

            {loading ? (
              <p className="meta" data-testid="tag-manage-loading">
                正在读取标签…
              </p>
            ) : error ? (
              <div role="alert" data-testid="tag-manage-load-error">
                <p className="meta">标签列表没能载入。</p>
                <button className="btn btn-sm" type="button" onClick={onReload}>
                  重试
                </button>
              </div>
            ) : tags.length === 0 ? (
              <p className="meta" data-testid="tag-manage-empty">
                还没有标签。新建一个，之后在书的元信息里就能填。
              </p>
            ) : (
              <ul
                data-testid="tag-manage-list"
                style={{ display: "grid", gap: 8, listStyle: "none", margin: 0, padding: 0 }}
              >
                {tags.map((tag) => (
                  <li
                    key={tag.id}
                    data-testid={`tag-row-${tag.id}`}
                    style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}
                  >
                    {renaming?.id === tag.id ? (
                      <>
                        <input
                          className="input"
                          style={{ flex: "1 1 160px" }}
                          aria-label={`新标签名（${tag.name}）`}
                          data-testid="tag-rename-input"
                          value={renaming.text}
                          disabled={busy}
                          onChange={(event) => {
                            setRenaming({ id: tag.id, text: event.target.value });
                            setErrorText("");
                          }}
                        />
                        <button
                          className="btn btn-sm btn-primary"
                          type="button"
                          disabled={busy}
                          data-testid="tag-rename-save"
                          onClick={() => void rename(tag)}
                        >
                          保存
                        </button>
                        <button
                          className="btn btn-sm btn-ghost"
                          type="button"
                          disabled={busy}
                          data-testid="tag-rename-cancel"
                          onClick={() => setRenaming(null)}
                        >
                          取消
                        </button>
                      </>
                    ) : (
                      <>
                        <strong style={{ flex: "1 1 160px" }}>{tag.name}</strong>
                        <span className="meta num">{tag.bookCount} 本</span>
                        <button
                          className="btn btn-sm"
                          type="button"
                          disabled={busy}
                          data-testid={`tag-rename-${tag.id}`}
                          onClick={() => {
                            setRenaming({ id: tag.id, text: tag.name });
                            setErrorText("");
                          }}
                        >
                          改名
                        </button>
                        <button
                          className="btn btn-sm btn-ghost btn-danger"
                          type="button"
                          disabled={busy}
                          data-testid={`tag-delete-${tag.id}`}
                          onClick={() => {
                            setDeleting(tag);
                            setErrorText("");
                          }}
                        >
                          删除
                        </button>
                      </>
                    )}
                  </li>
                ))}
              </ul>
            )}

            <div className="field-row" style={{ alignItems: "flex-end" }}>
              <div className="field" style={{ flex: 1 }}>
                <label htmlFor="tag-new">新建标签</label>
                <input
                  className="input"
                  id="tag-new"
                  data-testid="tag-new"
                  value={name}
                  disabled={busy}
                  placeholder="例如 武侠；同名会直接复用已有的"
                  onChange={(event) => {
                    setName(event.target.value);
                    setErrorText("");
                  }}
                />
              </div>
              <button
                className="btn btn-primary"
                type="button"
                disabled={busy}
                data-testid="tag-create"
                onClick={() => void create()}
              >
                {busyWith === "create" ? "正在新建…" : "新建"}
              </button>
            </div>

            <div className="confirm-actions">
              <button className="btn" type="button" disabled={busy} data-testid="tag-manage-close" onClick={onClose}>
                关闭
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

/** 失败文案：重名 / 已消失 / 参数不合法分开说，不把后端错误原文甩给用户。 */
function tagErrorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    if (error.status === 409) return "已经有同名标签了，换个名字再试。";
    if (error.status === 404) return "这个标签已经不在了，关掉重开一次列表。";
    if (error.status === 400) return error.message || "标签名不符合要求：不能为空，也不能太长。";
    if (error.kind === "timeout") return `${fallback}（超时），稍后再试。`;
  }
  return `${fallback}，稍后再试。`;
}
