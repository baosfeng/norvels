import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { useBooks } from "../../api/hooks";
import type { BookSummary, Collection } from "../../api/types";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { showToast } from "../../lib/toastStore";
import { fetchAllMemberIds, memberErrorText, withoutMember, MEMBER_PAGE_SIZE } from "./collectionMembers";

/**
 * 书单成员面板（行内展开）：封面 + 书名 + 作者 + 章节数，单本可移出。
 *
 * 三个取舍：
 *   - 成员单独取（GET /api/books?collection=id），不塞进书单列表接口：书单列表只给本数，
 *     展开哪一个才取哪一个，书单多时不会一次拉几百本书；
 *   - 移出走「全量替换」而不是新增单本接口：先翻页取全量成员再减去这一本，
 *     避免只拿第一页就写回把超长书单截断；
 *   - 移出即时生效 + 提示条，不做二次确认：这是可逆操作（再加回来即可），
 *     不像删书单那样不可恢复。
 */
export function CollectionMembers({
  collection,
  refreshToken,
  onChanged,
}: {
  collection: Collection;
  /** 父行在「添加书籍」成功后 +1，让成员面板重新取数（本组件不重挂也能刷新）。 */
  refreshToken: number;
  onChanged: () => void;
}) {
  const [page, setPage] = useState(1);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [error, setError] = useState("");
  const { data, loading, error: loadError, reload } = useBooks({
    collection: collection.id,
    page,
    size: MEMBER_PAGE_SIZE,
    sort: "updated",
    limit: MEMBER_PAGE_SIZE,
    offset: (page - 1) * MEMBER_PAGE_SIZE,
  });

  const tokenRef = useRef(refreshToken);
  useEffect(() => {
    if (tokenRef.current === refreshToken) return;
    tokenRef.current = refreshToken;
    setPage(1);
    reload();
  }, [refreshToken, reload]);

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / MEMBER_PAGE_SIZE));

  /** 移出：先取全量成员，减去这一本后再全量写回。 */
  async function removeBook(book: BookSummary) {
    if (busyId !== null) return;
    setBusyId(book.id);
    setError("");
    try {
      const existing = await fetchAllMemberIds(async (pageNo, size) => {
        const res = await api.books({ collection: collection.id, page: pageNo, size });
        return { ids: res.items.map((item) => item.id), total: res.total };
      });
      await api.replaceCollectionBooks(collection.id, withoutMember(existing, book.id));
      if (items.length === 1 && page > 1) setPage(page - 1);
      else reload();
      onChanged();
      showToast(`已从「${collection.name}」移出《${book.title}》`);
    } catch (err) {
      setError(memberErrorText(err, "移出"));
    } finally {
      setBusyId(null);
    }
  }

  if (loading && !data) {
    return <LoadingPanel title="正在读取这个书单的书" description="稍等一下。" testId={`collection-members-loading-${collection.id}`} />;
  }
  if (loadError) {
    return (
      <ErrorPanel
        error={loadError}
        title="这个书单的书没能读取"
        onRetry={reload}
        testId={`collection-members-error-${collection.id}`}
      />
    );
  }
  if (items.length === 0) {
    return (
      <EmptyPanel
        title="这个书单还没有书"
        description="点上面的「添加书籍」，从书库里挑几本放进来。"
        testId={`collection-members-empty-${collection.id}`}
      />
    );
  }

  return (
    <>
      <ul className="collection-members" data-testid={`collection-members-${collection.id}`}>
        {items.map((book) => (
          <li className="collection-member" key={book.id} data-testid={`collection-member-${collection.id}-${book.id}`}>
            <Cover
              title={book.title}
              bookId={book.id}
              updatedAt={book.updatedAt}
              hasCover={book.hasCover}
              className="collection-member-cover"
              showLabel={false}
            />
            <span className="collection-member-main">
              <b>{book.title}</b>
              <span className="meta">
                {book.authorName || "佚名"} · {book.chapterCount} 章
              </span>
            </span>
            <button
              className="btn btn-ghost"
              type="button"
              disabled={busyId !== null}
              data-testid={`collection-member-remove-${collection.id}-${book.id}`}
              onClick={() => void removeBook(book)}
            >
              {busyId === book.id ? "移出中…" : "移出"}
            </button>
          </li>
        ))}
      </ul>
      {error ? (
        <p className="inline-note is-error" role="alert" data-testid={`collection-members-op-error-${collection.id}`}>
          {error}
        </p>
      ) : null}
      {total > MEMBER_PAGE_SIZE ? (
        <div className="collection-members-pager">
          <button
            className="btn btn-sm"
            type="button"
            disabled={page <= 1 || loading}
            data-testid={`collection-members-prev-${collection.id}`}
            onClick={() => setPage((prev) => Math.max(1, prev - 1))}
          >
            上一页
          </button>
          <span className="meta">
            第 {page} / {pages} 页 · 共 {total} 本
          </span>
          <button
            className="btn btn-sm"
            type="button"
            disabled={page >= pages || loading}
            data-testid={`collection-members-next-${collection.id}`}
            onClick={() => setPage((prev) => Math.min(pages, prev + 1))}
          >
            下一页
          </button>
        </div>
      ) : null}
    </>
  );
}