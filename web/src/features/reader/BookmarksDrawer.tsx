import type { Bookmark } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { bookmarkTitle } from "../../lib/bookmark";
import type { ResourceState } from "../../lib/useResource";

/**
 * 书签抽屉：与目录抽屉同级、互斥打开（父组件负责互斥），列表按时间倒序。
 * 每行可跳转到该段；「在这里加书签」记在当前读到的段落（段落下标即后端 paraIndex）。
 */
export function BookmarksDrawer({
  open,
  bookmarks,
  currentChapterId,
  deletingId,
  adding,
  canAdd,
  onClose,
  onAdd,
  onJump,
  onDelete,
}: {
  open: boolean;
  bookmarks: ResourceState<{ bookmarks: Bookmark[] }>;
  currentChapterId: number;
  deletingId: number | null;
  adding: boolean;
  canAdd: boolean;
  onClose: () => void;
  onAdd: () => void;
  onJump: (bookmark: Bookmark) => void;
  onDelete: (id: number) => void;
}) {
  const list = bookmarks.data?.bookmarks ?? [];
  return (
    <aside
      className={`drawer${open ? " is-open" : ""}`}
      aria-label="书签"
      aria-hidden={!open}
      data-testid="bookmark-drawer"
      data-od-id="reader-bookmarks"
    >
      <div className="drawer-head">
        <strong>书签</strong>
        <span className="meta num">{list.length > 0 ? `${list.length} 条` : ""}</span>
        <button className="btn btn-ghost" type="button" onClick={onClose}>
          关闭
        </button>
      </div>
      <div className="drawer-body">
        <button
          className="btn bookmark-add"
          type="button"
          data-testid="bookmark-add"
          onClick={onAdd}
          disabled={!canAdd || adding}
        >
          {adding ? "正在保存…" : "在这里加书签"}
        </button>
        {bookmarks.loading ? (
          <LoadingPanel title="正在读取书签" description="稍等一下。" />
        ) : bookmarks.error ? (
          <ErrorPanel error={bookmarks.error} title="书签没能加载" onRetry={bookmarks.reload} testId="bookmark-error" />
        ) : list.length === 0 ? (
          <EmptyPanel
            title="还没有书签"
            description="读到想记的地方，点上面的「在这里加书签」，下次回来一眼就能跳回来。"
            testId="bookmark-empty"
          />
        ) : (
          <div className="drawer-scroll bookmark-list" data-testid="bookmark-list">
            {list.map((bookmark) => (
              <div
                key={bookmark.id}
                className={`bookmark-row${bookmark.chapterId === currentChapterId ? " is-current" : ""}`}
                data-testid={`bookmark-${bookmark.id}`}
                data-current={bookmark.chapterId === currentChapterId}
              >
                <button className="bookmark-jump" type="button" onClick={() => onJump(bookmark)}>
                  <span className="num">{bookmark.chapterTitle || `第 ${bookmark.chapterIdx + 1} 章`}</span>
                  <span className="bookmark-label">{bookmarkTitle(bookmark)}</span>
                </button>
                <button
                  className="bookmark-delete"
                  type="button"
                  aria-label={`删除书签 ${bookmarkTitle(bookmark)}`}
                  disabled={deletingId === bookmark.id}
                  onClick={() => onDelete(bookmark.id)}
                >
                  删除
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </aside>
  );
}
