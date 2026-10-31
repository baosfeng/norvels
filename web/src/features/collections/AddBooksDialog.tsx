import { useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { api } from "../../api/client";
import { useBooks } from "../../api/hooks";
import type { BookQuery } from "../../api/client";
import type { Collection } from "../../api/types";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { showToast } from "../../lib/toastStore";
import { fetchAllMemberIds, memberErrorText, mergeMemberIds } from "./collectionMembers";

/** 选择器里一次列多少本候选书。 */
const PICKER_PAGE_SIZE = 20;

/** 弹层内可聚焦的元素（按 DOM 顺序）：焦点陷阱用。 */
function focusableIn(root: HTMLElement): HTMLElement[] {
  const nodes = root.querySelectorAll<HTMLElement>(
    'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])',
  );
  return [...nodes].filter((node) => !node.hidden && node.getAttribute("aria-hidden") !== "true");
}

/**
 * 添加书籍弹层：搜索书库、勾选多本、确认后全量写回。
 *
 * 三个取舍：
 *   - 写回前先「重取现有成员再合并」：PUT 是全量替换，直接拿前端缓存的成员会覆盖
 *     并发改动（另一台设备刚加过书）；重取走翻页，书单超过单页上限也不会被截断；
 *   - 搜索复用 GET /api/books?q=（书库列表接口），不新开端点：书名/作者都能命中；
 *   - 已选中的书在列表里保持勾选状态，换关键词也不丢——选择集是 id 集合，不是列表下标。
 */
export function AddBooksDialog({
  collection,
  onClose,
  onAdded,
}: {
  collection: Collection;
  onClose: () => void;
  onAdded: (added: number) => void;
}) {
  const [q, setQ] = useState("");
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  /** 书单现有成员：列表里标「已加入」并禁止再勾，读完才知道该标谁。 */
  const [members, setMembers] = useState<Set<number> | null>(null);
  /** 成员没读出来时的兜底说明：仍可勾，保存时按合并去重。 */
  const [membersUnknown, setMembersUnknown] = useState(false);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const keyword = q.trim();
  const params: BookQuery = keyword
    ? { q: keyword, page: 1, size: PICKER_PAGE_SIZE, sort: "title" }
    : { page: 1, size: PICKER_PAGE_SIZE, sort: "updated" };
  const { data, loading, error: loadError, reload } = useBooks(params);
  const items = data?.items ?? [];

  /** 打开时记下触发按钮，关闭后把焦点还给它（modal 语义，键盘用户不迷路）。 */
  const restoreRef = useRef<HTMLElement | null>(null);
  useEffect(() => {
    restoreRef.current = document.activeElement as HTMLElement | null;
    return () => {
      const back = restoreRef.current;
      if (back && document.contains(back)) back.focus();
    };
  }, []);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  /** 打开弹层时锁住背景滚动（只锁这个弹层；阅读器/健康卡抽屉各有取舍，不统一）。 */
  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  /** 取书单现有成员：用于「已加入」标记；失败不挡操作（写回时会去重）。 */
  useEffect(() => {
    let active = true;
    fetchAllMemberIds(async (page, size) => {
      const res = await api.books({ collection: collection.id, page, size });
      return { ids: res.items.map((item) => item.id), total: res.total };
    })
      .then((ids) => {
        if (active) setMembers(new Set(ids));
      })
      .catch(() => {
        if (active) {
          setMembers(new Set());
          setMembersUnknown(true);
        }
      });
    return () => {
      active = false;
    };
  }, [collection.id]);

  /** 焦点陷阱：Tab 走到边界绕回另一端，不会跑到弹层背后的列表上。 */
  function trapTab(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (event.key !== "Tab") return;
    const nodes = focusableIn(event.currentTarget);
    if (nodes.length === 0) return;
    const first = nodes[0];
    const last = nodes[nodes.length - 1];
    const active = document.activeElement;
    const outside = !event.currentTarget.contains(active);
    if (event.shiftKey ? active === first || outside : active === last || outside) {
      event.preventDefault();
      (event.shiftKey ? last : first).focus();
    }
  }

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  function toggle(bookId: number) {
    // 已是成员的书不可再勾（checkbox 也 disabled，这里再兜一层）
    if (members?.has(bookId)) return;
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(bookId)) next.delete(bookId);
      else next.add(bookId);
      return next;
    });
  }

  async function confirm() {
    if (busy || picked.size === 0) return;
    setBusy(true);
    setError("");
    try {
      const existing = await fetchAllMemberIds(async (page, size) => {
        const res = await api.books({ collection: collection.id, page, size });
        return { ids: res.items.map((item) => item.id), total: res.total };
      });
      const merged = mergeMemberIds(existing, [...picked]);
      await api.replaceCollectionBooks(collection.id, merged);
      showToast(`已添加 ${merged.length - existing.length} 本到「${collection.name}」`);
      onAdded(merged.length - existing.length);
    } catch (err) {
      setError(memberErrorText(err, "添加"));
      setBusy(false);
    }
  }

  return (
    <>
      <div className="overlay is-open" role="presentation" data-testid="add-books-overlay" onClick={onClose} />
      <div
        className="add-books-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="add-books-title"
        data-testid="add-books-dialog"
        onKeyDown={trapTab}
      >
        <div className="add-books-head">
          <div>
            <p className="eyebrow">书单 · {collection.name}</p>
            <h2 id="add-books-title" data-testid="add-books-title">
              添加书籍
            </h2>
            <p className="meta">
              已选 <b data-testid="add-books-picked">{picked.size}</b> 本
            </p>
          </div>
          <button className="btn btn-sm" type="button" data-testid="add-books-close" onClick={onClose}>
            关闭
          </button>
        </div>

        <label className="add-books-search">
          <span className="sr-only">搜索书名或作者</span>
          <input
            className="input"
            ref={inputRef}
            type="search"
            placeholder="搜书名或作者，至少 1 个字就开始筛"
            aria-label="搜索书名或作者"
            value={q}
            data-testid="add-books-search"
            disabled={busy}
            onChange={(event) => setQ(event.target.value)}
          />
        </label>

        <div className="add-books-body">
          {loading && !data ? (
            <LoadingPanel title="正在读取书库" description="稍等一下。" testId="add-books-loading" />
          ) : loadError ? (
            <ErrorPanel error={loadError} title="书库没能读取" onRetry={reload} testId="add-books-error" />
          ) : items.length === 0 ? (
            <EmptyPanel
              title={keyword ? "没有匹配的书" : "书库还是空的"}
              description={keyword ? "换个关键词试试，书名和作者都能搜。" : "先把 txt 导进来，再来挑书。"}
              testId="add-books-empty"
            />
          ) : (
            <>
              {membersUnknown ? (
                <p className="inline-note is-tip" role="status" data-testid="add-books-members-unknown">
                  没能读取书单里已有的书，重复的书会在保存时自动去重。
                </p>
              ) : null}
              <ul className="add-books-list" data-testid="add-books-list">
              {items.map((book) => {
                const on = picked.has(book.id);
                const isMember = members?.has(book.id) ?? false;
                return (
                  <li key={book.id}>
                    <label
                      className={`add-books-item${on ? " is-picked" : ""}${isMember ? " is-member" : ""}`}
                      data-testid={`add-books-item-${book.id}`}
                    >
                      <input
                        className="add-books-check"
                        type="checkbox"
                        checked={on}
                        disabled={isMember || busy}
                        data-testid={`add-books-pick-${book.id}`}
                        onChange={() => toggle(book.id)}
                      />
                      <Cover
                        title={book.title}
                        bookId={book.id}
                        updatedAt={book.updatedAt}
                        hasCover={book.hasCover}
                        className="add-books-cover"
                        showLabel={false}
                      />
                      <span className="add-books-main">
                        <b>{book.title}</b>
                        <span className="meta">
                          {book.authorName || "佚名"} · {book.chapterCount} 章
                        </span>
                      </span>
                      {isMember ? (
                        <span className="add-books-tag" data-testid={`add-books-member-${book.id}`}>
                          已加入
                        </span>
                      ) : null}
                    </label>
                  </li>
                );
              })}
              </ul>
            </> 
          )}
        </div>

        {error ? (
          <p className="inline-note is-error" role="alert" data-testid="add-books-op-error">
            {error}
          </p>
        ) : null}

        <div className="add-books-foot">
          <span className="meta">确认后把勾选的书加进书单，原来的成员不动。</span>
          <button
            className="btn btn-primary"
            type="button"
            disabled={busy || picked.size === 0}
            data-testid="add-books-confirm"
            onClick={() => void confirm()}
          >
            {busy ? "添加中…" : `添加 ${picked.size} 本`}
          </button>
        </div>
      </div>
    </>
  );
}