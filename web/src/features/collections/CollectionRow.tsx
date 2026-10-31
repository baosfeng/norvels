import { useState } from "react";
import type { Collection } from "../../api/types";
import { AddBooksDialog } from "./AddBooksDialog";
import { CollectionMembers } from "./CollectionMembersPanel";
import { useCollectionRow } from "./useCollectionRow";

/**
 * 书单列表的一行：名称 + 本数 + 改名 / 删除。
 *
 * 删除按两段式：点「删除」只进入确认态，确认后才发请求（书单没了书不会跟着没，这一句写在提示里）。
 * 改名与删除各自就地展开，不弹层——页面本身就是书单的维护面，再套弹层只是多一层遮挡。
 *
 * 成员管理（本页新增）：行内展开成员列表（可单本移出）；「添加书籍」是唯一弹层——
 * 选书要搜索 + 多选，行内塞不下；写完回本页重取，展开着的成员列表同步刷新。
 */
export function CollectionRow({ item, onChanged }: { item: Collection; onChanged: () => void }) {
  const row = useCollectionRow(item, onChanged);
  const editing = row.mode === "renaming";
  const [expanded, setExpanded] = useState(false);
  const [adding, setAdding] = useState(false);
  /** 成员面板的重取令牌：添加成功后 +1，展开着的成员列表跟着刷新。 */
  const [membersRev, setMembersRev] = useState(0);

  return (
    <li
      className={`collections-row${row.mode === "confirming" ? " is-confirming" : ""}`}
      data-testid={`collections-row-${item.id}`}
    >
      <div className="collections-row-main">
        {editing ? (
          <input
            className="input collections-rename-input"
            aria-label={`书单「${item.name}」的新名字`}
            value={row.draft}
            data-testid={`collections-rename-input-${item.id}`}
            disabled={row.busy}
            maxLength={64}
            onChange={(event) => row.setDraft(event.target.value)}
          />
        ) : (
          <span className="collections-row-name">
            <strong>{item.name}</strong>
            <span className="meta" data-testid={`collections-count-${item.id}`}>
              {item.bookCount} 本
            </span>
          </span>
        )}
        <RowActions
          item={item}
          row={row}
          expanded={expanded}
          onToggleMembers={() => setExpanded((prev) => !prev)}
          onAdd={() => setAdding(true)}
        />
      </div>
      {row.error ? (
        <p className="inline-note is-error" role="alert" data-testid={`collections-row-error-${item.id}`}>
          {row.error}
        </p>
      ) : null}
      {expanded && !editing ? (
        <div className="collections-members-wrap" id={`collections-members-${item.id}`}>
          <CollectionMembers collection={item} refreshToken={membersRev} onChanged={onChanged} />
        </div>
      ) : null}
      {adding ? (
        <AddBooksDialog
          collection={item}
          onClose={() => setAdding(false)}
          onAdded={() => {
            setAdding(false);
            setExpanded(true);
            setMembersRev((prev) => prev + 1);
            onChanged();
          }}
        />
      ) : null}
    </li>
  );
}

/** 行右侧的动作：三种状态各自两个按钮，尺寸与筛选组里的 .btn 一致。 */
function RowActions({
  item,
  row,
  expanded,
  onToggleMembers,
  onAdd,
}: {
  item: Collection;
  row: ReturnType<typeof useCollectionRow>;
  expanded: boolean;
  onToggleMembers: () => void;
  onAdd: () => void;
}) {
  if (row.mode === "renaming") {
    return (
      <span className="collections-row-actions">
        <button
          className="btn btn-primary"
          type="button"
          data-testid={`collections-rename-save-${item.id}`}
          disabled={row.busy || row.draft.trim() === ""}
          onClick={() => void row.saveName()}
        >
          保存
        </button>
        <button className="btn btn-ghost" type="button" disabled={row.busy} onClick={row.cancel}>
          取消
        </button>
      </span>
    );
  }

  if (row.mode === "confirming") {
    return (
      <span className="collections-row-actions">
        <button
          className="btn btn-danger"
          type="button"
          data-testid={`collections-delete-confirm-${item.id}`}
          disabled={row.busy}
          onClick={() => void row.remove()}
        >
          确认删除
        </button>
        <button className="btn btn-ghost" type="button" disabled={row.busy} onClick={row.cancel}>
          取消
        </button>
      </span>
    );
  }

  return (
    <span className="collections-row-actions">
      <button
        className="btn btn-ghost collections-members-toggle"
        type="button"
        aria-expanded={expanded}
        aria-controls={`collections-members-${item.id}`}
        data-testid={`collections-members-toggle-${item.id}`}
        disabled={row.busy}
        onClick={onToggleMembers}
      >
        {expanded ? "收起成员" : `成员 ${item.bookCount} 本`}
      </button>
      <button
        className="btn btn-ghost"
        type="button"
        data-testid={`collections-add-books-${item.id}`}
        disabled={row.busy}
        onClick={onAdd}
      >
        添加书籍
      </button>
      <button
        className="btn btn-ghost"
        type="button"
        data-testid={`collections-rename-${item.id}`}
        disabled={row.busy}
        onClick={row.startRename}
      >
        改名
      </button>
      <button
        className="btn btn-ghost"
        type="button"
        data-testid={`collections-delete-${item.id}`}
        disabled={row.busy}
        onClick={() => row.setMode("confirming")}
      >
        删除
      </button>
    </span>
  );
}