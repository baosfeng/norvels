import { useRef } from "react";
import type { Bookmark } from "../../api/types";
import { EmptyPanel, LoadingPanel } from "../../components/StatePanel";
import { useFocusTrap } from "../../lib/focusTrap";
import { bookmarkTitle } from "../../lib/bookmark";
import type { ResourceState } from "../../lib/useResource";

/**
 * 书签抽屉：与目录抽屉同级、互斥打开（父组件负责互斥），列表按时间倒序。
 * 每行可跳转到该段；「在这里加书签」记在当前读到的段落。
 * 删除先弹二次确认（C-58），确认框本身在父级渲染——这里只负责发起。
 */
export function BookmarksDrawer({
  open,
  bookmarks,
  currentChapterId,
  adding,
  retrying,
  canAdd,
  onClose,
  onAdd,
  onJump,
  onRequestDelete,
  onRetry,
}: {
  open: boolean;
  bookmarks: ResourceState<{ bookmarks: Bookmark[] }>;
  currentChapterId: number;
  adding: boolean;
  retrying: boolean;
  canAdd: boolean;
  onClose: () => void;
  onAdd: () => void;
  onJump: (bookmark: Bookmark) => void;
  onRequestDelete: (bookmark: Bookmark) => void;
  onRetry: () => void;
}) {
  const list = bookmarks.data?.bookmarks ?? [];
  const rootRef = useRef<HTMLElement | null>(null);
  useFocusTrap(open, rootRef, onClose);
  return (
    <aside
      ref={rootRef}
      className={`drawer${open ? " is-open" : ""}`}
      aria-label="书签"
      aria-hidden={!open}
      data-testid="bookmark-drawer"
      data-od-id="reader-bookmarks"
    >
      <div className="drawer-head">
        <strong>书签</strong>
        <span className="meta num">{list.length > 0 ? `${list.length} 条` : ""}</span>
        <button className="btn btn-ghost" type="button" data-od-id="C-36" onClick={onClose}>
          关闭
        </button>
      </div>
      <div className="drawer-body">
        <button
          className="btn bookmark-add"
          type="button"
          data-testid="bookmark-add"
          data-od-id="C-32"
          onClick={onAdd}
          disabled={!canAdd || adding}
        >
          {adding ? "正在保存…" : "在这里加书签"}
        </button>
        {!canAdd ? <span className="inline-note">正文还没载入，稍后可以加书签</span> : null}
        {bookmarks.loading ? (
          <LoadingPanel title="正在读取书签" description="稍等一下。" />
        ) : bookmarks.error ? (
          <div className="state-panel is-error" data-testid="bookmark-error">
            <span className="state-mark" aria-hidden="true">
              !
            </span>
            <h2>书签没能加载</h2>
            <p>正文不受影响，可以继续往下读。</p>
            <div className="state-actions">
              <button className="btn btn-primary" type="button" disabled={retrying} data-od-id="C-35" onClick={onRetry}>
                {retrying ? "正在重试…" : "重试"}
              </button>
            </div>
          </div>
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
                <button className="bookmark-jump" type="button" data-od-id="C-33" onClick={() => onJump(bookmark)}>
                  <span className="num">{bookmark.chapterTitle || `第 ${bookmark.chapterIdx + 1} 章`}</span>
                  <span className="bookmark-label">{bookmarkTitle(bookmark)}</span>
                </button>
                <button
                  className="bookmark-delete"
                  type="button"
                  aria-label={`删除书签 ${bookmarkTitle(bookmark)}`}
                  data-testid={`bookmark-delete-${bookmark.id}`}
                  data-od-id="C-34"
                  onClick={() => onRequestDelete(bookmark)}
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
