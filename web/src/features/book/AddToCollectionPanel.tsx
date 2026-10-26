import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { useCollections } from "../../api/hooks";
import { showToast } from "../../lib/toastStore";
import { fetchCollectionMembers, toggleMember, type CollectionMembers } from "./collectionMembers";

/** 勾选后的底部提示条：2 秒自散（契约 C-17 反馈）。 */
const TIP_MS = 2000;

/**
 * 详情页「加入书单」面板：书单是扁平收藏夹，一本书可属多个书单。
 * 契约只给全量替换（PUT /collections/{id}/books），所以先取回成员再算「加上/去掉这一本」，
 * 成员取不全（超过上限）的书单直接禁用改动——宁可不给点，也不写坏别的书单。
 *
 * 锚点：C-17 每行勾选框、C-18 主按钮「完成」。
 * 移除后五问：焦点回「加入书单」（由 BookPage 在关闭时补）、滚动位置不动、
 * 书单本数即时更新、本书归属按勾选结果定格。
 */
export function AddToCollectionPanel({
  bookId,
  bookTitle,
  onClose,
}: {
  bookId: number;
  bookTitle: string;
  onClose: () => void;
}) {
  const list = useCollections();
  const [members, setMembers] = useState<Record<number, CollectionMembers>>({});
  const [loadingMembers, setLoadingMembers] = useState(false);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [tip, setTip] = useState("");
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const collections = list.data?.items ?? [];
  /** 成员全部取回之前不渲染勾选框：没取到时勾选框既不能点也会短暂显示成「未加入」。 */
  const membersReady = collections.length > 0 && collections.every((item) => members[item.id] !== undefined);

  useEffect(() => {
    if (collections.length === 0) return;
    let active = true;
    setLoadingMembers(true);
    Promise.all(
      collections.map(async (item) => [item.id, await fetchCollectionMembers(item.id)] as const),
    )
      .then((pairs) => {
        if (!active) return;
        setMembers(Object.fromEntries(pairs));
      })
      .catch(() => {
        if (active) showToast("书单成员没能读取，稍后再试。", "error");
      })
      .finally(() => {
        if (active) setLoadingMembers(false);
      });
    return () => {
      active = false;
    };
    // 只在书单列表变化时重取成员（挂载时一次）
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [list.data]);

  /** 打开期间锁背景滚动（契约 C-04），关闭即还原。 */
  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  /** Esc 与「完成」、点遮罩三条关闭路径等效（契约 C-18 键盘行）。 */
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape") return;
      event.preventDefault();
      onClose();
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  /** 焦点先落在第一个可改的勾选框（禁用行 Tab 会直接跳过）。 */
  useEffect(() => {
    if (!membersReady) return;
    const first = dialogRef.current?.querySelector<HTMLInputElement>("input[type=checkbox]:not([disabled])");
    first?.focus();
  }, [membersReady]);

  useEffect(() => {
    if (!tip) return;
    const timer = window.setTimeout(() => setTip(""), TIP_MS);
    return () => window.clearTimeout(timer);
  }, [tip]);

  async function toggle(collectionId: number, add: boolean) {
    const current = members[collectionId];
    if (!current || busyId !== null) return;
    const next = toggleMember(current, bookId, add);
    if (!next) {
      showToast("这个书单里的书太多了，先在这里改动可能会丢成员。", "error");
      return;
    }
    setBusyId(collectionId);
    setMembers((prev) => ({ ...prev, [collectionId]: { ids: next, complete: true } }));
    try {
      await api.replaceCollectionBooks(collectionId, next);
      setTip(add ? "已加入书单" : "已移出书单");
    } catch {
      setMembers((prev) => ({ ...prev, [collectionId]: current }));
      showToast("书单没能保存，稍后再试。", "error");
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="confirm-backdrop" data-testid="add-to-collection-panel" onClick={onClose}>
      <div
        ref={dialogRef}
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="add-to-collection-title"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="add-to-collection-title">加入书单</h2>
        <p className="meta" data-testid="add-to-collection-book">
          《{bookTitle}》可以同时放进多个书单。
        </p>

        {list.loading || loadingMembers || (collections.length > 0 && !membersReady) ? (
          <p className="meta" role="status">
            正在读取书单…
          </p>
        ) : list.error ? (
          <p className="meta" role="alert" data-testid="add-to-collection-error">
            书单暂时读不到，稍后再试。
          </p>
        ) : collections.length === 0 ? (
          <p className="meta" data-testid="add-to-collection-empty">
            还没有书单。到书架的筛选区点「管理书单」新建一个，再回来把书放进去。
          </p>
        ) : (
          <ul className="drawer-list" data-testid="add-to-collection-list">
            {collections.map((item) => {
              const state = members[item.id];
              const checked = Boolean(state?.ids.includes(bookId));
              const blocked = state ? !state.complete : false;
              return (
                <li className={`check-row${blocked ? " is-disabled" : ""}`} key={item.id}>
                  <label>
                    <input
                      type="checkbox"
                      checked={checked}
                      disabled={busyId !== null || blocked || !state}
                      data-testid={`collection-check-${item.id}`}
                      data-od-id="C-17"
                      onChange={(event) => void toggle(item.id, event.target.checked)}
                    />
                    <span>{item.name}</span>
                  </label>
                  <span className="meta">
                    {blocked ? "成员过多，暂不支持在这里改动" : `${item.bookCount} 本`}
                  </span>
                </li>
              );
            })}
          </ul>
        )}

        {busyId !== null ? <p className="inline-note">正在保存这一次改动，完成前先别改下一个。</p> : null}
        {tip ? (
          <p className="inline-note is-tip" role="status" data-testid="add-to-collection-tip">
            {tip}
          </p>
        ) : null}

        <div className="confirm-actions">
          <button
            className="btn btn-primary"
            type="button"
            data-testid="add-to-collection-close"
            data-od-id="C-18"
            onClick={onClose}
          >
            完成
          </button>
        </div>
      </div>
    </div>
  );
}
